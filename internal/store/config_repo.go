package store

import (
	"encoding/json"

	"openidm/internal/models"
)

// ListCategories retorna todas as categorias cadastradas.
func (s *Store) ListCategories() ([]models.Category, error) {
	rows, err := s.db.Query(`SELECT id, name, slug, extensions_json, default_dir, icon FROM categories ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []models.Category
	for rows.Next() {
		var c models.Category
		var extJSON string
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &extJSON, &c.DefaultDir, &c.Icon); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(extJSON), &c.Extensions)
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// SaveCategory cria ou atualiza uma categoria.
func (s *Store) SaveCategory(c models.Category) error {
	s.writeM.Lock()
	defer s.writeM.Unlock()

	extJSON, _ := json.Marshal(c.Extensions)
	_, err := s.db.Exec(`
		INSERT INTO categories (id, name, slug, extensions_json, default_dir, icon)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			slug = excluded.slug,
			extensions_json = excluded.extensions_json,
			default_dir = excluded.default_dir,
			icon = excluded.icon
	`, c.ID, c.Name, c.Slug, string(extJSON), c.DefaultDir, c.Icon)
	return err
}

// ListQueues retorna todas as filas de downloads (Principal, Sincronização e Customizadas).
func (s *Store) ListQueues() ([]models.Queue, error) {
	rows, err := s.db.Query(`
		SELECT id, name, is_main, is_sync, max_concurrent, start_time, stop_time,
		       traffic_quota_mb, used_traffic_bytes, retry_interval_sec, max_retries,
		       shutdown_on_finish, run_command_on_finish, active
		FROM queues
		ORDER BY is_main DESC, is_sync DESC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queues []models.Queue
	for rows.Next() {
		var q models.Queue
		var isMain, isSync, shutdown, active int
		if err := rows.Scan(
			&q.ID, &q.Name, &isMain, &isSync, &q.MaxConcurrent, &q.StartTime, &q.StopTime,
			&q.TrafficQuotaMB, &q.UsedTrafficBytes, &q.RetryIntervalSec, &q.MaxRetries,
			&shutdown, &q.RunCommandOnFinish, &active,
		); err != nil {
			return nil, err
		}
		q.IsMain = intToBool(isMain)
		q.IsSync = intToBool(isSync)
		q.ShutdownOnFinish = intToBool(shutdown)
		q.Active = intToBool(active)
		queues = append(queues, q)
	}
	return queues, rows.Err()
}

// SaveQueue insere ou atualiza as configurações de uma fila.
func (s *Store) SaveQueue(q models.Queue) error {
	s.writeM.Lock()
	defer s.writeM.Unlock()

	if q.MaxConcurrent <= 0 {
		q.MaxConcurrent = 4
	}
	if q.RetryIntervalSec <= 0 {
		q.RetryIntervalSec = 30
	}

	_, err := s.db.Exec(`
		INSERT INTO queues (
			id, name, is_main, is_sync, max_concurrent, start_time, stop_time,
			traffic_quota_mb, used_traffic_bytes, retry_interval_sec, max_retries,
			shutdown_on_finish, run_command_on_finish, active
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			is_main = excluded.is_main,
			is_sync = excluded.is_sync,
			max_concurrent = excluded.max_concurrent,
			start_time = excluded.start_time,
			stop_time = excluded.stop_time,
			traffic_quota_mb = excluded.traffic_quota_mb,
			used_traffic_bytes = excluded.used_traffic_bytes,
			retry_interval_sec = excluded.retry_interval_sec,
			max_retries = excluded.max_retries,
			shutdown_on_finish = excluded.shutdown_on_finish,
			run_command_on_finish = excluded.run_command_on_finish,
			active = excluded.active
	`,
		q.ID, q.Name, boolToInt(q.IsMain), boolToInt(q.IsSync), q.MaxConcurrent,
		q.StartTime, q.StopTime, q.TrafficQuotaMB, q.UsedTrafficBytes,
		q.RetryIntervalSec, q.MaxRetries, boolToInt(q.ShutdownOnFinish),
		q.RunCommandOnFinish, boolToInt(q.Active),
	)
	return err
}

// DeleteQueue remove uma fila customizada (não permite excluir fila principal ou sincronização).
func (s *Store) DeleteQueue(id string) error {
	s.writeM.Lock()
	defer s.writeM.Unlock()

	_, err := s.db.Exec(`DELETE FROM queues WHERE id = ? AND is_main = 0 AND is_sync = 0`, id)
	return err
}

// AddQueueTraffic incrementa o contador de bytes trafegados de uma fila.
func (s *Store) AddQueueTraffic(queueID string, deltaBytes int64) error {
	s.writeM.Lock()
	defer s.writeM.Unlock()

	_, err := s.db.Exec(`UPDATE queues SET used_traffic_bytes = used_traffic_bytes + ? WHERE id = ?`, deltaBytes, queueID)
	return err
}

// ListSiteRules retorna todas as regras e exceções por site.
func (s *Store) ListSiteRules() ([]models.SiteRule, error) {
	rows, err := s.db.Query(`
		SELECT id, host_pattern, max_connections, ignore_capture, retry_limit, retry_interval_sec
		FROM site_rules ORDER BY host_pattern ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.SiteRule
	for rows.Next() {
		var r models.SiteRule
		var ignore int
		if err := rows.Scan(&r.ID, &r.HostPattern, &r.MaxConnections, &ignore, &r.RetryLimit, &r.RetryIntervalSec); err != nil {
			return nil, err
		}
		r.IgnoreCapture = intToBool(ignore)
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// SaveSiteRule adiciona ou atualiza uma exceção/regra de conexões para um site.
func (s *Store) SaveSiteRule(r models.SiteRule) error {
	s.writeM.Lock()
	defer s.writeM.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO site_rules (host_pattern, max_connections, ignore_capture, retry_limit, retry_interval_sec)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(host_pattern) DO UPDATE SET
			max_connections = excluded.max_connections,
			ignore_capture = excluded.ignore_capture,
			retry_limit = excluded.retry_limit,
			retry_interval_sec = excluded.retry_interval_sec
	`, r.HostPattern, r.MaxConnections, boolToInt(r.IgnoreCapture), r.RetryLimit, r.RetryIntervalSec)
	return err
}

// DeleteSiteRule remove uma regra de site pelo ID.
func (s *Store) DeleteSiteRule(id int64) error {
	s.writeM.Lock()
	defer s.writeM.Unlock()

	_, err := s.db.Exec(`DELETE FROM site_rules WHERE id = ?`, id)
	return err
}

// ListSiteLogins retorna todos os logins salvos por servidor.
func (s *Store) ListSiteLogins() ([]models.SiteLogin, error) {
	rows, err := s.db.Query(`SELECT id, host_pattern, username, password, auth_type FROM site_logins ORDER BY host_pattern ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logins []models.SiteLogin
	for rows.Next() {
		var l models.SiteLogin
		if err := rows.Scan(&l.ID, &l.HostPattern, &l.Username, &l.Password, &l.AuthType); err != nil {
			return nil, err
		}
		logins = append(logins, l)
	}
	return logins, rows.Err()
}

// SaveSiteLogin cadastra ou atualiza um login automático por servidor.
func (s *Store) SaveSiteLogin(l models.SiteLogin) error {
	s.writeM.Lock()
	defer s.writeM.Unlock()

	if l.AuthType == "" {
		l.AuthType = "basic"
	}
	_, err := s.db.Exec(`
		INSERT INTO site_logins (host_pattern, username, password, auth_type)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(host_pattern) DO UPDATE SET
			username = excluded.username,
			password = excluded.password,
			auth_type = excluded.auth_type
	`, l.HostPattern, l.Username, l.Password, l.AuthType)
	return err
}

// DeleteSiteLogin remove uma credencial salva pelo ID.
func (s *Store) DeleteSiteLogin(id int64) error {
	s.writeM.Lock()
	defer s.writeM.Unlock()

	_, err := s.db.Exec(`DELETE FROM site_logins WHERE id = ?`, id)
	return err
}

// GetSettings carrega as configurações globais do aplicativo.
func (s *Store) GetSettings() (models.AppSettings, error) {
	var raw string
	err := s.db.QueryRow(`SELECT settings_json FROM app_settings WHERE id = 1`).Scan(&raw)
	if err != nil {
		return models.AppSettings{}, err
	}
	var cfg models.AppSettings
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return models.AppSettings{}, err
	}
	if cfg.MaxConnectionsPerDownload <= 0 {
		cfg.MaxConnectionsPerDownload = 8
	}
	if cfg.MinSplitSizeBytes <= 0 {
		cfg.MinSplitSizeBytes = 1024 * 1024
	}
	cfg.Language = models.NormalizeLanguage(cfg.Language)
	return cfg, nil
}

// SaveSettings persiste as configurações globais do aplicativo.
func (s *Store) SaveSettings(cfg models.AppSettings) error {
	s.writeM.Lock()
	defer s.writeM.Unlock()

	cfg.Language = models.NormalizeLanguage(cfg.Language)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO app_settings (id, settings_json) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET settings_json = excluded.settings_json
	`, string(raw))
	return err
}
