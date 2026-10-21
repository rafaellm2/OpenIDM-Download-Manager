package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"openidm/internal/engine"
	"openidm/internal/models"
	"openidm/internal/scheduler"
	"openidm/internal/store"
)

const DefaultBridgeAddr = "127.0.0.1:41820"

// SSEEvent encapsula eventos enviados em tempo real para o aplicativo e extensão.
type SSEEvent struct {
	Type    string `json:"type"` // "download_update", "clipboard_url", "captured_url", "capture_prompt"
	Payload any    `json:"payload"`
}

// BridgeServer expõe uma ponte IPC local (127.0.0.1:41820) exclusivamente para comunicação com
// o Native Messaging Host e Extensão do Navegador, sem hospedar interface web no navegador.
type BridgeServer struct {
	store             *store.Store
	manager           *engine.Manager
	scheduler         *scheduler.Scheduler
	frontendFS        fs.FS
	clientsMu         sync.Mutex
	clients           map[chan SSEEvent]struct{}
	httpSrv           *http.Server
	onCapturePrompt   func(msg NativeMessagePayload)
	onOpenApp         func()
	onSettingsUpdated func(cfg models.AppSettings)
	diagMu            sync.Mutex
	lastExtOrigin     string
	lastExtPingAt     time.Time
	lastModalURL      string
	lastModalFolders  int
	lastModalAt       time.Time
	lastFrontendError string
}

// NewBridgeServer configura a ponte local de IPC para integração com navegadores.
func NewBridgeServer(st *store.Store, mgr *engine.Manager, sched *scheduler.Scheduler, frontendFS fs.FS) *BridgeServer {
	return &BridgeServer{
		store:      st,
		manager:    mgr,
		scheduler:  sched,
		frontendFS: frontendFS,
		clients:    make(map[chan SSEEvent]struct{}),
	}
}

// RecordModalOpened registra que a tela AddUrlModal abriu e renderizou com sucesso na janela nativa.
func (b *BridgeServer) RecordModalOpened(url string, foldersCount int) {
	b.diagMu.Lock()
	defer b.diagMu.Unlock()
	b.lastModalURL = url
	b.lastModalFolders = foldersCount
	b.lastModalAt = time.Now()
}

// RecordFrontendError registra qualquer exceção não tratada do WebView2 para diagnóstico.
func (b *BridgeServer) RecordFrontendError(errMsg string) {
	b.diagMu.Lock()
	defer b.diagMu.Unlock()
	b.lastFrontendError = errMsg
}

// SetCapturePromptHandler define o callback acionado quando um link é capturado no navegador,
// trazendo a janela nativa do OpenIDM para frente já com a tela de salvar na organização.
func (b *BridgeServer) SetCapturePromptHandler(fn func(msg NativeMessagePayload)) {
	b.onCapturePrompt = fn
}

// SetOpenAppHandler define o callback para restaurar/focar a janela nativa do Windows.
func (b *BridgeServer) SetOpenAppHandler(fn func()) {
	b.onOpenApp = fn
}

// SetSettingsUpdatedHandler define o callback acionado quando as configurações globais são alteradas via ponte HTTP.
func (b *BridgeServer) SetSettingsUpdatedHandler(fn func(cfg models.AppSettings)) {
	b.onSettingsUpdated = fn
}

// Start inicia o listener IPC em background no endereço especificado (default 127.0.0.1:41820).
func (b *BridgeServer) Start(addr string) error {
	if addr == "" {
		addr = DefaultBridgeAddr
	}
	mux := http.NewServeMux()

	// Rotas IPC da extensão de navegador e native host
	mux.HandleFunc("/api/health", b.withCORS(b.handleHealth))
	mux.HandleFunc("/api/events", b.withCORS(b.handleSSE))
	mux.HandleFunc("/api/downloads", b.withCORS(b.handleDownloads))
	mux.HandleFunc("/api/downloads/action", b.withCORS(b.handleDownloadAction))
	mux.HandleFunc("/api/categories", b.withCORS(b.handleCategories))
	mux.HandleFunc("/api/queues", b.withCORS(b.handleQueues))
	mux.HandleFunc("/api/queues/action", b.withCORS(b.handleQueueAction))
	mux.HandleFunc("/api/settings", b.withCORS(b.handleSettings))
	mux.HandleFunc("/api/language", b.withCORS(b.handleLanguage))
	mux.HandleFunc("/api/site-rules", b.withCORS(b.handleSiteRules))
	mux.HandleFunc("/api/site-logins", b.withCORS(b.handleSiteLogins))
	mux.HandleFunc("/api/classify", b.withCORS(b.handleClassify))
	mux.HandleFunc("/api/capture", b.withCORS(b.handleBrowserCapture))
	mux.HandleFunc("/api/extension-wakeup", b.withCORS(b.handleExtensionWakeup))
	mux.HandleFunc("/api/test-download/", b.withCORS(b.handleTestDownload))
	mux.HandleFunc("/api/installer/status", b.withCORS(b.handleInstallerStatus))
	mux.HandleFunc("/api/installer/install", b.withCORS(b.handleInstallerInstall))
	mux.HandleFunc("/api/installer/open-extension-folder", b.withCORS(b.handleOpenExtensionFolder))
	mux.HandleFunc("/api/installer/launch-extension", b.withCORS(b.handleLaunchExtension))
	mux.HandleFunc("/api/update/check", b.withCORS(b.handleUpdateCheck))
	mux.HandleFunc("/api/update/postpone", b.withCORS(b.handleUpdatePostpone))
	mux.HandleFunc("/api/update/apply", b.withCORS(b.handleUpdateApply))

	b.httpSrv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		_ = b.httpSrv.ListenAndServe()
	}()
	return nil
}

// Stop encerra o servidor HTTP local com segurança.
func (b *BridgeServer) Stop() {
	if b.httpSrv != nil {
		_ = b.httpSrv.Close()
	}
}

// Broadcast envia um evento em tempo real para todas as conexões SSE ativas.
func (b *BridgeServer) Broadcast(ev SSEEvent) {
	b.clientsMu.Lock()
	defer b.clientsMu.Unlock()

	for ch := range b.clients {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (b *BridgeServer) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if strings.HasPrefix(origin, "chrome-extension://") || strings.HasPrefix(origin, "moz-extension://") {
			b.diagMu.Lock()
			b.lastExtOrigin = origin
			b.lastExtPingAt = time.Now()
			b.diagMu.Unlock()
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func (b *BridgeServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	b.diagMu.Lock()
	extOrigin := b.lastExtOrigin
	extPingAt := b.lastExtPingAt
	modalURL := b.lastModalURL
	modalFolders := b.lastModalFolders
	modalAt := b.lastModalAt
	feErr := b.lastFrontendError
	b.diagMu.Unlock()

	lang := "pt-BR"
	if b.store != nil {
		if cfg, err := b.store.GetSettings(); err == nil {
			lang = models.NormalizeLanguage(cfg.Language)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":                "ok",
		"product":               "OpenIDM",
		"version":               AppVersion,
		"language":              lang,
		"foregroundWindowTitle": GetForegroundWindowTitle(),
		"lastExtensionOrigin":   extOrigin,
		"lastExtensionPingAt":   extPingAt,
		"lastModalOpenedUrl":    modalURL,
		"lastModalFoldersCount": modalFolders,
		"lastModalOpenedAt":     modalAt,
		"lastFrontendError":     feErr,
	})
}

func (b *BridgeServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan SSEEvent, 32)
	b.clientsMu.Lock()
	b.clients[ch] = struct{}{}
	b.clientsMu.Unlock()

	defer func() {
		b.clientsMu.Lock()
		delete(b.clients, ch)
		b.clientsMu.Unlock()
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			raw, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", string(raw))
			flusher.Flush()
		}
	}
}

func (b *BridgeServer) handleDownloads(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		id := r.URL.Query().Get("id")
		if id != "" {
			dl, err := b.manager.GetDownload(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, dl)
			return
		}
		writeJSON(w, http.StatusOK, b.manager.ListDownloads())

	case http.MethodPost:
		var req models.AddDownloadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		dl, err := b.manager.AddDownload(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, dl)
	}
}

func (b *BridgeServer) handleDownloadAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Action        string `json:"action"` // "start", "pause", "pause_all", "cancel", "delete", "delete_completed", "open_file", "open_folder", "speed_limit"
		ID            string `json:"id"`
		DeleteFile    bool   `json:"deleteFile"`
		SpeedLimitBps int64  `json:"speedLimitBps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var err error
	switch payload.Action {
	case "start":
		err = b.manager.StartDownload(payload.ID)
	case "pause":
		err = b.manager.PauseDownload(payload.ID)
	case "pause_all":
		b.manager.PauseAll()
	case "cancel":
		err = b.manager.CancelDownload(payload.ID)
	case "delete":
		err = b.manager.DeleteDownload(payload.ID, payload.DeleteFile)
	case "delete_completed":
		err = b.manager.DeleteCompleted()
	case "speed_limit":
		err = b.manager.SetDownloadSpeedLimit(payload.ID, payload.SpeedLimitBps)
	case "open_file":
		if dl, gErr := b.manager.GetDownload(payload.ID); gErr == nil {
			err = engine.OpenFileInOS(dl.TargetPath)
		} else {
			err = gErr
		}
	case "open_folder":
		if dl, gErr := b.manager.GetDownload(payload.ID); gErr == nil {
			err = engine.OpenFolderInOS(dl.TargetPath)
		} else {
			err = gErr
		}
	}

	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (b *BridgeServer) handleCategories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cats, err := b.store.ListCategories()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, cats)
	case http.MethodPost:
		var c models.Category
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := b.store.SaveCategory(c); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, c)
	}
}

func (b *BridgeServer) handleQueues(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		queues, err := b.store.ListQueues()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, queues)
	case http.MethodPost:
		var q models.Queue
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if q.ID == "" {
			q.ID = fmt.Sprintf("queue_%d", time.Now().UnixMilli())
		}
		if err := b.store.SaveQueue(q); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, q)
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if err := b.store.DeleteQueue(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (b *BridgeServer) handleQueueAction(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Action  string `json:"action"` // "start", "stop", "cancel_shutdown"
		QueueID string `json:"queueId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var err error
	switch payload.Action {
	case "start":
		err = b.scheduler.StartQueue(payload.QueueID)
	case "stop":
		err = b.scheduler.StopQueue(payload.QueueID)
	case "cancel_shutdown":
		err = scheduler.CancelSystemShutdown()
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (b *BridgeServer) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := b.store.GetSettings()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPost:
		var cfg models.AppSettings
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		cfg.Language = models.NormalizeLanguage(cfg.Language)
		if err := b.store.SaveSettings(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		_ = SetAutoStartWithWindows("", cfg.StartWithWindows, true)
		b.manager.RefreshGlobalLimiter(cfg.GlobalSpeedLimitBps)
		if b.onSettingsUpdated != nil {
			b.onSettingsUpdated(cfg)
		}
		b.Broadcast(SSEEvent{
			Type:    "language_changed",
			Payload: map[string]string{"language": cfg.Language},
		})
		writeJSON(w, http.StatusOK, cfg)
	}
}

func (b *BridgeServer) handleLanguage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		lang := models.LangPtBR
		if b.store != nil {
			if cfg, err := b.store.GetSettings(); err == nil {
				lang = models.NormalizeLanguage(cfg.Language)
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"language": lang})
	case http.MethodPost:
		var req struct {
			Language string `json:"language"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		norm := models.NormalizeLanguage(req.Language)
		if b.store != nil {
			if cfg, err := b.store.GetSettings(); err == nil {
				cfg.Language = norm
				if err := b.store.SaveSettings(cfg); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				if b.onSettingsUpdated != nil {
					b.onSettingsUpdated(cfg)
				}
			}
		}
		b.Broadcast(SSEEvent{
			Type:    "language_changed",
			Payload: map[string]string{"language": norm},
		})
		writeJSON(w, http.StatusOK, map[string]string{"language": norm})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (b *BridgeServer) handleSiteRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rules, err := b.store.ListSiteRules()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rules)
	case http.MethodPost:
		var rule models.SiteRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := b.store.SaveSiteRule(rule); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		_ = b.store.DeleteSiteRule(id)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (b *BridgeServer) handleSiteLogins(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		logins, err := b.store.ListSiteLogins()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, logins)
	case http.MethodPost:
		var login models.SiteLogin
		if err := json.NewDecoder(r.Body).Decode(&login); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := b.store.SaveSiteLogin(login); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		_ = b.store.DeleteSiteLogin(id)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (b *BridgeServer) handleClassify(w http.ResponseWriter, r *http.Request) {
	rawURL := engine.NormalizeDownloadURL(r.URL.Query().Get("url"))
	customName := r.URL.Query().Get("fileName")
	cats, _ := b.store.ListCategories()
	cfg, _ := b.store.GetSettings()
	fileName, catID, saveDir := SuggestCategoryAndDir(rawURL, customName, cats, cfg.DefaultSaveDir)
	writeJSON(w, http.StatusOK, map[string]string{
		"fileName":   fileName,
		"categoryId": catID,
		"saveDir":    saveDir,
	})
}

func (b *BridgeServer) handleBrowserCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var msg NativeMessagePayload
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		writeJSON(w, http.StatusBadRequest, NativeMessageResponse{Success: false, Reason: err.Error()})
		return
	}

	settings, _ := b.store.GetSettings()
	curLang := models.NormalizeLanguage(settings.Language)

	if msg.Action == "set_language" && strings.TrimSpace(msg.Language) != "" {
		norm := models.NormalizeLanguage(msg.Language)
		settings.Language = norm
		_ = b.store.SaveSettings(settings)
		if b.onSettingsUpdated != nil {
			b.onSettingsUpdated(settings)
		}
		b.Broadcast(SSEEvent{
			Type:    "language_changed",
			Payload: map[string]string{"language": norm},
		})
		writeJSON(w, http.StatusOK, NativeMessageResponse{
			Success:  true,
			Version:  AppVersion,
			Language: norm,
		})
		return
	}

	if msg.Action == "ping" {
		if msg.FileName != "" {
			b.diagMu.Lock()
			b.lastExtOrigin = msg.FileName
			b.lastExtPingAt = time.Now()
			b.diagMu.Unlock()
		}
		writeJSON(w, http.StatusOK, NativeMessageResponse{
			Success:  true,
			Version:  AppVersion,
			Language: curLang,
		})
		return
	}

	if msg.Action == "open_app" {
		if b.onOpenApp != nil {
			b.onOpenApp()
		}
		writeJSON(w, http.StatusOK, NativeMessageResponse{
			Success:  true,
			Captured: true,
			Version:  AppVersion,
			Language: curLang,
		})
		return
	}

	msg.URL = engine.NormalizeDownloadURL(msg.URL)
	if strings.Contains(msg.URL, "/api/extension-wakeup") {
		writeJSON(w, http.StatusOK, NativeMessageResponse{
			Success:  true,
			Captured: true,
			Version:  AppVersion,
			Language: curLang,
		})
		return
	}

	rules, _ := b.store.ListSiteRules()

	if msg.Action == "check" {
		ok, reason := ShouldCaptureBrowserDownload(msg.URL, msg.FileName, msg.Referrer, msg.ModifierKey, settings, rules)
		writeJSON(w, http.StatusOK, NativeMessageResponse{
			Success:  true,
			Captured: ok,
			Reason:   reason,
			Version:  AppVersion,
			Language: curLang,
		})
		return
	}

	// Verifica se não está ignorado por regra de exceção do site, página .html ou tecla modificadora (ALT)
	if ok, reason := ShouldCaptureBrowserDownload(msg.URL, msg.FileName, msg.Referrer, msg.ModifierKey, settings, rules); !ok {
		writeJSON(w, http.StatusOK, NativeMessageResponse{
			Success:  true,
			Captured: false,
			Reason:   reason,
			Version:  AppVersion,
			Language: curLang,
		})
		return
	}

	// Ao capturar o link, abre a janela nativa do OpenIDM perguntando onde salvar dentro da organização de categorias.
	if b.onCapturePrompt != nil {
		b.onCapturePrompt(msg)
		writeJSON(w, http.StatusOK, NativeMessageResponse{
			Success:  true,
			Captured: true,
			Version:  AppVersion,
			Language: curLang,
		})
		return
	}

	dl, err := b.manager.AddDownload(models.AddDownloadRequest{
		URL:         msg.URL,
		Referrer:    msg.Referrer,
		FileName:    msg.FileName,
		Description: "Capturado do navegador (" + msg.Referrer + ")",
		StartNow:    msg.StartNow,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, NativeMessageResponse{Success: false, Reason: err.Error(), Language: curLang})
		return
	}

	b.Broadcast(SSEEvent{
		Type:    "captured_url",
		Payload: dl,
	})

	writeJSON(w, http.StatusOK, NativeMessageResponse{
		Success:    true,
		Captured:   true,
		DownloadID: dl.ID,
		Version:    AppVersion,
		Language:   curLang,
	})
}

// handleExtensionWakeup dispara um evento downloads.onCreated efêmero no navegador para acordar e recarregar o Service Worker da extensão.
func (b *BridgeServer) handleExtensionWakeup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="openidm-sync.bin"`)
	w.Header().Set("Content-Length", "65536")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(make([]byte, 64))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	select {
	case <-r.Context().Done():
	case <-time.After(1500 * time.Millisecond):
	}
}

// handleTestDownload serve um arquivo binário de 1 MB com suporte completo a HTTP Range (206 Partial Content)
// para validar tanto a interceptação pelo navegador quanto o download segmentado completo pelo OpenIDM.
func (b *BridgeServer) handleTestDownload(w http.ResponseWriter, r *http.Request) {
	fileName := strings.TrimPrefix(r.URL.Path, "/api/test-download/")
	if fileName == "" {
		fileName = "instalador_openidm_teste.zip"
	}

	const totalSize = 1048576 // 1 MB
	payload := make([]byte, totalSize)
	for i := range payload {
		payload[i] = byte((i*31 + 17) & 0xFF)
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, fileName))

	// Quando requisitado com cabeçalho Range (pelo motor do OpenIDM) ou HEAD, utiliza http.ServeContent padrão.
	if r.Method == http.MethodHead || r.Header.Get("Range") != "" {
		http.ServeContent(w, r, fileName, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), bytes.NewReader(payload))
		return
	}

	// Em GET sem Range (ex: clique inicial no navegador), envia os primeiros 64 bytes imediatamente e aguarda 600ms
	// para que o chrome.downloads intercepte e cancele o download nativo antes de transmitir o restante.
	w.Header().Set("Content-Length", strconv.Itoa(totalSize))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload[:64])
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	select {
	case <-r.Context().Done():
		return
	case <-time.After(600 * time.Millisecond):
	}
	_, _ = w.Write(payload[64:])
}

func (b *BridgeServer) handleInstallerStatus(w http.ResponseWriter, r *http.Request) {
	status := GetInstallationStatus("")
	writeJSON(w, http.StatusOK, status)
}

func (b *BridgeServer) handleInstallerInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var opts InstallOptions
	_ = json.NewDecoder(r.Body).Decode(&opts)
	if strings.TrimSpace(opts.Language) == "" && b.store != nil {
		if cfg, err := b.store.GetSettings(); err == nil {
			opts.Language = cfg.Language
		}
	}
	res, err := RunInstallation(opts, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if b.store != nil {
		if cfg, cErr := b.store.GetSettings(); cErr == nil {
			cfg.StartWithWindows = opts.StartWithWindows
			if strings.TrimSpace(opts.Language) != "" {
				cfg.Language = models.NormalizeLanguage(opts.Language)
			}
			_ = b.store.SaveSettings(cfg)
			if b.onSettingsUpdated != nil {
				b.onSettingsUpdated(cfg)
			}
		}
	}
	writeJSON(w, http.StatusOK, res)
}

func (b *BridgeServer) handleOpenExtensionFolder(w http.ResponseWriter, r *http.Request) {
	status := GetInstallationStatus("")
	_ = WriteClipboardText(status.ExtensionDir)
	if runtime.GOOS == "windows" && status.ExtensionDir != "" {
		_ = exec.Command("explorer.exe", status.ExtensionDir).Start()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"extensionDir": status.ExtensionDir,
	})
}

func (b *BridgeServer) handleLaunchExtension(w http.ResponseWriter, r *http.Request) {
	status := GetInstallationStatus("")
	_ = LaunchBrowserExtensionInstaller(status.ExtensionDir, status.ExtensionSetupHTML)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"extensionDir": status.ExtensionDir,
	})
}

func (b *BridgeServer) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	manual := r.URL.Query().Get("manual") == "true" || r.URL.Query().Get("manual") == "1"
	var postponedUntil int64
	if b.store != nil {
		if cfg, err := b.store.GetSettings(); err == nil {
			postponedUntil = cfg.UpdatePostponedUntil
		}
	}
	info, err := CheckForUpdate(r.Context(), "", AppVersion, postponedUntil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if manual {
		info.Postponed = false
	}
	writeJSON(w, http.StatusOK, info)
}

func (b *BridgeServer) handleUpdatePostpone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	until := time.Now().Add(24 * time.Hour).Unix()
	if b.store != nil {
		if cfg, err := b.store.GetSettings(); err == nil {
			cfg.UpdatePostponedUntil = until
			_ = b.store.SaveSettings(cfg)
			if b.onSettingsUpdated != nil {
				b.onSettingsUpdated(cfg)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                   true,
		"updatePostponedUntil": until,
	})
}

func (b *BridgeServer) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		DownloadURL string `json:"downloadUrl"`
		AssetName   string `json:"assetName"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var postponedUntil int64
	if b.store != nil {
		if cfg, err := b.store.GetSettings(); err == nil {
			postponedUntil = cfg.UpdatePostponedUntil
		}
	}

	dlURL := strings.TrimSpace(req.DownloadURL)
	assetName := strings.TrimSpace(req.AssetName)
	if dlURL == "" {
		info, err := CheckForUpdate(r.Context(), "", AppVersion, postponedUntil)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		dlURL = info.DownloadURL
		assetName = info.AssetName
		if dlURL == "" && info.ReleaseURL != "" {
			writeJSON(w, http.StatusOK, map[string]any{
				"status":     "opened_release_page",
				"releaseUrl": info.ReleaseURL,
			})
			return
		}
	}

	b.Broadcast(SSEEvent{
		Type: "update_progress",
		Payload: map[string]any{
			"percent": 1,
			"status":  "downloading",
		},
	})

	filePath, err := DownloadUpdateAsset(r.Context(), dlURL, assetName, func(pct int, dl int64, total int64) {
		b.Broadcast(SSEEvent{
			Type: "update_progress",
			Payload: map[string]any{
				"percent":    pct,
				"downloaded": dl,
				"total":      total,
				"status":     "downloading",
			},
		})
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	installDir := DefaultInstallDir()
	if st := GetInstallationStatus(""); st.InstallDir != "" {
		installDir = st.InstallDir
	}

	if err := LaunchDownloadedUpdate(filePath, installDir); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "started",
		"filePath": filePath,
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

