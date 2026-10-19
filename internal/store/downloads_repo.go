package store

import (
	"database/sql"
	"time"

	"openidm/internal/models"
)

// SaveDownload insere ou atualiza um download e seu mapa de segmentos em uma única transação.
func (s *Store) SaveDownload(d *models.Download) error {
	s.writeM.Lock()
	defer s.writeM.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	d.UpdatedAt = time.Now()

	var completedAt any
	if d.CompletedAt != nil {
		completedAt = d.CompletedAt.UTC()
	}

	_, err = tx.Exec(`
		INSERT INTO downloads (
			id, url, referrer, file_name, save_dir, target_path, description, category_id, queue_id,
			total_size, downloaded_size, progress, status, accept_ranges, max_connections,
			degraded_single, degraded_reason, retry_count, error_message, etag, last_modified,
			speed_limit_bps, open_on_complete, created_at, updated_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			url = excluded.url,
			referrer = excluded.referrer,
			file_name = excluded.file_name,
			save_dir = excluded.save_dir,
			target_path = excluded.target_path,
			description = excluded.description,
			category_id = excluded.category_id,
			queue_id = excluded.queue_id,
			total_size = excluded.total_size,
			downloaded_size = excluded.downloaded_size,
			progress = excluded.progress,
			status = excluded.status,
			accept_ranges = excluded.accept_ranges,
			max_connections = excluded.max_connections,
			degraded_single = excluded.degraded_single,
			degraded_reason = excluded.degraded_reason,
			retry_count = excluded.retry_count,
			error_message = excluded.error_message,
			etag = excluded.etag,
			last_modified = excluded.last_modified,
			speed_limit_bps = excluded.speed_limit_bps,
			open_on_complete = excluded.open_on_complete,
			updated_at = excluded.updated_at,
			completed_at = excluded.completed_at
	`,
		d.ID, d.URL, d.Referrer, d.FileName, d.SaveDir, d.TargetPath, d.Description, d.CategoryID, d.QueueID,
		d.TotalSize, d.DownloadedSize, d.Progress, string(d.Status), boolToInt(d.AcceptRanges), d.MaxConnections,
		boolToInt(d.DegradedSingle), d.DegradedReason, d.RetryCount, d.ErrorMessage, d.ETag, d.LastModified,
		d.SpeedLimitBps, boolToInt(d.OpenOnComplete), d.CreatedAt.UTC(), d.UpdatedAt.UTC(), completedAt,
	)
	if err != nil {
		return err
	}

	// Regrava o mapa de segmentos atualizado (checkpoint atômico).
	if _, err := tx.Exec(`DELETE FROM segments WHERE download_id = ?`, d.ID); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO segments (
			download_id, seg_index, start_byte, current_byte, end_byte, status, worker_id, reused_conn, last_error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, seg := range d.Segments {
		_, err := stmt.Exec(
			d.ID, i, seg.Start, seg.Current, seg.End,
			string(seg.Status), seg.WorkerID, boolToInt(seg.ReusedConn), seg.LastError,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetDownload carrega um download pelo ID junto com todos os seus segmentos ordenados.
func (s *Store) GetDownload(id string) (*models.Download, error) {
	row := s.db.QueryRow(`
		SELECT
			id, url, referrer, file_name, save_dir, target_path, description, category_id, queue_id,
			total_size, downloaded_size, progress, status, accept_ranges, max_connections,
			degraded_single, degraded_reason, retry_count, error_message, etag, last_modified,
			speed_limit_bps, open_on_complete, created_at, updated_at, completed_at
		FROM downloads WHERE id = ?
	`, id)

	d, err := scanDownloadRow(row)
	if err != nil {
		return nil, err
	}

	segs, err := s.GetSegments(d.ID)
	if err != nil {
		return nil, err
	}
	d.Segments = segs
	return d, nil
}

// ListDownloads retorna todos os downloads ordenados do mais recente ao mais antigo, incluindo segmentos.
func (s *Store) ListDownloads() ([]models.Download, error) {
	rows, err := s.db.Query(`
		SELECT
			id, url, referrer, file_name, save_dir, target_path, description, category_id, queue_id,
			total_size, downloaded_size, progress, status, accept_ranges, max_connections,
			degraded_single, degraded_reason, retry_count, error_message, etag, last_modified,
			speed_limit_bps, open_on_complete, created_at, updated_at, completed_at
		FROM downloads
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Download
	for rows.Next() {
		d, err := scanDownloadRows(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range list {
		segs, err := s.GetSegments(list[i].ID)
		if err != nil {
			return nil, err
		}
		list[i].Segments = segs
	}

	return list, nil
}

// GetSegments retorna o mapa de segmentos de um download ordenado pelo índice.
func (s *Store) GetSegments(downloadID string) ([]models.Segment, error) {
	rows, err := s.db.Query(`
		SELECT id, download_id, seg_index, start_byte, current_byte, end_byte, status, worker_id, reused_conn, last_error
		FROM segments
		WHERE download_id = ?
		ORDER BY seg_index ASC
	`, downloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var segs []models.Segment
	for rows.Next() {
		var seg models.Segment
		var status string
		var reused int
		if err := rows.Scan(
			&seg.ID, &seg.DownloadID, &seg.Index, &seg.Start, &seg.Current, &seg.End,
			&status, &seg.WorkerID, &reused, &seg.LastError,
		); err != nil {
			return nil, err
		}
		seg.Status = models.SegmentStatus(status)
		seg.ReusedConn = intToBool(reused)
		segs = append(segs, seg)
	}
	return segs, rows.Err()
}

// DeleteDownload remove o download e seus segmentos do SQLite.
func (s *Store) DeleteDownload(id string) error {
	s.writeM.Lock()
	defer s.writeM.Unlock()

	_, err := s.db.Exec(`DELETE FROM downloads WHERE id = ?`, id)
	return err
}

// DeleteCompletedDownloads remove todos os downloads com status "completed" do histórico.
func (s *Store) DeleteCompletedDownloads() error {
	s.writeM.Lock()
	defer s.writeM.Unlock()

	_, err := s.db.Exec(`DELETE FROM downloads WHERE status = ?`, string(models.StatusCompleted))
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDownloadRow(r rowScanner) (*models.Download, error) {
	var d models.Download
	var status string
	var acceptRanges, degradedSingle, openOnComplete int
	var completedAt sql.NullTime

	err := r.Scan(
		&d.ID, &d.URL, &d.Referrer, &d.FileName, &d.SaveDir, &d.TargetPath, &d.Description, &d.CategoryID, &d.QueueID,
		&d.TotalSize, &d.DownloadedSize, &d.Progress, &status, &acceptRanges, &d.MaxConnections,
		&degradedSingle, &d.DegradedReason, &d.RetryCount, &d.ErrorMessage, &d.ETag, &d.LastModified,
		&d.SpeedLimitBps, &openOnComplete, &d.CreatedAt, &d.UpdatedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}

	d.Status = models.DownloadStatus(status)
	d.AcceptRanges = intToBool(acceptRanges)
	d.DegradedSingle = intToBool(degradedSingle)
	d.OpenOnComplete = intToBool(openOnComplete)
	if completedAt.Valid {
		t := completedAt.Time
		d.CompletedAt = &t
	}
	return &d, nil
}

func scanDownloadRows(rows *sql.Rows) (*models.Download, error) {
	return scanDownloadRow(rows)
}
