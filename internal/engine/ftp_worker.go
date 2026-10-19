package engine

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"openidm/internal/models"

	"github.com/jlaffaye/ftp"
)

// probeFTP obtém o tamanho do arquivo em um servidor FTP em modo PASV.
func probeFTP(rawURL string, login *models.SiteLogin) (int64, bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, false, err
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":21"
	}

	conn, err := ftp.Dial(host, ftp.DialWithTimeout(15*time.Second))
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = conn.Quit() }()

	user, pass := "anonymous", "anonymous@openidm.local"
	if u.User != nil {
		user = u.User.Username()
		if p, ok := u.User.Password(); ok {
			pass = p
		}
	} else if login != nil && login.Username != "" {
		user = login.Username
		pass = login.Password
	}

	if err := conn.Login(user, pass); err != nil {
		return 0, false, fmt.Errorf("falha no login FTP: %w", err)
	}

	remotePath := strings.TrimPrefix(u.Path, "/")
	size, err := conn.FileSize(remotePath)
	if err != nil {
		return 0, false, nil // Tamanho desconhecido, faz stream
	}
	return size, size > 0, nil
}

// downloadFTPSegment baixa um intervalo [seg.Current, seg.End) via FTP PASV usando RetrFrom (REST).
func downloadFTPSegment(
	ctx context.Context,
	rawURL string,
	login *models.SiteLogin,
	file *os.File,
	segStart int64,
	table *SegmentTable,
	globalLimiter *SpeedLimiter,
	localLimiter *SpeedLimiter,
) error {
	cur, end, ok := table.GetSegmentBound(segStart)
	if !ok {
		return nil
	}
	if end > 0 && cur >= end {
		table.CompleteSegment(segStart)
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":21"
	}

	conn, err := ftp.Dial(host, ftp.DialWithTimeout(20*time.Second))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Quit() }()

	user, pass := "anonymous", "anonymous@openidm.local"
	if u.User != nil {
		user = u.User.Username()
		if p, ok := u.User.Password(); ok {
			pass = p
		}
	} else if login != nil && login.Username != "" {
		user = login.Username
		pass = login.Password
	}

	if err := conn.Login(user, pass); err != nil {
		return err
	}

	cur, end, ok = table.GetSegmentBound(segStart)
	if !ok {
		return nil
	}
	if end > 0 && cur >= end {
		table.CompleteSegment(segStart)
		return nil
	}

	remotePath := strings.TrimPrefix(u.Path, "/")
	resp, err := conn.RetrFrom(remotePath, uint64(cur))
	if err != nil {
		return err
	}
	defer resp.Close()

	buf := make([]byte, 32*1024)
	windowStart := time.Now()
	var windowBytes int64
	var currentSpeed int64

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := resp.Read(buf)
		if n > 0 {
			if err := globalLimiter.Wait(ctx, n); err != nil {
				return err
			}
			if err := localLimiter.Wait(ctx, n); err != nil {
				return err
			}

			writeOffset, _, valid := table.GetSegmentBound(segStart)
			if !valid {
				return nil
			}

			windowBytes += int64(n)
			elapsed := time.Since(windowStart).Seconds()
			if elapsed >= 0.25 {
				instSpeed := int64(float64(windowBytes) / elapsed)
				if currentSpeed > 0 {
					currentSpeed = (currentSpeed*3 + instSpeed*7) / 10
				} else {
					currentSpeed = instSpeed
				}
				windowStart = time.Now()
				windowBytes = 0
			} else if currentSpeed == 0 && elapsed >= 0.02 && windowBytes > 0 {
				currentSpeed = int64(float64(windowBytes) / elapsed)
			}

			accepted, reachedEnd, found := table.AdvanceSegment(segStart, int64(n), currentSpeed)
			if !found {
				return nil
			}
			if accepted > 0 {
				if _, wErr := file.WriteAt(buf[:accepted], writeOffset); wErr != nil {
					return wErr
				}
			}
			if reachedEnd {
				return nil
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				curNow, endNow, ok := table.GetSegmentBound(segStart)
				if !ok {
					return nil
				}
				if endNow <= 0 || curNow >= endNow {
					table.CompleteSegment(segStart)
					return nil
				}
				return io.ErrUnexpectedEOF
			}
			return readErr
		}
	}
}
