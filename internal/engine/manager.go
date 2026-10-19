package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"openidm/internal/models"
	"openidm/internal/store"
	"openidm/internal/sysproc"
)

// UpdateCallback é invocado sempre que um download sofre alteração de progresso ou status.
type UpdateCallback func(d models.Download)

// Manager coordena todos os downloads ativos, limites de banda, eventos e persistência no SQLite.
type Manager struct {
	mu            sync.RWMutex
	store         *store.Store
	downloads     map[string]*models.Download
	cancels       map[string]context.CancelFunc
	doneChs       map[string]chan struct{}
	localLimiters map[string]*SpeedLimiter
	globalLimiter *SpeedLimiter
	listeners     []UpdateCallback
}

// NewManager inicializa o gerenciador carregando downloads anteriores do SQLite.
func NewManager(st *store.Store) (*Manager, error) {
	settings, err := st.GetSettings()
	if err != nil {
		return nil, err
	}

	m := &Manager{
		store:         st,
		downloads:     make(map[string]*models.Download),
		cancels:       make(map[string]context.CancelFunc),
		doneChs:       make(map[string]chan struct{}),
		localLimiters: make(map[string]*SpeedLimiter),
		globalLimiter: NewSpeedLimiter(settings.GlobalSpeedLimitBps),
	}

	existing, err := st.ListDownloads()
	if err != nil {
		return nil, err
	}

	for i := range existing {
		d := existing[i]
		// Se o processo anterior fechou abruptamente durante um download, coloca em Paused pronto para retomar do último checkpoint!
		if d.Status == models.StatusDownloading || d.Status == models.StatusProbing || d.Status == models.StatusAssembling {
			d.Status = models.StatusPaused
			d.SpeedBps = 0
			d.ActiveWorkers = 0
			_ = st.SaveDownload(&d)
		}
		cp := d
		m.downloads[d.ID] = &cp
	}

	return m, nil
}

// OnUpdate registra um ouvinte para receber atualizações em tempo real dos downloads.
func (m *Manager) OnUpdate(cb UpdateCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, cb)
}

// RefreshGlobalLimiter atualiza a velocidade máxima global após alteração nas configurações.
func (m *Manager) RefreshGlobalLimiter(rateBps int64) {
	m.globalLimiter.SetRate(rateBps)
}

// SetDownloadSpeedLimit altera em tempo real o limitador de velocidade de um download específico.
func (m *Manager) SetDownloadSpeedLimit(id string, rateBps int64) error {
	m.mu.Lock()
	d, ok := m.downloads[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("download não encontrado")
	}
	d.SpeedLimitBps = rateBps
	lim := m.localLimiters[id]
	m.mu.Unlock()

	if lim != nil {
		lim.SetRate(rateBps)
	}
	m.persistSnapshot(id)
	m.emitUpdate(id)
	return nil
}

// AddDownload cria um novo download, classifica automaticamente a categoria e inicia se solicitado.
func (m *Manager) AddDownload(req models.AddDownloadRequest) (*models.Download, error) {
	// Normaliza links de visualização (/blob/, Google Drive, Dropbox) para a URL direta do binário.
	rawURL := NormalizeDownloadURL(strings.TrimSpace(req.URL))
	if rawURL == "" {
		return nil, errors.New("a URL do download não pode estar vazia")
	}

	settings, _ := m.store.GetSettings()
	cats, _ := m.store.ListCategories()

	fileName := SanitizeDownloadFileName(req.FileName)
	if fileName == "" {
		fileName = ExtractFileNameFromURL(rawURL)
	}

	catID := req.CategoryID
	saveDir := strings.TrimSpace(req.SaveDir)
	if catID == "" || saveDir == "" {
		matchedCat := matchCategoryByFileName(fileName, cats)
		if catID == "" {
			catID = matchedCat.ID
		}
		if saveDir == "" {
			saveDir = matchedCat.DefaultDir
		}
	}
	if saveDir == "" {
		saveDir = settings.DefaultSaveDir
	}

	queueID := req.QueueID
	if queueID == "" {
		queueID = "main"
	}

	maxConns := req.MaxConnections
	if maxConns <= 0 {
		maxConns = settings.MaxConnectionsPerDownload
	}

	// Se o usuário informou login no diálogo de Adicionar URL, salva nas credenciais de site.
	if req.Username != "" {
		host := extractHost(rawURL)
		if host != "" {
			authType := "basic"
			if strings.HasPrefix(strings.ToLower(rawURL), "ftp://") {
				authType = "ftp"
			}
			_ = m.store.SaveSiteLogin(models.SiteLogin{
				HostPattern: host,
				Username:    req.Username,
				Password:    req.Password,
				AuthType:    authType,
			})
		}
	}

	dl := &models.Download{
		ID:             generateID(),
		URL:            rawURL,
		Referrer:       strings.TrimSpace(req.Referrer),
		FileName:       fileName,
		SaveDir:        saveDir,
		TargetPath:     filepath.Join(saveDir, fileName),
		Description:    req.Description,
		CategoryID:     catID,
		QueueID:        queueID,
		Status:         models.StatusQueued,
		MaxConnections: maxConns,
		SpeedLimitBps:  req.SpeedLimitBps,
		OpenOnComplete: req.OpenOnComplete,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := m.store.SaveDownload(dl); err != nil {
		return nil, err
	}

	m.mu.Lock()
	cp := *dl
	m.downloads[dl.ID] = &cp
	m.mu.Unlock()

	m.emitUpdate(dl.ID)

	if req.StartNow {
		_ = m.StartDownload(dl.ID)
	}

	return m.GetDownload(dl.ID)
}

// StartDownload inicia ou retoma (resume) um download a partir do último checkpoint.
func (m *Manager) StartDownload(id string) error {
	m.mu.Lock()
	d, ok := m.downloads[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("download não encontrado")
	}
	if _, active := m.cancels[id]; active {
		m.mu.Unlock()
		return nil // Já está rodando
	}
	if d.Status == models.StatusCompleted || d.Status == models.StatusCancelled {
		// Se o usuário ou fila de sincronização pediu para reiniciar, reseta segmentos e tamanho.
		d.Segments = nil
		d.DownloadedSize = 0
		d.Progress = 0
		d.TotalSize = 0
		d.DegradedSingle = false
		d.DegradedReason = ""
	}

	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan struct{})
	m.cancels[id] = cancel
	m.doneChs[id] = doneCh
	d.Status = models.StatusProbing
	d.ErrorMessage = ""
	dlCopy := *d
	m.mu.Unlock()

	m.emitUpdate(id)

	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.cancels, id)
			delete(m.doneChs, id)
			delete(m.localLimiters, id)
			close(doneCh)
			m.mu.Unlock()
		}()
		m.runDownloadTask(ctx, &dlCopy)
	}()

	return nil
}

// PauseDownload pausa imediatamente as conexões do download e salva o checkpoint dos segmentos.
func (m *Manager) PauseDownload(id string) error {
	m.mu.Lock()
	d, ok := m.downloads[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("download não encontrado")
	}
	cancel, active := m.cancels[id]
	doneCh := m.doneChs[id]
	d.Status = models.StatusPaused
	d.SpeedBps = 0
	d.ActiveWorkers = 0
	m.appendEventLocked(d, 0, "info", "Download pausado pelo usuário. Posições salvas no checkpoint.")
	m.mu.Unlock()

	if active && cancel != nil {
		cancel()
		if doneCh != nil {
			select {
			case <-doneCh:
			case <-time.After(5 * time.Second):
			}
		}
	}
	m.persistSnapshot(id)
	m.emitUpdate(id)
	return nil
}

// PauseAll pausa todos os downloads em andamento.
func (m *Manager) PauseAll() {
	m.mu.RLock()
	var activeIDs []string
	for id := range m.cancels {
		activeIDs = append(activeIDs, id)
	}
	m.mu.RUnlock()

	for _, id := range activeIDs {
		_ = m.PauseDownload(id)
	}
}

// CancelDownload interrompe o download e define o status como cancelado.
func (m *Manager) CancelDownload(id string) error {
	m.mu.Lock()
	d, ok := m.downloads[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("download não encontrado")
	}
	cancel, active := m.cancels[id]
	doneCh := m.doneChs[id]
	d.Status = models.StatusCancelled
	d.SpeedBps = 0
	d.ActiveWorkers = 0
	m.appendEventLocked(d, 0, "warn", "Download parado/cancelado.")
	m.mu.Unlock()

	if active && cancel != nil {
		cancel()
		if doneCh != nil {
			select {
			case <-doneCh:
			case <-time.After(5 * time.Second):
			}
		}
	}
	m.persistSnapshot(id)
	m.emitUpdate(id)
	return nil
}

// DeleteDownload remove o download imediatamente da memória e do banco (e opcionalmente o arquivo em disco),
// cancelando workers ativos sem emitir estado intermediário "cancelled" nem forçar sync desnecessário.
func (m *Manager) DeleteDownload(id string, deleteFile bool) error {
	m.mu.Lock()
	d, ok := m.downloads[id]
	var targetPath string
	var cancel context.CancelFunc
	var doneCh chan struct{}
	var active bool
	if ok {
		targetPath = d.TargetPath
		cancel, active = m.cancels[id]
		doneCh = m.doneChs[id]
		delete(m.downloads, id)
		delete(m.cancels, id)
		delete(m.doneChs, id)
		delete(m.localLimiters, id)
	}
	m.mu.Unlock()

	if active && cancel != nil {
		cancel()
		if doneCh != nil {
			select {
			case <-doneCh:
			case <-time.After(3 * time.Second):
			}
		}
	}

	if targetPath != "" {
		_ = os.Remove(targetPath + ".oidmpart")
		_ = os.Remove(targetPath + ".tdmpart")
		if deleteFile {
			_ = os.Remove(targetPath)
		}
	}
	return m.store.DeleteDownload(id)
}

func (m *Manager) isDownloadExists(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.downloads[id]
	return ok
}

// DeleteCompleted remove da lista todos os downloads concluídos.
func (m *Manager) DeleteCompleted() error {
	m.mu.Lock()
	for id, d := range m.downloads {
		if d.Status == models.StatusCompleted {
			delete(m.downloads, id)
		}
	}
	m.mu.Unlock()
	return m.store.DeleteCompletedDownloads()
}

// GetDownload retorna uma cópia thread-safe de um download.
func (m *Manager) GetDownload(id string) (*models.Download, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	d, ok := m.downloads[id]
	if !ok {
		return nil, errors.New("download não encontrado")
	}
	cp := copyDownload(d)
	return &cp, nil
}

// ListDownloads retorna todos os downloads em memória ordenados por data de criação decrescente.
func (m *Manager) ListDownloads() []models.Download {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]models.Download, 0, len(m.downloads))
	for _, d := range m.downloads {
		out = append(out, copyDownload(d))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// ActiveCountForQueue retorna quantos downloads estão em andamento para uma fila específica.
func (m *Manager) ActiveCountForQueue(queueID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for id, d := range m.downloads {
		if d.QueueID == queueID {
			if _, running := m.cancels[id]; running {
				count++
			}
		}
	}
	return count
}

func (m *Manager) setLocalLimiter(id string, lim *SpeedLimiter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.localLimiters[id] = lim
}

func (m *Manager) updateDownloadState(id string, fn func(d *models.Download)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d, ok := m.downloads[id]; ok {
		fn(d)
		d.UpdatedAt = time.Now()
	}
}

func (m *Manager) appendEventLocked(d *models.Download, workerID int, level, msg string) {
	ev := models.ConnectionEvent{
		Timestamp: time.Now(),
		WorkerID:  workerID,
		Level:     level,
		Message:   msg,
	}
	d.Events = append(d.Events, ev)
	if len(d.Events) > 80 {
		d.Events = d.Events[len(d.Events)-80:]
	}
}

func (m *Manager) logConnectionEvent(id string, workerID int, level, msg string) {
	m.updateDownloadState(id, func(d *models.Download) {
		m.appendEventLocked(d, workerID, level, msg)
	})
}

func (m *Manager) handleTaskFailure(id string, errMsg string) {
	m.updateDownloadState(id, func(d *models.Download) {
		d.Status = models.StatusError
		d.ErrorMessage = errMsg
		d.RetryCount++
		d.SpeedBps = 0
		d.ActiveWorkers = 0
		m.appendEventLocked(d, 0, "error", errMsg)
	})
	m.persistSnapshot(id)
	m.emitUpdate(id)
}

func (m *Manager) persistSnapshot(id string) {
	dl, err := m.GetDownload(id)
	if err != nil {
		return
	}
	_ = m.store.SaveDownload(dl)
}

func (m *Manager) emitUpdate(id string) {
	dl, err := m.GetDownload(id)
	if err != nil {
		return
	}
	m.mu.RLock()
	cbs := make([]UpdateCallback, len(m.listeners))
	copy(cbs, m.listeners)
	m.mu.RUnlock()

	for _, cb := range cbs {
		cb(*dl)
	}
}

// FindLoginForURL busca a credencial de servidor correspondente ao domínio da URL (suporta wildcards).
func (m *Manager) FindLoginForURL(rawURL string) *models.SiteLogin {
	return m.findLoginForURL(rawURL)
}

func (m *Manager) findLoginForURL(rawURL string) *models.SiteLogin {
	host := strings.ToLower(extractHost(rawURL))
	if host == "" {
		return nil
	}
	logins, err := m.store.ListSiteLogins()
	if err != nil {
		return nil
	}
	for _, l := range logins {
		if MatchHostWildcard(l.HostPattern, host) {
			cp := l
			return &cp
		}
	}
	return nil
}

func (m *Manager) findRuleForURL(rawURL string) *models.SiteRule {
	host := strings.ToLower(extractHost(rawURL))
	if host == "" {
		return nil
	}
	rules, err := m.store.ListSiteRules()
	if err != nil {
		return nil
	}
	for _, r := range rules {
		if MatchHostWildcard(r.HostPattern, host) {
			cp := r
			return &cp
		}
	}
	return nil
}

// ParseCommandArgs divide uma string de parâmetros respeitando argumentos entre aspas e substituindo [file] com segurança.
func ParseCommandArgs(rawArgs string, targetFilePath string) []string {
	var tokens []string
	var current strings.Builder
	inQuotes := false
	var quoteChar rune

	for _, r := range rawArgs {
		switch {
		case !inQuotes && (r == '"' || r == '\''):
			inQuotes = true
			quoteChar = r
		case inQuotes && r == quoteChar:
			inQuotes = false
		case !inQuotes && (r == ' ' || r == '\t'):
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	if targetFilePath != "" {
		replaced := false
		for i := range tokens {
			if strings.Contains(tokens[i], "[file]") {
				tokens[i] = strings.ReplaceAll(tokens[i], "[file]", targetFilePath)
				replaced = true
			}
		}
		if !replaced && len(tokens) == 0 {
			tokens = []string{targetFilePath}
		}
	}
	return tokens
}

// runPostDownloadActions executa a varredura de antivírus opcional e abertura automática do arquivo.
func (m *Manager) runPostDownloadActions(id string) {
	dl, err := m.GetDownload(id)
	if err != nil {
		return
	}
	settings, _ := m.store.GetSettings()

	// 1. Varredura por antivírus externo configurado.
	if settings.AntivirusEnabled && strings.TrimSpace(settings.AntivirusPath) != "" {
		args := ParseCommandArgs(settings.AntivirusArgs, dl.TargetPath)
		if len(args) == 0 {
			args = []string{dl.TargetPath}
		}
		m.logConnectionEvent(id, 0, "info", "Executando verificação de antivírus no arquivo concluído...")
		m.emitUpdate(id)

		cmd := sysproc.HiddenCommand(settings.AntivirusPath, args...)
		if err := cmd.Run(); err != nil {
			m.logConnectionEvent(id, 0, "warn", fmt.Sprintf("Antivírus retornou aviso/erro: %v", err))
		} else {
			m.logConnectionEvent(id, 0, "info", "Verificação de antivírus concluída: arquivo limpo.")
		}
		m.persistSnapshot(id)
		m.emitUpdate(id)
	}

	// 2. Abrir arquivo automaticamente ao concluir, se marcado pelo usuário.
	if dl.OpenOnComplete {
		_ = OpenFileInOS(dl.TargetPath)
	}
}

// OpenFileInOS abre um arquivo ou pasta usando o shell nativo do sistema operacional.
func OpenFileInOS(targetPath string) error {
	if runtime.GOOS == "windows" {
		return sysproc.HiddenCommand("cmd", "/c", "start", "", targetPath).Start()
	}
	return exec.Command("xdg-open", targetPath).Start()
}

// OpenFolderInOS abre o Windows Explorer destacando o arquivo baixado.
func OpenFolderInOS(targetPath string) error {
	if runtime.GOOS == "windows" {
		return exec.Command("explorer.exe", "/select,", filepath.Clean(targetPath)).Start()
	}
	return exec.Command("xdg-open", filepath.Dir(targetPath)).Start()
}

// MatchHostWildcard compara um host com um padrão que pode conter wildcard (ex: *.exemplo.com).
func MatchHostWildcard(pattern, host string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	if pattern == "" || host == "" {
		return false
	}
	if pattern == "*" || pattern == host {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*.")
		return host == suffix || strings.HasSuffix(host, "."+suffix)
	}
	matched, _ := filepath.Match(pattern, host)
	return matched
}

func matchCategoryByFileName(fileName string, categories []models.Category) models.Category {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
	for _, c := range categories {
		for _, candidate := range c.Extensions {
			if strings.EqualFold(candidate, ext) {
				return c
			}
		}
	}
	for _, c := range categories {
		if c.ID == "general" {
			return c
		}
	}
	if len(categories) > 0 {
		return categories[0]
	}
	return models.Category{ID: "general", Name: "Geral / Outros"}
}

func copyDownload(d *models.Download) models.Download {
	cp := *d
	if len(d.Segments) > 0 {
		cp.Segments = make([]models.Segment, len(d.Segments))
		copy(cp.Segments, d.Segments)
	}
	if len(d.Events) > 0 {
		cp.Events = make([]models.ConnectionEvent, len(d.Events))
		copy(cp.Events, d.Events)
	}
	return cp
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("dl_%s_%d", hex.EncodeToString(b), time.Now().UnixMilli())
}
