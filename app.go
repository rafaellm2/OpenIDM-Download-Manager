package main

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"openidm/internal/engine"
	"openidm/internal/integration"
	"openidm/internal/models"
	"openidm/internal/scheduler"
	"openidm/internal/store"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	AppWindowTitle            = "OpenIDM - Gerenciador de Downloads"
	DefaultCardWidth          = 680
	DefaultCardPromptHeight   = 430
	DefaultCardListHeight     = 280
	DefaultCardCombinedHeight = 430
)

// App centraliza a fachada do aplicativo nativo Windows (Wails v2) e orquestra Store, Engine, Scheduler, Bandeja do Relógio e Integração.
type App struct {
	ctx                  context.Context
	store                *store.Store
	manager              *engine.Manager
	scheduler            *scheduler.Scheduler
	clipboard            *integration.ClipboardMonitor
	bridge               *integration.BridgeServer
	tray                 *integration.SystemTray
	captureMu            sync.Mutex
	windowMode           string
	windowHidden         bool
	cardPositioned       bool
	cardWidth            int
	cardHeight           int
	cardPosX             int
	cardPosY             int
	cardPosSaved         bool
	lastAppliedX         int
	lastAppliedY         int
	hasAppliedPos        bool
	lastProgrammaticMove time.Time
	pendingCapture       *models.CapturePrompt
	captureQueue         []*models.CapturePrompt
	cardDownloadIDs      []string
	dismissedCardIDs     map[string]bool
	lastPrompt           *models.CapturePrompt
	lastPromptURL        string
	lastPromptTime       time.Time
	notifiedTrayOnce     bool
	quitting             bool
	pendingInstallerOpen bool
	completedNotifMu     sync.Mutex
	completedNotif       map[string]bool
}

// NewApp inicializa o banco SQLite, o motor de segmentação dinâmica, o agendador, a bandeja do sistema e a ponte de captura.
func NewApp(dbPath string, frontendFS fs.FS) (*App, error) {
	if dbPath == "" {
		appData, err := os.UserConfigDir()
		if err != nil || appData == "" {
			appData = "."
		}
		dbPath = filepath.Join(appData, "OpenIDM", "openidm.sqlite")
	}

	st, err := store.NewStore(dbPath)
	if err != nil {
		return nil, err
	}

	mgr, err := engine.NewManager(st)
	if err != nil {
		_ = st.Close()
		return nil, err
	}

	sched := scheduler.NewScheduler(st, mgr)
	bridge := integration.NewBridgeServer(st, mgr, sched, frontendFS)

	app := &App{
		store:            st,
		manager:          mgr,
		scheduler:        sched,
		bridge:           bridge,
		windowMode:       "full",
		cardWidth:        DefaultCardWidth,
		cardHeight:       DefaultCardPromptHeight,
		dismissedCardIDs: make(map[string]bool),
		completedNotif:   make(map[string]bool),
	}

	initialLang := "pt-BR"
	if cfg, cfgErr := st.GetSettings(); cfgErr == nil {
		initialLang = models.NormalizeLanguage(cfg.Language)
		if cfg.CardPosSaved {
			app.cardPosX = cfg.CardPosX
			app.cardPosY = cfg.CardPosY
			app.cardPosSaved = true
		}
	}

	// Emite atualizações em tempo real para a janela nativa Wails, vincula downloads ativos ao card solto e atualiza o ícone do relógio.
	mgr.OnUpdate(func(d models.Download) {
		if d.Status == models.StatusDownloading || d.Status == models.StatusProbing || d.Status == models.StatusAssembling {
			app.trackCardDownloadID(d.ID, false)
		}
		app.emitEvent("download_update", d)
		if d.Status == models.StatusCompleted {
			app.completedNotifMu.Lock()
			already := app.completedNotif[d.ID]
			app.completedNotif[d.ID] = true
			app.completedNotifMu.Unlock()
			if !already && app.tray != nil {
				if app.currentLanguage() == "en-US" {
					app.tray.ShowBalloon("Download Complete — OpenIDM", d.FileName+" saved in "+d.SaveDir)
				} else {
					app.tray.ShowBalloon("Download Concluído — OpenIDM", d.FileName+" salvo em "+d.SaveDir)
				}
			}
		}
	})

	// Quando o navegador captura um clique em link de download, abre apenas o Card Solto perguntando onde salvar e mostrando a lista ativa.
	bridge.SetCapturePromptHandler(func(msg integration.NativeMessagePayload) {
		app.PromptCaptureURL(msg.URL, msg.FileName, msg.Referrer, "browser")
	})

	bridge.SetOpenAppHandler(func() {
		go app.ShowWindow()
	})

	bridge.SetSettingsUpdatedHandler(func(cfg models.AppSettings) {
		norm := models.NormalizeLanguage(cfg.Language)
		if app.tray != nil {
			app.tray.SetLanguage(norm)
		}
		app.emitEvent("language_changed", map[string]string{"language": norm})
	})

	// Quando um link de download é copiado na área de transferência, abre apenas o Card Solto sem abrir o app completo.
	app.clipboard = integration.NewClipboardMonitor(st, func(detectedURL string) {
		app.PromptCaptureURL(detectedURL, "", "", "clipboard")
	})

	// Configura o ícone nativo na bandeja do sistema do Windows (canto do relógio).
	app.tray = integration.NewSystemTray(integration.TrayCallbacks{
		OnOpen: func() {
			app.ShowWindow()
		},
		OnOpenCard: func() {
			app.OpenStandaloneCardFromTray()
		},
		OnCaptureLink: func() {
			app.OpenCaptureModalFromTray()
		},
		OnStartMainQueue: func() {
			_ = app.StartQueue("main")
		},
		OnPauseAll: func() {
			app.PauseAll()
		},
		OnQuit: func() {
			app.QuitApp()
		},
	})
	app.tray.SetLanguage(initialLang)

	return app, nil
}

// Startup é invocado pelo ciclo de vida do Wails ao iniciar a janela nativa no Windows.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.ensureExtendedCaptureSettings()
	_ = a.bridge.Start(integration.DefaultBridgeAddr)
	a.scheduler.Start()
	a.clipboard.Start()
	if a.tray != nil {
		a.tray.Start()
	}

	// Monitora em segundo plano se o usuário movimentou o card de lugar na tela para salvar a preferência de posição atual.
	go func() {
		ticker := time.NewTicker(600 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.syncCardPositionFromOS()
			}
		}
	}()

	// Garante aplicação do ícone oficial na janela, arquivos .ico/PNG da extensão, auto-start e Native Messaging Host.
	go func() {
		integration.ApplyWindowIcon(AppWindowTitle)
		time.Sleep(180 * time.Millisecond)
		integration.ApplyWindowIcon(AppWindowTitle)

		if exePath, err := os.Executable(); err == nil && exePath != "" {
			exeDir := filepath.Dir(exePath)
			_, _ = integration.EnsureIconAssets(exeDir)
			if cfg, cErr := a.store.GetSettings(); cErr == nil {
				if autoEnabled, _ := integration.GetAutoStartStatus(); autoEnabled && !cfg.StartWithWindows {
					cfg.StartWithWindows = true
					_ = a.store.SaveSettings(cfg)
				} else if cfg.StartWithWindows {
					_ = integration.SetAutoStartWithWindows(exePath, true, true)
				}
			}
		}
		_, _ = a.InstallBrowserIntegration("")

		if a.pendingInstallerOpen {
			time.Sleep(250 * time.Millisecond)
			a.emitEvent("open_installer", map[string]bool{"open": true})
		}
	}()

	// Monitora periodicamente em background se há uma nova release publicada no GitHub (respeitando adiamento de 24h).
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
			_, _ = a.CheckForUpdates(false)
		}
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = a.CheckForUpdates(false)
			}
		}
	}()

	// Verifica se o executável foi chamado passando uma URL via linha de comando.
	for _, arg := range os.Args[1:] {
		clean := strings.TrimSpace(arg)
		if clean == "--installer" || clean == "-installer" {
			a.pendingInstallerOpen = true
			continue
		}
		if isSupportedDownloadURL(clean) {
			go func(u string) {
				time.Sleep(400 * time.Millisecond)
				a.PromptCaptureURL(u, "", "", "cli")
			}(clean)
			break
		}
	}
}

// ensureExtendedCaptureSettings sincroniza novas extensões nas configurações globais e nas categorias do SQLite,
// removendo extensões ambíguas (.bin, .ts, .txt, .md) da captura automática de clipboard para evitar falsos-positivos.
func (a *App) ensureExtendedCaptureSettings() {
	cfg, err := a.store.GetSettings()
	if err == nil {
		noisyExts := map[string]bool{
			"bin": true,
			"ts":  true,
			"txt": true,
			"md":  true,
		}
		extraExts := []string{
			"safetensors", "gguf", "ckpt", "pt", "pth", "onnx", "whl",
			"bz2", "xz", "tgz", "zst", "img", "dmg", "deb", "rpm", "torrent",
			"doc", "xls", "ppt", "epub", "csv",
		}
		filtered := make([]string, 0, len(cfg.CaptureExtensions))
		existing := make(map[string]bool, len(cfg.CaptureExtensions))
		changed := false
		for _, e := range cfg.CaptureExtensions {
			norm := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(e), "."))
			if norm == "" || noisyExts[norm] {
				changed = true
				continue
			}
			if !existing[norm] {
				filtered = append(filtered, norm)
				existing[norm] = true
			}
		}
		for _, ext := range extraExts {
			if !existing[ext] {
				filtered = append(filtered, ext)
				existing[ext] = true
				changed = true
			}
		}
		if changed {
			cfg.CaptureExtensions = filtered
			_ = a.store.SaveSettings(cfg)
		}
	}

	// Garante que bancos SQLite já existentes também classifiquem modelos de IA, pacotes e arquivos compactados extras nas categorias corretas.
	cats, cErr := a.store.ListCategories()
	if cErr == nil {
		catAdditions := map[string][]string{
			"programs":   {"safetensors", "gguf", "ckpt", "pt", "pth", "onnx", "bin", "whl", "dmg", "deb", "rpm", "img"},
			"compressed": {"bz2", "xz", "tgz", "zst", "torrent"},
			"documents":  {"doc", "xls", "ppt", "epub", "csv", "txt", "md"},
		}
		for _, c := range cats {
			addList, ok := catAdditions[c.ID]
			if !ok {
				continue
			}
			extMap := make(map[string]bool, len(c.Extensions))
			for _, e := range c.Extensions {
				extMap[strings.ToLower(strings.TrimPrefix(e, "."))] = true
			}
			catChanged := false
			for _, ext := range addList {
				if !extMap[ext] {
					c.Extensions = append(c.Extensions, ext)
					extMap[ext] = true
					catChanged = true
				}
			}
			if catChanged {
				_ = a.store.SaveCategory(c)
			}
		}
	}
}

// Shutdown encerra com segurança os serviços em background, remove o ícone da bandeja e salva os checkpoints.
func (a *App) Shutdown(_ context.Context) {
	if a.tray != nil {
		a.tray.Stop()
	}
	if a.clipboard != nil {
		a.clipboard.Stop()
	}
	if a.scheduler != nil {
		a.scheduler.Stop()
	}
	if a.bridge != nil {
		a.bridge.Stop()
	}
	if a.manager != nil {
		a.manager.PauseAll()
	}
	if a.store != nil {
		_ = a.store.Close()
	}
}

// HandleSecondInstance trata quando o usuário abre o executável novamente ou clica em um link associado ao app.
func (a *App) HandleSecondInstance(args []string) {
	for _, arg := range args {
		clean := strings.TrimSpace(arg)
		if clean == "--installer" || clean == "-installer" {
			a.captureMu.Lock()
			a.pendingInstallerOpen = true
			a.captureMu.Unlock()
			a.ShowWindow()
			a.emitEvent("open_installer", map[string]bool{"open": true})
			return
		}
		if isSupportedDownloadURL(clean) {
			a.PromptCaptureURL(clean, "", "", "cli")
			return
		}
	}
	a.ShowWindow()
}

// BeforeClose intercepta o clique no X da janela nativa do Windows para manter o aplicativo ativo no canto do relógio.
func (a *App) BeforeClose(_ context.Context) bool {
	if a.quitting {
		return false
	}
	a.HideToTray()
	return true
}

// SetInitialWindowMode define o modo inicial da janela ("card" ou "full") antes da abertura.
func (a *App) SetInitialWindowMode(mode string) {
	a.captureMu.Lock()
	if mode == "card" || mode == "full" {
		a.windowMode = mode
	}
	a.captureMu.Unlock()
}

// SetStartHidden informa se a janela iniciou oculta na bandeja do relógio (--tray).
func (a *App) SetStartHidden(hidden bool) {
	a.captureMu.Lock()
	a.windowHidden = hidden
	a.captureMu.Unlock()
}

// GetWindowMode retorna o modo atual da interface ("card" para o card solto compacto ou "full" para o gerenciador completo).
func (a *App) GetWindowMode() string {
	a.captureMu.Lock()
	defer a.captureMu.Unlock()
	if a.windowMode == "" {
		return "full"
	}
	return a.windowMode
}

// SetWindowMode alterna entre o modo Card Solto ("card") e o Gerenciador Completo ("full").
func (a *App) SetWindowMode(mode string) string {
	if mode == "card" {
		a.ShowStandaloneCard()
		return "card"
	}
	a.ShowWindow()
	return "full"
}

// ShowWindow restaura a janela completa (1240x780) do Gerenciador de Downloads a partir do relógio.
func (a *App) ShowWindow() {
	a.syncCardPositionFromOS()

	a.captureMu.Lock()
	a.windowMode = "full"
	a.windowHidden = false
	a.cardPositioned = false
	a.hasAppliedPos = false
	a.pendingCapture = nil
	a.lastProgrammaticMove = time.Now()
	a.captureMu.Unlock()

	a.emitEvent("window_mode", map[string]any{"mode": "full"})

	if a.ctx != nil {
		if wailsRuntime.WindowIsMinimised(a.ctx) {
			wailsRuntime.WindowUnminimise(a.ctx)
		}
		wailsRuntime.WindowSetAlwaysOnTop(a.ctx, false)
		wailsRuntime.WindowSetMaxSize(a.ctx, 0, 0)
		wailsRuntime.WindowSetMinSize(a.ctx, 960, 600)
		wailsRuntime.WindowSetSize(a.ctx, 1240, 780)
		wailsRuntime.WindowCenter(a.ctx)
		wailsRuntime.WindowShow(a.ctx)
	}
	integration.ForceForegroundWindow(AppWindowTitle)
	integration.ReleaseTopMostWindow(AppWindowTitle)
	integration.ApplyWindowIcon(AppWindowTitle)

	go func() {
		time.Sleep(120 * time.Millisecond)
		a.captureMu.Lock()
		stillFullVisible := a.windowMode == "full" && !a.windowHidden
		a.captureMu.Unlock()
		if stillFullVisible {
			integration.ForceForegroundWindow(AppWindowTitle)
			integration.ReleaseTopMostWindow(AppWindowTitle)
			integration.ApplyWindowIcon(AppWindowTitle)
		}
	}()
}

// ShowStandaloneCard exibe o Card Reduzido retangular no canto esquerdo (ou na posição salva pelo usuário),
// permitindo que a janela fique atrás de outras janelas normalmente.
func (a *App) ShowStandaloneCard() {
	a.captureMu.Lock()
	if a.windowMode != "card" {
		a.cardPositioned = false
	}
	a.windowMode = "card"
	a.windowHidden = false
	a.captureMu.Unlock()

	a.presentStandaloneCardIfActive()
}

// presentStandaloneCardIfActive aplica o layout e exibe o Card Solto apenas se o modo atual ainda for "card".
func (a *App) presentStandaloneCardIfActive() {
	a.syncCardPositionFromOS()

	a.captureMu.Lock()
	if a.windowMode != "card" {
		a.captureMu.Unlock()
		return
	}
	wasPositioned := a.cardPositioned && !a.windowHidden
	a.windowHidden = false
	hasPending := a.pendingCapture != nil || len(a.captureQueue) > 0
	hasDownloads := len(a.cardDownloadIDs) > 0
	width := a.cardWidth
	if width < 520 {
		width = DefaultCardWidth
	}
	height := a.cardHeight
	if hasPending {
		height = DefaultCardPromptHeight
	} else if hasDownloads && (height < 200 || height > 420) {
		height = DefaultCardListHeight
	} else if height < 200 {
		height = DefaultCardListHeight
	}
	a.cardWidth = width
	a.cardHeight = height
	savedX := a.cardPosX
	savedY := a.cardPosY
	hasSavedPos := a.cardPosSaved
	a.lastProgrammaticMove = time.Now()
	a.captureMu.Unlock()

	a.emitEvent("window_mode", map[string]any{"mode": "card"})

	if a.ctx != nil {
		if wailsRuntime.WindowIsMaximised(a.ctx) {
			wailsRuntime.WindowUnmaximise(a.ctx)
		}
		if wailsRuntime.WindowIsMinimised(a.ctx) {
			wailsRuntime.WindowUnminimise(a.ctx)
		}
		wailsRuntime.WindowSetMinSize(a.ctx, 500, 175)
	}

	// Posiciona e dimensiona via Win32 antes/junto ao WindowShow para evitar piscar no centro da tela
	appliedX, appliedY, appliedOk := integration.ApplyStandaloneCardWindowAt(
		AppWindowTitle,
		width,
		height,
		!wasPositioned,
		savedX,
		savedY,
		hasSavedPos,
	)

	if a.ctx != nil {
		if !appliedOk {
			wailsRuntime.WindowSetSize(a.ctx, width, height)
			if hasSavedPos {
				wailsRuntime.WindowSetPosition(a.ctx, savedX, savedY)
			}
		}
		wailsRuntime.WindowShow(a.ctx)
	}

	if !appliedOk {
		appliedX, appliedY, appliedOk = integration.ApplyStandaloneCardWindowAt(
			AppWindowTitle,
			width,
			height,
			!wasPositioned,
			savedX,
			savedY,
			hasSavedPos,
		)
	}

	a.captureMu.Lock()
	a.cardPositioned = true
	a.lastProgrammaticMove = time.Now()
	if appliedOk {
		a.lastAppliedX = appliedX
		a.lastAppliedY = appliedY
		a.hasAppliedPos = true
	}
	a.captureMu.Unlock()

	if !wasPositioned {
		integration.ForceForegroundWindow(AppWindowTitle)
	}
	integration.ReleaseTopMostWindow(AppWindowTitle)
	integration.ApplyWindowIcon(AppWindowTitle)

	go func() {
		time.Sleep(120 * time.Millisecond)
		a.captureMu.Lock()
		stillCardVisible := a.windowMode == "card" && !a.windowHidden
		w := a.cardWidth
		h := a.cardHeight
		sX := a.cardPosX
		sY := a.cardPosY
		sOk := a.cardPosSaved
		a.captureMu.Unlock()
		if stillCardVisible {
			ax, ay, ok := integration.ApplyStandaloneCardWindowAt(AppWindowTitle, w, h, false, sX, sY, sOk)
			if ok {
				a.captureMu.Lock()
				a.lastAppliedX = ax
				a.lastAppliedY = ay
				a.hasAppliedPos = true
				a.lastProgrammaticMove = time.Now()
				a.captureMu.Unlock()
			}
			integration.ReleaseTopMostWindow(AppWindowTitle)
			integration.ApplyWindowIcon(AppWindowTitle)
		}
	}()
}

// ResizeStandaloneCard ajusta dinamicamente as dimensões retangulares do card sem roubar foco nem fixar no topo.
func (a *App) ResizeStandaloneCard(width int, height int) {
	if width < 500 {
		width = 500
	} else if width > 960 {
		width = 960
	}
	if height < 200 {
		height = 200
	} else if height > 780 {
		height = 780
	}

	a.syncCardPositionFromOS()

	a.captureMu.Lock()
	isCard := a.windowMode == "card"
	isHidden := a.windowHidden
	wasPositioned := a.cardPositioned
	sameDimensions := a.cardWidth == width && a.cardHeight == height && wasPositioned
	a.cardWidth = width
	a.cardHeight = height
	savedX := a.cardPosX
	savedY := a.cardPosY
	hasSavedPos := a.cardPosSaved
	a.captureMu.Unlock()

	if !isCard || isHidden || sameDimensions {
		return
	}
	if a.ctx != nil {
		wailsRuntime.WindowSetMinSize(a.ctx, 500, 175)
	}
	appliedX, appliedY, appliedOk := integration.ApplyStandaloneCardWindowAt(
		AppWindowTitle,
		width,
		height,
		!wasPositioned,
		savedX,
		savedY,
		hasSavedPos,
	)
	a.captureMu.Lock()
	a.cardPositioned = true
	a.lastProgrammaticMove = time.Now()
	if appliedOk {
		a.lastAppliedX = appliedX
		a.lastAppliedY = appliedY
		a.hasAppliedPos = true
	}
	a.captureMu.Unlock()
	if !appliedOk && a.ctx != nil {
		wailsRuntime.WindowSetSize(a.ctx, width, height)
	}
	integration.ReleaseTopMostWindow(AppWindowTitle)
}

// SaveCardPosition salva explicitamente a preferência de posição (X, Y) onde o card reduzido foi colocado na tela.
func (a *App) SaveCardPosition(x int, y int) {
	a.captureMu.Lock()
	changed := !a.cardPosSaved || a.cardPosX != x || a.cardPosY != y
	a.cardPosX = x
	a.cardPosY = y
	a.cardPosSaved = true
	a.lastAppliedX = x
	a.lastAppliedY = y
	a.hasAppliedPos = true
	a.captureMu.Unlock()

	if changed && a.store != nil {
		if cfg, err := a.store.GetSettings(); err == nil {
			cfg.CardPosX = x
			cfg.CardPosY = y
			cfg.CardPosSaved = true
			_ = a.store.SaveSettings(cfg)
		}
	}
}

// GetCardPosition retorna a posição salva do card reduzido.
func (a *App) GetCardPosition() map[string]any {
	a.syncCardPositionFromOS()
	a.captureMu.Lock()
	defer a.captureMu.Unlock()
	return map[string]any{
		"x":     a.cardPosX,
		"y":     a.cardPosY,
		"saved": a.cardPosSaved,
	}
}

// syncCardPositionFromOS detecta se o usuário arrastou o card na tela e persiste a preferência de posição atual.
func (a *App) syncCardPositionFromOS() {
	a.captureMu.Lock()
	isCardVisible := a.windowMode == "card" && !a.windowHidden && a.cardPositioned
	lastX := a.lastAppliedX
	lastY := a.lastAppliedY
	hasApplied := a.hasAppliedPos
	inCooldown := !a.lastProgrammaticMove.IsZero() && time.Since(a.lastProgrammaticMove) < 180*time.Millisecond
	a.captureMu.Unlock()

	if !isCardVisible || !hasApplied || inCooldown {
		return
	}
	curX, curY, curW, _, ok := integration.GetWindowBounds(AppWindowTitle)
	if !ok || curW <= 0 {
		return
	}
	if absDiff(curX, lastX) > 4 || absDiff(curY, lastY) > 4 {
		a.SaveCardPosition(curX, curY)
	}
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// OpenStandaloneCardFromTray abre o Card Solto a partir do ícone da bandeja do relógio sem abrir o app completo.
func (a *App) OpenStandaloneCardFromTray() {
	a.ShowStandaloneCard()
}

// OpenFullAppFromCard expande do Card Solto para a janela completa do Gerenciador de Downloads.
func (a *App) OpenFullAppFromCard() {
	a.ShowWindow()
}

// MinimizeToCard minimiza o app principal para o relógio e deixa apenas o Card Solto na tela para acompanhar os downloads.
func (a *App) MinimizeToCard() {
	a.ShowStandaloneCard()
}

// CloseStandaloneCard oculta o Card Solto mantendo o OpenIDM ativo no canto do relógio.
func (a *App) CloseStandaloneCard() {
	a.HideToTray()
}

// HideToTray oculta a janela deixando o OpenIDM rodando na bandeja do sistema (canto do relógio).
func (a *App) HideToTray() {
	a.syncCardPositionFromOS()

	a.captureMu.Lock()
	a.windowHidden = true
	a.cardPositioned = false
	a.captureMu.Unlock()

	if a.ctx != nil {
		wailsRuntime.WindowHide(a.ctx)
	}
	if a.tray != nil && !a.notifiedTrayOnce {
		a.notifiedTrayOnce = true
		if a.currentLanguage() == "en-US" {
			a.tray.ShowBalloon(
				"OpenIDM in System Tray",
				"The application remains active in the Windows system tray monitoring your downloads and links.",
			)
		} else {
			a.tray.ShowBalloon(
				"OpenIDM no Canto do Relógio",
				"O aplicativo continua ativo na bandeja do Windows monitorando seus downloads e links.",
			)
		}
	}
}

// MinimizeWindow minimiza a janela nativa para a barra de tarefas.
func (a *App) MinimizeWindow() {
	if a.ctx != nil {
		wailsRuntime.WindowMinimise(a.ctx)
	}
}

// ToggleMaximizeWindow alterna entre janela maximizada e tamanho normal.
func (a *App) ToggleMaximizeWindow() {
	if a.ctx != nil {
		wailsRuntime.WindowToggleMaximise(a.ctx)
	}
}

// QuitApp encerra completamente o aplicativo e remove o ícone do canto do relógio.
func (a *App) QuitApp() {
	a.quitting = true
	if a.tray != nil {
		a.tray.Stop()
	}
	if a.ctx != nil {
		wailsRuntime.Quit(a.ctx)
	} else {
		os.Exit(0)
	}
}

// OpenCaptureModalFromTray abre apenas o Card Solto a partir do relógio já com o link da área de transferência.
func (a *App) OpenCaptureModalFromTray() {
	clipURL := a.GetClipboardURL()
	a.PromptCaptureURL(clipURL, "", "", "manual")
}

// PromptCaptureURL classifica o link na organização de categorias do OpenIDM e abre apenas o Card Solto
// (sem abrir o app completo), empilhando múltiplos links caso o usuário baixe mais de um.
func (a *App) PromptCaptureURL(rawURL, customName, referrer, source string) *models.CapturePrompt {
	cleanURL := engine.NormalizeDownloadURL(rawURL)
	cleanCustom := engine.SanitizeDownloadFileName(customName)

	var replacedURL string
	a.captureMu.Lock()
	if a.windowMode != "card" {
		a.cardPositioned = false
	}
	// Evita disparo duplicado caso o mesmo clique seja reportado pelo content script e pelo downloads.onCreated em menos de 3s,
	// e impede que um hash de blob sem extensão sobrescreva um nome de arquivo válido já identificado.
	if source != "manual" && cleanURL != "" && cleanURL == a.lastPromptURL && time.Since(a.lastPromptTime) < 3000*time.Millisecond {
		existing := a.lastPrompt
		if existing != nil {
			if cleanCustom == "" ||
				strings.EqualFold(existing.FileName, cleanCustom) ||
				(!engine.IsGenericOrScriptFileName(existing.FileName) && engine.IsGenericOrScriptFileName(cleanCustom)) {
				a.windowMode = "card"
				a.windowHidden = false
				a.captureMu.Unlock()
				go a.presentStandaloneCardIfActive()
				return existing
			}
		}
	}
	// Se a segunda chamada para outra URL (redirecionamento 302 para CDN ou hash blob) ocorrer em menos de 2.5s
	// na mesma aba (mesmo referrer) ou com o mesmo nome de arquivo válido, substitui a URL pré-redirecionamento na fila.
	if source != "manual" {
		if existing := a.lastPrompt; existing != nil && time.Since(a.lastPromptTime) < 2500*time.Millisecond {
			sameReferrer := referrer != "" && strings.EqualFold(existing.Referrer, referrer)
			sameValidFile := cleanCustom != "" && !engine.IsGenericOrScriptFileName(cleanCustom) && strings.EqualFold(existing.FileName, cleanCustom)
			genericRedirect := engine.IsGenericOrScriptFileName(cleanCustom) &&
				(isHexOrUUIDBlobName(cleanCustom) || isHexOrUUIDBlobName(engine.ExtractFileNameFromURL(cleanURL))) &&
				!engine.IsGenericOrScriptFileName(existing.FileName) &&
				(sameReferrer || existing.URL == cleanURL)

			if genericRedirect || (sameValidFile && (sameReferrer || existing.Referrer == "" || referrer == "")) {
				customName = existing.FileName
				replacedURL = existing.URL
			}
		}
	}
	if cleanURL != "" {
		a.lastPromptURL = cleanURL
		a.lastPromptTime = time.Now()
	}
	a.captureMu.Unlock()

	prompt := a.buildCapturePrompt(cleanURL, customName, referrer, source)

	a.captureMu.Lock()
	if a.windowMode != "card" {
		a.cardPositioned = false
	}
	a.pendingCapture = prompt
	a.lastPrompt = prompt
	a.windowMode = "card"
	a.windowHidden = false
	a.enqueueCapturePromptLocked(prompt, replacedURL)
	a.captureMu.Unlock()

	// Abre apenas o Card Solto em goroutine para não bloquear a resposta IPC HTTP (<5ms para a extensão do navegador).
	go a.presentStandaloneCardIfActive()
	a.emitEvent("capture_prompt", prompt)
	a.emitEvent("capture_queue_updated", a.GetPendingCaptureQueue())
	// Re-emite após o card restaurar da bandeja apenas se o WebView2 ainda não tiver acusado abertura.
	if a.ctx != nil {
		go func(expected *models.CapturePrompt) {
			time.Sleep(180 * time.Millisecond)
			a.captureMu.Lock()
			stillPending := a.pendingCapture != nil && a.pendingCapture == expected
			a.captureMu.Unlock()
			if stillPending {
				a.emitEvent("capture_prompt", expected)
				a.emitEvent("capture_queue_updated", a.GetPendingCaptureQueue())
			}
		}(prompt)
	}
	return prompt
}

// enqueueCapturePromptLocked insere ou atualiza um prompt na fila de capturas pendentes do Card Solto.
func (a *App) enqueueCapturePromptLocked(prompt *models.CapturePrompt, replacedURL string) {
	if prompt == nil {
		return
	}
	clean := strings.TrimSpace(prompt.URL)
	if clean == "" {
		return
	}
	cleanReplaced := strings.TrimSpace(replacedURL)
	for i, existing := range a.captureQueue {
		if existing == nil {
			continue
		}
		existingURL := strings.TrimSpace(existing.URL)
		if strings.EqualFold(existingURL, clean) || (cleanReplaced != "" && strings.EqualFold(existingURL, cleanReplaced)) {
			a.captureQueue[i] = prompt
			return
		}
	}
	a.captureQueue = append(a.captureQueue, prompt)
}

// GetPendingCaptureQueue retorna a fila de links capturados aguardando confirmação no Card Solto.
func (a *App) GetPendingCaptureQueue() []*models.CapturePrompt {
	a.captureMu.Lock()
	defer a.captureMu.Unlock()
	out := make([]*models.CapturePrompt, 0, len(a.captureQueue))
	for _, item := range a.captureQueue {
		if item != nil {
			cp := *item
			out = append(out, &cp)
		}
	}
	return out
}

// RemovePendingCaptureByURL remove um link específico da fila de capturas pendentes (quando salvo ou cancelado no card).
func (a *App) RemovePendingCaptureByURL(rawURL string) {
	clean := engine.NormalizeDownloadURL(rawURL)
	a.captureMu.Lock()
	filtered := a.captureQueue[:0]
	for _, item := range a.captureQueue {
		if item == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.URL), strings.TrimSpace(rawURL)) ||
			(clean != "" && strings.EqualFold(strings.TrimSpace(item.URL), clean)) {
			continue
		}
		filtered = append(filtered, item)
	}
	a.captureQueue = filtered
	if a.pendingCapture != nil && (strings.EqualFold(a.pendingCapture.URL, rawURL) || strings.EqualFold(a.pendingCapture.URL, clean)) {
		if len(a.captureQueue) > 0 {
			a.pendingCapture = a.captureQueue[0]
		} else {
			a.pendingCapture = nil
		}
	}
	a.captureMu.Unlock()
	a.emitEvent("capture_queue_updated", a.GetPendingCaptureQueue())
}

// GetPendingCapture retorna e limpa um link capturado pendente (útil quando a UI acaba de montar ou focar).
func (a *App) GetPendingCapture() *models.CapturePrompt {
	a.captureMu.Lock()
	defer a.captureMu.Unlock()
	p := a.pendingCapture
	a.pendingCapture = nil
	return p
}

// AckCaptureModalOpened confirma que a tela de captura foi aberta e renderizada no WebView2 sem erros.
func (a *App) AckCaptureModalOpened(url string, foldersCount int) {
	a.captureMu.Lock()
	a.pendingCapture = nil
	a.captureMu.Unlock()
	if a.bridge != nil {
		a.bridge.RecordModalOpened(url, foldersCount)
	}
}

// ReportFrontendError registra qualquer erro de renderização do WebView2 para telemetria local.
func (a *App) ReportFrontendError(errMsg string) {
	if a.bridge != nil {
		a.bridge.RecordFrontendError(errMsg)
	}
}

// ProbeAndClassifyURL consulta os cabeçalhos do servidor (tamanho real, suporte a Range, redirecionamentos e Content-Disposition)
// e classifica automaticamente o arquivo na organização de categorias e pastas do OpenIDM.
func (a *App) ProbeAndClassifyURL(rawURL string, customName string) (*models.CapturePrompt, error) {
	cleanURL := engine.NormalizeDownloadURL(rawURL)

	var referrer string
	a.captureMu.Lock()
	if a.lastPrompt != nil && (a.lastPrompt.URL == cleanURL || a.lastPrompt.URL == strings.TrimSpace(rawURL)) {
		referrer = a.lastPrompt.Referrer
	}
	a.captureMu.Unlock()

	prompt := a.buildCapturePrompt(cleanURL, customName, referrer, "manual")
	if !isSupportedDownloadURL(cleanURL) {
		return prompt, nil
	}

	settings, _ := a.store.GetSettings()
	login := a.manager.FindLoginForURL(cleanURL)
	client := engine.BuildHTTPClient(settings, 4)

	probeCtx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	probeRes, err := engine.ProbeURLWithReferrer(probeCtx, client, cleanURL, login, referrer)
	if err != nil {
		prompt.Probed = false
		prompt.ProbeError = err.Error()
		return prompt, nil
	}
	if probeRes != nil {
		prompt.TotalSize = probeRes.TotalSize
		prompt.AcceptRanges = probeRes.AcceptRanges
		prompt.Probed = true

		effectiveName := engine.SanitizeDownloadFileName(customName)
		urlDefaultName := engine.ExtractFileNameFromURL(cleanURL)
		probedName := engine.SanitizeDownloadFileName(probeRes.FileName)

		// O nome descoberto no servidor (Content-Disposition ou URL final pós-302) tem prioridade sobre
		// nomes genéricos/scripts extraídos da URL inicial ou sem extensão.
		if !engine.IsGenericOrScriptFileName(probedName) {
			if engine.IsGenericOrScriptFileName(effectiveName) || strings.EqualFold(effectiveName, urlDefaultName) {
				effectiveName = probedName
			}
		}

		if effectiveName != "" {
			reclassified := a.buildCapturePrompt(cleanURL, effectiveName, referrer, "manual")
			prompt.FileName = reclassified.FileName
			prompt.CategoryID = reclassified.CategoryID
			prompt.CategoryName = reclassified.CategoryName
			prompt.SaveDir = reclassified.SaveDir
			prompt.TargetPath = reclassified.TargetPath
		}
	}

	return prompt, nil
}

func (a *App) buildCapturePrompt(rawURL, customName, referrer, source string) *models.CapturePrompt {
	cats, _ := a.store.ListCategories()
	cfg, _ := a.store.GetSettings()

	var fileName, catID, saveDir string
	if strings.TrimSpace(rawURL) != "" {
		fileName, catID, saveDir = integration.SuggestCategoryAndDir(rawURL, customName, cats, cfg.DefaultSaveDir)
	} else {
		catID = "general"
		saveDir = cfg.DefaultSaveDir
		for _, c := range cats {
			if c.ID == "general" && c.DefaultDir != "" {
				saveDir = c.DefaultDir
				break
			}
		}
	}

	isEN := models.NormalizeLanguage(cfg.Language) == models.LangEnUS
	catName := "Geral / Outros"
	if isEN {
		catName = "General / Other"
	}
	for _, c := range cats {
		if c.ID == catID {
			catName = localizeDefaultCategoryName(c.ID, c.Name, isEN)
			break
		}
	}

	targetPath := saveDir
	if fileName != "" && saveDir != "" {
		targetPath = filepath.Join(saveDir, fileName)
	}

	if source == "" {
		source = "manual"
	}

	return &models.CapturePrompt{
		URL:          rawURL,
		FileName:     fileName,
		CategoryID:   catID,
		CategoryName: catName,
		SaveDir:      saveDir,
		TargetPath:   targetPath,
		Referrer:     referrer,
		Source:       source,
	}
}

// GetClipboardURL verifica se há uma URL de download válida (filtrando sites comuns, localhost e vídeos web) na área de transferência.
func (a *App) GetClipboardURL() string {
	txt := integration.ReadClipboardText()
	if !isSupportedDownloadURL(txt) {
		return ""
	}
	normalized := engine.NormalizeDownloadURL(txt)
	if a.store != nil {
		settings, err := a.store.GetSettings()
		if err == nil {
			rules, _ := a.store.ListSiteRules()
			if ok, _ := integration.ShouldCaptureURL(normalized, "", settings, rules); !ok {
				return ""
			}
		}
	}
	return normalized
}

// BrowseFolder abre o seletor nativo de pastas do Windows Explorer para o usuário escolher onde salvar.
func (a *App) BrowseFolder(defaultDir string) (string, error) {
	if a.ctx == nil {
		return defaultDir, nil
	}
	cleanDir := strings.TrimSpace(defaultDir)
	if cleanDir != "" {
		_ = os.MkdirAll(cleanDir, 0o755)
	}
	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, false)
	integration.ReleaseTopMostWindow(AppWindowTitle)
	dialogTitle := "Escolher Pasta na Organização do OpenIDM"
	if a.currentLanguage() == "en-US" {
		dialogTitle = "Choose Folder in OpenIDM Organization"
	}
	selected, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:                dialogTitle,
		DefaultDirectory:     cleanDir,
		CanCreateDirectories: true,
	})
	if err != nil {
		return "", err
	}
	return selected, nil
}

// OpenDirectoryPath cria (se necessário) e abre uma pasta da organização diretamente no Windows Explorer.
func (a *App) OpenDirectoryPath(dirPath string) error {
	clean := filepath.Clean(strings.TrimSpace(dirPath))
	if clean == "" || clean == "." {
		cfg, _ := a.store.GetSettings()
		clean = cfg.DefaultSaveDir
	}
	if err := os.MkdirAll(clean, 0o755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return exec.Command("explorer.exe", clean).Start()
	}
	return exec.Command("xdg-open", clean).Start()
}

// GetExistingDownloadFolders retorna as subpastas existentes dentro da pasta Downloads do usuário
// (ex: Compressed, Documents, Images, Music, Programs, Video, higgs, etc.) para seleção rápida na organização.
func (a *App) GetExistingDownloadFolders() []map[string]string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return nil
	}
	downloadsDir := filepath.Join(homeDir, "Downloads")
	entries, err := os.ReadDir(downloadsDir)
	if err != nil {
		return nil
	}
	var out []map[string]string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.EqualFold(name, "OpenIDM") || strings.EqualFold(name, "TurboDM") {
			continue
		}
		out = append(out, map[string]string{
			"name": name,
			"path": filepath.Join(downloadsDir, name),
		})
	}
	return out
}

func (a *App) emitEvent(eventType string, payload any) {
	ev := integration.SSEEvent{
		Type:    eventType,
		Payload: payload,
	}
	if a.bridge != nil {
		a.bridge.Broadcast(ev)
	}
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "openidm:event", ev)
	}
}

func isSupportedDownloadURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return (strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "ftp://")) && len(lower) < 2048
}

// isHexOrUUIDBlobName identifica identificadores opacos de blobs de CDN (ex: 69dc9a3d52797c523c6c91bc ou UUIDs sem extensão).
func isHexOrUUIDBlobName(name string) bool {
	clean := strings.TrimSpace(name)
	if len(clean) < 16 || filepath.Ext(clean) != "" {
		return false
	}
	for _, r := range clean {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') || r == '-'
		if !isHex {
			return false
		}
	}
	return true
}

// trackCardDownloadID vincula um download à lista do Card Solto para acompanhamento com o app principal no relógio.
func (a *App) trackCardDownloadID(id string, forceUnDismiss bool) {
	if strings.TrimSpace(id) == "" {
		return
	}
	a.captureMu.Lock()
	defer a.captureMu.Unlock()
	if forceUnDismiss {
		delete(a.dismissedCardIDs, id)
	} else if a.dismissedCardIDs[id] {
		return
	}
	for _, existing := range a.cardDownloadIDs {
		if existing == id {
			return
		}
	}
	a.cardDownloadIDs = append([]string{id}, a.cardDownloadIDs...)
}

// GetCardDownloads retorna a lista consolidada de downloads exibidos no Card Solto (downloads iniciados no card e downloads ativos).
func (a *App) GetCardDownloads() []models.Download {
	all := a.manager.ListDownloads()
	byID := make(map[string]models.Download, len(all))
	for _, d := range all {
		byID[d.ID] = d
		if d.Status == models.StatusDownloading || d.Status == models.StatusProbing || d.Status == models.StatusAssembling {
			a.trackCardDownloadID(d.ID, false)
		}
	}

	a.captureMu.Lock()
	ids := make([]string, len(a.cardDownloadIDs))
	copy(ids, a.cardDownloadIDs)
	dismissed := make(map[string]bool, len(a.dismissedCardIDs))
	for k, v := range a.dismissedCardIDs {
		dismissed[k] = v
	}
	a.captureMu.Unlock()

	seen := make(map[string]bool, len(ids))
	out := make([]models.Download, 0, len(ids))
	for _, id := range ids {
		if dismissed[id] || seen[id] {
			continue
		}
		if d, ok := byID[id]; ok {
			seen[id] = true
			out = append(out, d)
		}
	}
	for _, d := range all {
		if dismissed[d.ID] || seen[d.ID] {
			continue
		}
		if d.Status == models.StatusDownloading || d.Status == models.StatusProbing || d.Status == models.StatusAssembling {
			seen[d.ID] = true
			out = append(out, d)
		}
	}
	return out
}

// DismissCardDownload remove um item específico da visualização do Card Solto sem apagá-lo do histórico principal.
func (a *App) DismissCardDownload(id string) {
	a.captureMu.Lock()
	a.dismissedCardIDs[id] = true
	filtered := a.cardDownloadIDs[:0]
	for _, cur := range a.cardDownloadIDs {
		if cur != id {
			filtered = append(filtered, cur)
		}
	}
	a.cardDownloadIDs = filtered
	a.captureMu.Unlock()
}

// ClearCompletedFromCard remove todos os downloads já concluídos ou cancelados da lista do Card Solto, mantendo os ativos.
func (a *App) ClearCompletedFromCard() {
	all := a.manager.ListDownloads()
	statusByID := make(map[string]models.DownloadStatus, len(all))
	for _, d := range all {
		statusByID[d.ID] = d.Status
	}

	a.captureMu.Lock()
	filtered := a.cardDownloadIDs[:0]
	for _, id := range a.cardDownloadIDs {
		st, exists := statusByID[id]
		if !exists || st == models.StatusCompleted || st == models.StatusCancelled {
			a.dismissedCardIDs[id] = true
			continue
		}
		filtered = append(filtered, id)
	}
	a.cardDownloadIDs = filtered
	a.captureMu.Unlock()
}

// GetDownloads retorna a lista completa de downloads.
func (a *App) GetDownloads() []models.Download {
	return a.manager.ListDownloads()
}

// GetDownload retorna os detalhes e mapa de segmentos de um download pelo ID.
func (a *App) GetDownload(id string) (*models.Download, error) {
	return a.manager.GetDownload(id)
}

// AddDownload adiciona um novo download ao gerenciador e o junta na lista do Card Solto para acompanhamento imediato.
func (a *App) AddDownload(req models.AddDownloadRequest) (*models.Download, error) {
	dl, err := a.manager.AddDownload(req)
	if err != nil {
		return nil, err
	}
	if dl != nil {
		a.trackCardDownloadID(dl.ID, true)
		a.RemovePendingCaptureByURL(req.URL)
		if dl.URL != "" && dl.URL != req.URL {
			a.RemovePendingCaptureByURL(dl.URL)
		}
		a.emitEvent("card_download_added", dl)
	}
	return dl, nil
}

// StartDownload inicia ou retoma um download pausado/com erro e garante presença na lista do Card Solto.
func (a *App) StartDownload(id string) error {
	a.trackCardDownloadID(id, true)
	return a.manager.StartDownload(id)
}

// PauseDownload pausa um download ativo salvando o checkpoint dos segmentos.
func (a *App) PauseDownload(id string) error {
	return a.manager.PauseDownload(id)
}

// PauseAll pausa todos os downloads ativos.
func (a *App) PauseAll() {
	a.manager.PauseAll()
}

// CancelDownload para/cancela um download.
func (a *App) CancelDownload(id string) error {
	return a.manager.CancelDownload(id)
}

// DeleteDownload remove o download da lista e opcionalmente apaga o arquivo do disco.
func (a *App) DeleteDownload(id string, deleteFile bool) error {
	a.DismissCardDownload(id)
	a.emitEvent("download_deleted", map[string]string{"id": id})
	return a.manager.DeleteDownload(id, deleteFile)
}

// DeleteCompleted limpa todos os downloads concluídos da lista.
func (a *App) DeleteCompleted() error {
	a.ClearCompletedFromCard()
	return a.manager.DeleteCompleted()
}

// SetDownloadSpeedLimit ajusta em tempo real o limite de banda (B/s) de um download.
func (a *App) SetDownloadSpeedLimit(id string, rateBps int64) error {
	return a.manager.SetDownloadSpeedLimit(id, rateBps)
}

// OpenFile abre o arquivo concluído no programa padrão do sistema operacional.
func (a *App) OpenFile(id string) error {
	dl, err := a.manager.GetDownload(id)
	if err != nil {
		return err
	}
	return engine.OpenFileInOS(dl.TargetPath)
}

// OpenFolder abre a pasta onde o arquivo está salvo no Windows Explorer.
func (a *App) OpenFolder(id string) error {
	dl, err := a.manager.GetDownload(id)
	if err != nil {
		return err
	}
	return engine.OpenFolderInOS(dl.TargetPath)
}

// GetCategories lista todas as categorias automáticas por extensão.
func (a *App) GetCategories() ([]models.Category, error) {
	return a.store.ListCategories()
}

// SaveCategory cria ou edita uma categoria da organização.
func (a *App) SaveCategory(c models.Category) error {
	return a.store.SaveCategory(c)
}

// GetQueues lista todas as filas do Agendador (Principal, Sincronização e Customizadas).
func (a *App) GetQueues() ([]models.Queue, error) {
	return a.store.ListQueues()
}

// SaveQueue cria ou atualiza uma fila no Agendador.
func (a *App) SaveQueue(q models.Queue) error {
	return a.store.SaveQueue(q)
}

// DeleteQueue remove uma fila customizada.
func (a *App) DeleteQueue(id string) error {
	return a.store.DeleteQueue(id)
}

// StartQueue inicia o processamento imediato de uma fila.
func (a *App) StartQueue(queueID string) error {
	return a.scheduler.StartQueue(queueID)
}

// StopQueue interrompe o processamento de uma fila e pausa seus itens ativos.
func (a *App) StopQueue(queueID string) error {
	return a.scheduler.StopQueue(queueID)
}

// CancelSystemShutdown aborta um desligamento agendado do Windows.
func (a *App) CancelSystemShutdown() error {
	return scheduler.CancelSystemShutdown()
}

// GetSettings retorna as configurações globais do OpenIDM.
func (a *App) GetSettings() (models.AppSettings, error) {
	return a.store.GetSettings()
}

// currentLanguage retorna o idioma atual configurado no aplicativo ("pt-BR" ou "en-US").
func (a *App) currentLanguage() string {
	if a.store != nil {
		if cfg, err := a.store.GetSettings(); err == nil {
			return models.NormalizeLanguage(cfg.Language)
		}
	}
	if a.tray != nil {
		return a.tray.GetLanguage()
	}
	return "pt-BR"
}

// GetLanguage retorna o idioma atual da interface ("pt-BR" ou "en-US").
func (a *App) GetLanguage() string {
	return a.currentLanguage()
}

// SetLanguage altera e persiste o idioma da interface ("pt-BR" ou "en-US") e atualiza a bandeja do sistema.
func (a *App) SetLanguage(lang string) (string, error) {
	norm := models.NormalizeLanguage(lang)
	if a.store != nil {
		cfg, err := a.store.GetSettings()
		if err != nil {
			return norm, err
		}
		cfg.Language = norm
		if err := a.store.SaveSettings(cfg); err != nil {
			return norm, err
		}
	}
	if a.tray != nil {
		a.tray.SetLanguage(norm)
	}
	a.emitEvent("language_changed", map[string]string{"language": norm})
	return norm, nil
}

// SaveSettings salva as configurações globais, sincroniza inicialização com o Windows e atualiza o limitador global de velocidade.
func (a *App) SaveSettings(cfg models.AppSettings) error {
	cfg.Language = models.NormalizeLanguage(cfg.Language)

	a.captureMu.Lock()
	if a.cardPosSaved && !cfg.CardPosSaved {
		cfg.CardPosX = a.cardPosX
		cfg.CardPosY = a.cardPosY
		cfg.CardPosSaved = true
	} else if cfg.CardPosSaved {
		a.cardPosX = cfg.CardPosX
		a.cardPosY = cfg.CardPosY
		a.cardPosSaved = true
	}
	a.captureMu.Unlock()

	if err := a.store.SaveSettings(cfg); err != nil {
		return err
	}
	if a.tray != nil {
		a.tray.SetLanguage(cfg.Language)
	}
	_ = integration.SetAutoStartWithWindows("", cfg.StartWithWindows, true)
	a.manager.RefreshGlobalLimiter(cfg.GlobalSpeedLimitBps)
	a.emitEvent("language_changed", map[string]string{"language": cfg.Language})
	return nil
}

// GetSiteRules lista as exceções e limites de conexões por site.
func (a *App) GetSiteRules() ([]models.SiteRule, error) {
	return a.store.ListSiteRules()
}

// SaveSiteRule salva uma regra/exceção por site (suporta *.exemplo.com).
func (a *App) SaveSiteRule(r models.SiteRule) error {
	return a.store.SaveSiteRule(r)
}

// DeleteSiteRule exclui uma regra de site pelo ID.
func (a *App) DeleteSiteRule(id int64) error {
	return a.store.DeleteSiteRule(id)
}

// GetSiteLogins lista todos os logins salvos por servidor.
func (a *App) GetSiteLogins() ([]models.SiteLogin, error) {
	return a.store.ListSiteLogins()
}

// SaveSiteLogin salva uma credencial automática por servidor.
func (a *App) SaveSiteLogin(l models.SiteLogin) error {
	return a.store.SaveSiteLogin(l)
}

// DeleteSiteLogin remove uma credencial de servidor pelo ID.
func (a *App) DeleteSiteLogin(id int64) error {
	return a.store.DeleteSiteLogin(id)
}

// ClassifyURL sugere automaticamente nome do arquivo, categoria e diretório de destino na organização.
func (a *App) ClassifyURL(rawURL string, customName string) map[string]string {
	cleanURL := engine.NormalizeDownloadURL(rawURL)
	cats, _ := a.store.ListCategories()
	cfg, _ := a.store.GetSettings()
	fileName, catID, saveDir := integration.SuggestCategoryAndDir(cleanURL, customName, cats, cfg.DefaultSaveDir)
	return map[string]string{
		"fileName":   fileName,
		"categoryId": catID,
		"saveDir":    saveDir,
	}
}

// InstallBrowserIntegration registra o Native Messaging Host no Registro do Windows para Chrome, Edge e Firefox.
func (a *App) InstallBrowserIntegration(chromeExtID string) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	hostPath := filepath.Join(filepath.Dir(exePath), "openidm-native-host.exe")
	return integration.InstallNativeMessagingHost(hostPath, chromeExtID)
}

// SetOpenInstallerOnStartup configura o aplicativo para abrir automaticamente o Assistente de Instalação ao iniciar.
func (a *App) SetOpenInstallerOnStartup(open bool) {
	a.captureMu.Lock()
	a.pendingInstallerOpen = open
	a.captureMu.Unlock()
}

// ShouldOpenInstaller verifica e consome a flag de abertura do Assistente de Instalação (--installer).
func (a *App) ShouldOpenInstaller() bool {
	a.captureMu.Lock()
	defer a.captureMu.Unlock()
	v := a.pendingInstallerOpen
	a.pendingInstallerOpen = false
	return v
}

// GetInstallationStatus retorna o diagnóstico completo da instalação no Windows (auto-start, atalhos, extensão, navegadores).
func (a *App) GetInstallationStatus() integration.InstallationStatus {
	return integration.GetInstallationStatus("")
}

// RunSystemInstaller executa a instalação completa do OpenIDM (auto-start no sistema, atalhos com ícone, registro e extensão).
func (a *App) RunSystemInstaller(opts integration.InstallOptions) (*integration.InstallResult, error) {
	if strings.TrimSpace(opts.Language) == "" {
		opts.Language = a.currentLanguage()
	}
	res, err := integration.RunInstallation(opts, func(step int, total int, message string) {
		a.emitEvent("installer_progress", map[string]any{
			"step":    step,
			"total":   total,
			"message": message,
		})
	})
	if err != nil {
		return nil, err
	}
	if cfg, cErr := a.store.GetSettings(); cErr == nil {
		cfg.StartWithWindows = opts.StartWithWindows
		if strings.TrimSpace(opts.Language) != "" {
			cfg.Language = models.NormalizeLanguage(opts.Language)
			if a.tray != nil {
				a.tray.SetLanguage(cfg.Language)
			}
		}
		_ = a.store.SaveSettings(cfg)
	}
	return res, nil
}

// LaunchBrowserExtensionSetup inicia a instalação guiada da extensão nos navegadores instalados e copia o caminho para o Clipboard.
func (a *App) LaunchBrowserExtensionSetup() error {
	status := integration.GetInstallationStatus("")
	return integration.LaunchBrowserExtensionInstaller(status.ExtensionDir, status.ExtensionSetupHTML)
}

// OpenExtensionFolder copia o caminho da extensão para o Clipboard e abre a pasta no Windows Explorer.
func (a *App) OpenExtensionFolder() (string, error) {
	status := integration.GetInstallationStatus("")
	_ = integration.WriteClipboardText(status.ExtensionDir)
	if runtime.GOOS == "windows" && status.ExtensionDir != "" {
		_ = exec.Command("explorer.exe", status.ExtensionDir).Start()
	}
	return status.ExtensionDir, nil
}

// CheckForUpdates consulta o GitHub Releases para verificar se há uma nova versão do OpenIDM disponível.
// Quando manual=false (verificação automática em background), respeita o adiamento de 24 horas escolhido pelo usuário.
func (a *App) CheckForUpdates(manual bool) (*integration.UpdateInfo, error) {
	var postponedUntil int64
	if a.store != nil {
		if cfg, err := a.store.GetSettings(); err == nil {
			postponedUntil = cfg.UpdatePostponedUntil
		}
	}

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	info, err := integration.CheckForUpdate(ctx, "", integration.AppVersion, postponedUntil)
	if err != nil {
		return nil, err
	}

	if info != nil && info.Available && (manual || !info.Postponed) {
		a.emitEvent("update_available", info)
		if !manual {
			a.captureMu.Lock()
			wasHidden := a.windowHidden
			a.captureMu.Unlock()
			if wasHidden {
				if a.tray != nil {
					if a.currentLanguage() == "en-US" {
						a.tray.ShowBalloon(
							"New Update Available — OpenIDM v"+info.LatestVersion,
							"A new release is available on GitHub. Confirm to update now or postpone for 24h.",
						)
					} else {
						a.tray.ShowBalloon(
							"Nova Atualização Disponível — OpenIDM v"+info.LatestVersion,
							"Saiu uma nova versão no GitHub. Confirme para atualizar agora ou adiar por 24h.",
						)
					}
				}
				go a.ShowWindow()
			}
		}
	}
	return info, nil
}

// PostponeUpdate adia as notificações automáticas de atualização por 24 horas a partir do momento atual.
func (a *App) PostponeUpdate() (int64, error) {
	until := time.Now().Add(24 * time.Hour).Unix()
	if a.store != nil {
		cfg, err := a.store.GetSettings()
		if err != nil {
			return until, err
		}
		cfg.UpdatePostponedUntil = until
		if err := a.store.SaveSettings(cfg); err != nil {
			return until, err
		}
	}
	a.emitEvent("update_postponed", map[string]any{"postponedUntil": until})
	return until, nil
}

// StartSelfUpdate baixa o instalador da nova release do GitHub emitindo progresso em tempo real e inicia a atualização.
func (a *App) StartSelfUpdate(downloadURL string, assetName string) (map[string]any, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	cleanURL := strings.TrimSpace(downloadURL)
	cleanAsset := strings.TrimSpace(assetName)
	var releasePageURL string

	if cleanURL == "" {
		info, err := integration.CheckForUpdate(ctx, "", integration.AppVersion, 0)
		if err != nil {
			return nil, err
		}
		if info != nil {
			cleanURL = strings.TrimSpace(info.DownloadURL)
			cleanAsset = strings.TrimSpace(info.AssetName)
			releasePageURL = strings.TrimSpace(info.ReleaseURL)
		}
	}

	// Se a release no GitHub não tiver um instalador .exe anexado diretamente, abre a página da release no navegador.
	if cleanURL == "" {
		if releasePageURL == "" {
			releasePageURL = integration.DefaultGitHubReleasesPage
		}
		_ = a.OpenExternalURL(releasePageURL)
		return map[string]any{
			"started":       false,
			"openedBrowser": true,
			"releaseUrl":    releasePageURL,
		}, nil
	}

	isEN := a.currentLanguage() == "en-US"
	msgDownloading := "Baixando atualização do GitHub..."
	msgApplying := "Iniciando instalador da nova versão..."
	if isEN {
		msgDownloading = "Downloading update from GitHub..."
		msgApplying = "Launching new version installer..."
	}

	a.emitEvent("update_progress", map[string]any{
		"percent":    5,
		"downloaded": 0,
		"total":      0,
		"status":     "downloading",
		"message":    msgDownloading,
	})

	downloadedPath, err := integration.DownloadUpdateAsset(ctx, cleanURL, cleanAsset, func(pct int, dl int64, total int64) {
		a.emitEvent("update_progress", map[string]any{
			"percent":    pct,
			"downloaded": dl,
			"total":      total,
			"status":     "downloading",
			"message":    msgDownloading,
		})
	})
	if err != nil {
		return nil, err
	}

	a.emitEvent("update_progress", map[string]any{
		"percent":    100,
		"downloaded": 0,
		"total":      0,
		"status":     "applying",
		"message":    msgApplying,
	})

	st := integration.GetInstallationStatus("")
	if err := integration.LaunchDownloadedUpdate(downloadedPath, st.InstallDir); err != nil {
		return nil, err
	}

	// Encerra a instância atual após um breve intervalo para permitir que o instalador substitua o binário e reinicie o app.
	go func() {
		time.Sleep(500 * time.Millisecond)
		a.QuitApp()
	}()

	return map[string]any{
		"started":        true,
		"downloadedPath": downloadedPath,
	}, nil
}

// OpenExternalURL abre uma URL externa (ex: página de release no GitHub) no navegador padrão do sistema.
func (a *App) OpenExternalURL(targetURL string) error {
	clean := strings.TrimSpace(targetURL)
	if !strings.HasPrefix(strings.ToLower(clean), "http://") && !strings.HasPrefix(strings.ToLower(clean), "https://") {
		return nil
	}
	if runtime.GOOS == "windows" {
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", clean).Start()
	}
	return exec.Command("xdg-open", clean).Start()
}

// localizeDefaultCategoryName traduz nomes de categorias padrão do banco quando o idioma ativo é inglês,
// preservando nomes personalizados definidos pelo usuário.
func localizeDefaultCategoryName(id, name string, isEN bool) string {
	trimmed := strings.TrimSpace(name)
	defaults := map[string]struct {
		pt      string
		en      string
		aliases []string
	}{
		"compressed": {"Comprimidos", "Compressed", []string{"comprimidos", "compactados", "compressed"}},
		"documents":  {"Documentos", "Documents", []string{"documentos", "documents"}},
		"music":      {"Música", "Music", []string{"música", "musica", "músicas", "musicas", "music"}},
		"programs":   {"Programas", "Programs", []string{"programas", "programs"}},
		"video":      {"Vídeo", "Video", []string{"vídeo", "video", "vídeos", "videos"}},
		"general":    {"Geral / Outros", "General / Other", []string{"geral", "geral / outros", "general", "general / other"}},
	}
	entry, ok := defaults[id]
	if !ok {
		return name
	}
	lower := strings.ToLower(trimmed)
	matched := trimmed == "" || lower == strings.ToLower(id)
	if !matched {
		for _, al := range entry.aliases {
			if lower == al {
				matched = true
				break
			}
		}
	}
	if !matched {
		return name
	}
	if isEN {
		return entry.en
	}
	return entry.pt
}


