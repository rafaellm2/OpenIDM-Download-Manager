package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"openidm/internal/models"

	_ "modernc.org/sqlite"
)

// Store encapsula a conexão SQLite e sincroniza escritas concorrentes de checkpoints.
type Store struct {
	db     *sql.DB
	writeM sync.Mutex
}

// NewStore abre ou cria o banco SQLite puro Go (sem CGO) com modo WAL ativado.
func NewStore(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("falha ao criar diretório do banco: %w", err)
	}

	// Configura WAL e busy_timeout para suportar múltiplos workers gravando checkpoints.
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", filepath.ToSlash(dbPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.seedDefaults(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close fecha a conexão com o banco SQLite.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// migrate executa a criação das tabelas necessárias para downloads, segmentos, filas, categorias e regras.
func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS categories (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		slug TEXT NOT NULL,
		extensions_json TEXT NOT NULL,
		default_dir TEXT NOT NULL,
		icon TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS queues (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		is_main INTEGER NOT NULL DEFAULT 0,
		is_sync INTEGER NOT NULL DEFAULT 0,
		max_concurrent INTEGER NOT NULL DEFAULT 4,
		start_time TEXT NOT NULL DEFAULT '',
		stop_time TEXT NOT NULL DEFAULT '',
		traffic_quota_mb INTEGER NOT NULL DEFAULT 0,
		used_traffic_bytes INTEGER NOT NULL DEFAULT 0,
		retry_interval_sec INTEGER NOT NULL DEFAULT 30,
		max_retries INTEGER NOT NULL DEFAULT 10,
		shutdown_on_finish INTEGER NOT NULL DEFAULT 0,
		run_command_on_finish TEXT NOT NULL DEFAULT '',
		active INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS downloads (
		id TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		referrer TEXT NOT NULL DEFAULT '',
		file_name TEXT NOT NULL,
		save_dir TEXT NOT NULL,
		target_path TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		category_id TEXT NOT NULL,
		queue_id TEXT NOT NULL,
		total_size INTEGER NOT NULL DEFAULT 0,
		downloaded_size INTEGER NOT NULL DEFAULT 0,
		progress REAL NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		accept_ranges INTEGER NOT NULL DEFAULT 0,
		max_connections INTEGER NOT NULL DEFAULT 8,
		degraded_single INTEGER NOT NULL DEFAULT 0,
		degraded_reason TEXT NOT NULL DEFAULT '',
		retry_count INTEGER NOT NULL DEFAULT 0,
		error_message TEXT NOT NULL DEFAULT '',
		etag TEXT NOT NULL DEFAULT '',
		last_modified TEXT NOT NULL DEFAULT '',
		speed_limit_bps INTEGER NOT NULL DEFAULT 0,
		open_on_complete INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		completed_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS segments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		download_id TEXT NOT NULL,
		seg_index INTEGER NOT NULL,
		start_byte INTEGER NOT NULL,
		current_byte INTEGER NOT NULL,
		end_byte INTEGER NOT NULL,
		status TEXT NOT NULL,
		worker_id INTEGER NOT NULL DEFAULT 0,
		reused_conn INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		FOREIGN KEY(download_id) REFERENCES downloads(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_segments_download ON segments(download_id, seg_index);

	CREATE TABLE IF NOT EXISTS site_rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		host_pattern TEXT NOT NULL UNIQUE,
		max_connections INTEGER NOT NULL DEFAULT 8,
		ignore_capture INTEGER NOT NULL DEFAULT 0,
		retry_limit INTEGER NOT NULL DEFAULT 10,
		retry_interval_sec INTEGER NOT NULL DEFAULT 30
	);

	CREATE TABLE IF NOT EXISTS site_logins (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		host_pattern TEXT NOT NULL UNIQUE,
		username TEXT NOT NULL,
		password TEXT NOT NULL,
		auth_type TEXT NOT NULL DEFAULT 'basic'
	);

	CREATE TABLE IF NOT EXISTS app_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		settings_json TEXT NOT NULL
	);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Migração incremental segura para bancos existentes sem a coluna referrer.
	_, _ = s.db.Exec(`ALTER TABLE downloads ADD COLUMN referrer TEXT NOT NULL DEFAULT ''`)
	return nil
}

// seedDefaults popula as categorias padrão, as filas (Principal e Sincronização) e as configurações iniciais.
func (s *Store) seedDefaults() error {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		homeDir = "."
	}
	baseDownloads := filepath.Join(homeDir, "Downloads")

	defaultCategories := []models.Category{
		{
			ID:         "compressed",
			Name:       "Comprimidos",
			Slug:       "compressed",
			Extensions: []string{"zip", "rar", "7z", "tar", "gz", "bz2", "xz", "tgz", "zst", "torrent"},
			DefaultDir: filepath.Join(baseDownloads, "Comprimidos"),
			Icon:       "archive",
		},
		{
			ID:         "documents",
			Name:       "Documentos",
			Slug:       "documents",
			Extensions: []string{"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "txt", "csv", "epub", "md"},
			DefaultDir: filepath.Join(baseDownloads, "Documentos"),
			Icon:       "file-text",
		},
		{
			ID:         "music",
			Name:       "Música",
			Slug:       "music",
			Extensions: []string{"mp3", "wav", "flac", "aac", "ogg", "m4a", "wma", "opus"},
			DefaultDir: filepath.Join(baseDownloads, "Musica"),
			Icon:       "music",
		},
		{
			ID:         "programs",
			Name:       "Programas",
			Slug:       "programs",
			Extensions: []string{"exe", "msi", "iso", "img", "apk", "dmg", "deb", "rpm", "bat", "ps1", "safetensors", "gguf", "ckpt", "pt", "pth", "onnx", "bin", "whl"},
			DefaultDir: filepath.Join(baseDownloads, "Programas"),
			Icon:       "cpu",
		},
		{
			ID:         "video",
			Name:       "Vídeo",
			Slug:       "video",
			Extensions: []string{"mp4", "mkv", "webm", "avi", "mov", "flv", "wmv", "m4v", "ts"},
			DefaultDir: filepath.Join(baseDownloads, "Video"),
			Icon:       "film",
		},
		{
			ID:         "general",
			Name:       "Geral / Outros",
			Slug:       "general",
			Extensions: []string{},
			DefaultDir: filepath.Join(baseDownloads, "Geral"),
			Icon:       "folder",
		},
	}

	for _, c := range defaultCategories {
		extJSON, _ := json.Marshal(c.Extensions)
		_, err := s.db.Exec(`
			INSERT OR IGNORE INTO categories (id, name, slug, extensions_json, default_dir, icon)
			VALUES (?, ?, ?, ?, ?, ?)`,
			c.ID, c.Name, c.Slug, string(extJSON), c.DefaultDir, c.Icon,
		)
		if err != nil {
			return err
		}
	}

	// Filas padrão iniciais: Fila Principal e Fila de Sincronização.
	defaultQueues := []models.Queue{
		{
			ID:               "main",
			Name:             "Fila Principal de Downloads",
			IsMain:           true,
			IsSync:           false,
			MaxConcurrent:    4,
			RetryIntervalSec: 30,
			MaxRetries:       10,
			Active:           false,
		},
		{
			ID:               "sync",
			Name:             "Fila de Sincronização",
			IsMain:           false,
			IsSync:           true,
			MaxConcurrent:    2,
			RetryIntervalSec: 30,
			MaxRetries:       5,
			Active:           false,
		},
	}

	for _, q := range defaultQueues {
		_, err := s.db.Exec(`
			INSERT OR IGNORE INTO queues (
				id, name, is_main, is_sync, max_concurrent, start_time, stop_time,
				traffic_quota_mb, used_traffic_bytes, retry_interval_sec, max_retries,
				shutdown_on_finish, run_command_on_finish, active
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			q.ID, q.Name, boolToInt(q.IsMain), boolToInt(q.IsSync), q.MaxConcurrent,
			q.StartTime, q.StopTime, q.TrafficQuotaMB, q.UsedTrafficBytes,
			q.RetryIntervalSec, q.MaxRetries, boolToInt(q.ShutdownOnFinish),
			q.RunCommandOnFinish, boolToInt(q.Active),
		)
		if err != nil {
			return err
		}
	}

	// Configurações globais padrão.
	defaultSettings := models.AppSettings{
		MaxConnectionsPerDownload: 8,
		MinSplitSizeBytes:         1024 * 1024, // 1 MB (especificação da Seção 2 do plano)
		GlobalSpeedLimitBps:       0,
		DefaultSaveDir:            baseDownloads,
		StartWithWindows:          true,
		MonitorClipboard:          true,
		CaptureExtensions: []string{
			"zip", "rar", "7z", "tar", "gz", "bz2", "xz", "tgz", "zst", "torrent",
			"exe", "msi", "iso", "img", "apk", "dmg", "deb", "rpm", "bat", "ps1",
			"safetensors", "gguf", "ckpt", "pt", "pth", "onnx", "bin", "whl",
			"mp4", "mkv", "webm", "avi", "mov", "flv", "wmv", "m4v", "ts",
			"mp3", "flac", "wav", "aac", "ogg", "m4a", "wma", "opus",
			"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "txt", "csv", "epub", "md",
		},
		IgnoreModifierKey:     "ALT",
		ProxyType:             "none",
		ProxyPort:             8080,
		AntivirusEnabled:      false,
		AntivirusArgs:         "\"[file]\"",
		CheckpointIntervalSec: 2,
		Language:              "pt-BR",
	}

	rawSettings, _ := json.Marshal(defaultSettings)
	_, err = s.db.Exec(`INSERT OR IGNORE INTO app_settings (id, settings_json) VALUES (1, ?)`, string(rawSettings))
	if err != nil {
		return err
	}

	// Migra caminhos padrão legados (removendo subpasta intermediária OpenIDM ou TurboDM de Downloads)
	// Atualiza os diretórios padrão na tabela de categorias
	_, _ = s.db.Exec(`
		UPDATE categories
		SET default_dir = REPLACE(REPLACE(REPLACE(REPLACE(
			REPLACE(REPLACE(REPLACE(REPLACE(
				default_dir,
				'\Downloads\OpenIDM\', '\Downloads\'),
				'/Downloads/OpenIDM/', '/Downloads/'),
				'\Downloads\OpenIDM', '\Downloads'),
				'/Downloads/OpenIDM', '/Downloads'),
				'\Downloads\TurboDM\', '\Downloads\'),
				'/Downloads/TurboDM/', '/Downloads/'),
				'\Downloads\TurboDM', '\Downloads'),
				'/Downloads/TurboDM', '/Downloads')
		WHERE default_dir LIKE '%Downloads%OpenIDM%' OR default_dir LIKE '%Downloads%TurboDM%'
	`)
	// Atualiza o caminho global default_save_dir no JSON da tabela app_settings
	_, _ = s.db.Exec(`
		UPDATE app_settings
		SET settings_json = REPLACE(REPLACE(REPLACE(REPLACE(
			REPLACE(REPLACE(REPLACE(REPLACE(
				settings_json,
				'\\Downloads\\OpenIDM\\', '\\Downloads\\'),
				'/Downloads/OpenIDM/', '/Downloads/'),
				'\\Downloads\\OpenIDM', '\\Downloads'),
				'/Downloads/OpenIDM', '/Downloads'),
				'\\Downloads\\TurboDM\\', '\\Downloads\\'),
				'/Downloads/TurboDM/', '/Downloads/'),
				'\\Downloads\\TurboDM', '\\Downloads'),
				'/Downloads/TurboDM', '/Downloads')
		WHERE settings_json LIKE '%Downloads%OpenIDM%' OR settings_json LIKE '%Downloads%TurboDM%'
	`)
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(v int) bool {
	return v != 0
}
