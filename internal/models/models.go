package models

import "time"

// DownloadStatus define os estados possíveis de uma tarefa de download.
type DownloadStatus string

const (
	StatusQueued      DownloadStatus = "queued"
	StatusProbing     DownloadStatus = "probing"
	StatusDownloading DownloadStatus = "downloading"
	StatusPaused      DownloadStatus = "paused"
	StatusAssembling  DownloadStatus = "assembling"
	StatusCompleted   DownloadStatus = "completed"
	StatusError       DownloadStatus = "error"
	StatusCancelled   DownloadStatus = "cancelled"
)

// SegmentStatus define o estado individual de cada segmento dinâmico do arquivo.
type SegmentStatus string

const (
	SegmentPending     SegmentStatus = "pending"
	SegmentDownloading SegmentStatus = "downloading"
	SegmentCompleted   SegmentStatus = "completed"
	SegmentError       SegmentStatus = "error"
)

// Segment representa uma fatia de bytes [Start, End) no mapa de segmentação dinâmica.
// Current aponta para o próximo byte a ser gravado (Start <= Current <= End).
type Segment struct {
	ID         int64         `json:"id"`
	DownloadID string        `json:"downloadId"`
	Index      int           `json:"index"`
	Start      int64         `json:"start"`
	Current    int64         `json:"current"`
	End        int64         `json:"end"`
	Status     SegmentStatus `json:"status"`
	WorkerID   int           `json:"workerId"`
	SpeedBps   int64         `json:"speedBps"`
	ReusedConn bool          `json:"reusedConn"`
	LastError  string        `json:"lastError,omitempty"`
}

// DownloadedBytes retorna o total de bytes já baixados neste segmento.
func (s Segment) DownloadedBytes() int64 {
	if s.Current < s.Start {
		return 0
	}
	if s.End > 0 && s.Current > s.End {
		return s.End - s.Start
	}
	return s.Current - s.Start
}

// RemainingBytes retorna quantos bytes ainda faltam para concluir este segmento.
func (s Segment) RemainingBytes() int64 {
	if s.End <= 0 || s.Current >= s.End {
		return 0
	}
	return s.End - s.Current
}

// ConnectionEvent registra eventos em tempo real das conexões de download.
type ConnectionEvent struct {
	Timestamp time.Time `json:"timestamp"`
	WorkerID  int       `json:"workerId"`
	Message   string    `json:"message"`
	Level     string    `json:"level"` // "info", "warn", "error", "reuse"
}

// Download representa um item completo gerenciado pelo motor de downloads.
type Download struct {
	ID               string            `json:"id"`
	URL              string            `json:"url"`
	FileName         string            `json:"fileName"`
	SaveDir          string            `json:"saveDir"`
	TargetPath       string            `json:"targetPath"`
	Description      string            `json:"description"`
	CategoryID       string            `json:"categoryId"`
	QueueID          string            `json:"queueId"`
	TotalSize        int64             `json:"totalSize"`
	DownloadedSize   int64             `json:"downloadedSize"`
	Progress         float64           `json:"progress"`
	SpeedBps         int64             `json:"speedBps"`
	ETASeconds       int64             `json:"etaSeconds"`
	Status           DownloadStatus    `json:"status"`
	AcceptRanges     bool              `json:"acceptRanges"`
	MaxConnections   int               `json:"maxConnections"`
	ActiveWorkers    int               `json:"activeWorkers"`
	DegradedSingle   bool              `json:"degradedSingle"`
	DegradedReason   string            `json:"degradedReason,omitempty"`
	RetryCount       int               `json:"retryCount"`
	ErrorMessage     string            `json:"errorMessage,omitempty"`
	ETag             string            `json:"etag,omitempty"`
	LastModified     string            `json:"lastModified,omitempty"`
	SpeedLimitBps    int64             `json:"speedLimitBps"`
	OpenOnComplete   bool              `json:"openOnComplete"`
	Referrer         string            `json:"referrer,omitempty"`
	Segments         []Segment         `json:"segments"`
	Events           []ConnectionEvent `json:"events,omitempty"`
	CreatedAt        time.Time         `json:"createdAt"`
	UpdatedAt        time.Time         `json:"updatedAt"`
	CompletedAt      *time.Time        `json:"completedAt,omitempty"`
}

// Category define uma categoria automática por extensão (Vídeo, Música, Programas, etc.).
type Category struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Slug       string   `json:"slug"`
	Extensions []string `json:"extensions"`
	DefaultDir string   `json:"defaultDir"`
	Icon       string   `json:"icon"`
}

// Queue define uma fila de downloads e suas regras no Scheduler (Agendador).
type Queue struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	IsMain             bool   `json:"isMain"`
	IsSync             bool   `json:"isSync"` // Fila de sincronização (verifica alterações no servidor via ETag/Last-Modified)
	MaxConcurrent      int    `json:"maxConcurrent"`
	StartTime          string `json:"startTime"`          // Formato "HH:MM" (vazio = manual/imediato)
	StopTime           string `json:"stopTime"`           // Formato "HH:MM"
	TrafficQuotaMB     int64  `json:"trafficQuotaMB"`     // 0 = sem limite de quota por período
	UsedTrafficBytes   int64  `json:"usedTrafficBytes"`   // Tráfego acumulado na fila
	RetryIntervalSec   int    `json:"retryIntervalSec"`   // Intervalo padrão de retry em segundos (default 30s)
	MaxRetries         int    `json:"maxRetries"`         // 0 = infinito quando sozinho na fila
	ShutdownOnFinish   bool   `json:"shutdownOnFinish"`   // Desligar o PC ao concluir a fila
	RunCommandOnFinish string `json:"runCommandOnFinish"` // Comando opcional ao concluir fila
	Active             bool   `json:"active"`
}

// SiteRule configura exceções e limites de conexões por domínio/host (suporta wildcard *.exemplo.com).
type SiteRule struct {
	ID               int64  `json:"id"`
	HostPattern      string `json:"hostPattern"`
	MaxConnections   int    `json:"maxConnections"`
	IgnoreCapture    bool   `json:"ignoreCapture"`
	RetryLimit       int    `json:"retryLimit"`
	RetryIntervalSec int    `json:"retryIntervalSec"`
}

// SiteLogin armazena credenciais automáticas para servidores HTTP/FTP protegidos.
type SiteLogin struct {
	ID          int64  `json:"id"`
	HostPattern string `json:"hostPattern"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	AuthType    string `json:"authType"` // "basic", "bearer", "ftp"
}

// AppSettings reúne as configurações globais do gerenciador.
type AppSettings struct {
	MaxConnectionsPerDownload int      `json:"maxConnectionsPerDownload"` // 1..32 (default 8)
	MinSplitSizeBytes         int64    `json:"minSplitSizeBytes"`         // Default 1 MB (1048576)
	GlobalSpeedLimitBps       int64    `json:"globalSpeedLimitBps"`       // 0 = ilimitado
	DefaultSaveDir            string   `json:"defaultSaveDir"`
	StartWithWindows          bool     `json:"startWithWindows"`
	MonitorClipboard          bool     `json:"monitorClipboard"`
	CaptureExtensions         []string `json:"captureExtensions"`
	IgnoreModifierKey         string   `json:"ignoreModifierKey"` // Default "ALT"
	ProxyType                 string   `json:"proxyType"`         // "none", "system", "http", "socks5", "pac"
	ProxyHost                 string   `json:"proxyHost"`
	ProxyPort                 int      `json:"proxyPort"`
	ProxyUser                 string   `json:"proxyUser"`
	ProxyPass                 string   `json:"proxyPass"`
	ProxyPACUrl               string   `json:"proxyPACUrl,omitempty"`
	AntivirusEnabled          bool     `json:"antivirusEnabled"`
	AntivirusPath             string   `json:"antivirusPath"`
	AntivirusArgs             string   `json:"antivirusArgs"` // ex: "--scan [file]"
	CheckpointIntervalSec     int      `json:"checkpointIntervalSec"`
	Language                  string   `json:"language,omitempty"` // "pt-BR" (default) ou "en-US"
	CardPosX                  int      `json:"cardPosX,omitempty"`
	CardPosY                  int      `json:"cardPosY,omitempty"`
	CardPosSaved              bool     `json:"cardPosSaved,omitempty"`
}

const (
	LangPtBR = "pt-BR"
	LangEnUS = "en-US"
)

// NormalizeLanguage normaliza o código de idioma para "pt-BR" (padrão) ou "en-US".
func NormalizeLanguage(lang string) string {
	for len(lang) > 0 && (lang[0] == ' ' || lang[0] == '\t' || lang[0] == '\n' || lang[0] == '\r') {
		lang = lang[1:]
	}
	for len(lang) > 0 && (lang[len(lang)-1] == ' ' || lang[len(lang)-1] == '\t' || lang[len(lang)-1] == '\n' || lang[len(lang)-1] == '\r') {
		lang = lang[:len(lang)-1]
	}
	switch lang {
	case "en", "en-US", "en-GB", "en_US", "EN", "EN-US":
		return LangEnUS
	default:
		if len(lang) >= 2 && (lang[:2] == "en" || lang[:2] == "EN" || lang[:2] == "En" || lang[:2] == "eN") {
			return LangEnUS
		}
		return LangPtBR
	}
}

// AddDownloadRequest encapsula os dados enviados pela UI ou pela Extensão do Navegador.
type AddDownloadRequest struct {
	URL            string `json:"url"`
	FileName       string `json:"fileName"`
	SaveDir        string `json:"saveDir"`
	Description    string `json:"description"`
	Referrer       string `json:"referrer,omitempty"`
	CategoryID     string `json:"categoryId"`
	QueueID        string `json:"queueId"`
	MaxConnections int    `json:"maxConnections"`
	StartNow       bool   `json:"startNow"`
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
	SpeedLimitBps  int64  `json:"speedLimitBps,omitempty"`
	OpenOnComplete bool   `json:"openOnComplete,omitempty"`
}

// CapturePrompt representa os metadados de um link capturado para exibição imediata
// na tela de confirmação e organização por categorias do aplicativo desktop.
type CapturePrompt struct {
	URL          string `json:"url"`
	FileName     string `json:"fileName"`
	CategoryID   string `json:"categoryId"`
	CategoryName string `json:"categoryName"`
	SaveDir      string `json:"saveDir"`
	TargetPath   string `json:"targetPath"`
	Referrer     string `json:"referrer,omitempty"`
	Source       string `json:"source"` // "browser", "clipboard", "cli", "manual"
	TotalSize    int64  `json:"totalSize"`
	AcceptRanges bool   `json:"acceptRanges"`
	Probed       bool   `json:"probed"`
	ProbeError   string `json:"probeError,omitempty"`
}

