package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"openidm/internal/models"
)

// ServerProbeResult armazena metadados descobertos no probe inicial da URL.
type ServerProbeResult struct {
	TotalSize    int64
	AcceptRanges bool
	FileName     string
	ETag         string
	LastModified string
	ResolvedURL  string
}

// ProbeURL consulta o servidor (HTTP/HTTPS ou FTP) para descobrir tamanho, suporte a Range e ETag.
func ProbeURL(ctx context.Context, client *http.Client, rawURL string, login *models.SiteLogin) (*ServerProbeResult, error) {
	return ProbeURLWithReferrer(ctx, client, rawURL, login, "")
}

// ProbeURLWithReferrer consulta o servidor enviando o cabeçalho Referer da página de origem quando disponível.
func ProbeURLWithReferrer(ctx context.Context, client *http.Client, rawURL string, login *models.SiteLogin, referrer string) (*ServerProbeResult, error) {
	rawURL = NormalizeDownloadURL(rawURL)
	if strings.HasPrefix(strings.ToLower(rawURL), "ftp://") {
		size, accept, err := probeFTP(rawURL, login)
		if err != nil {
			return nil, err
		}
		return &ServerProbeResult{
			TotalSize:    size,
			AcceptRanges: accept,
			FileName:     ExtractFileNameFromURL(rawURL),
			ResolvedURL:  rawURL,
		}, nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	res := &ServerProbeResult{
		FileName:    ExtractFileNameFromURL(rawURL),
		ResolvedURL: rawURL,
	}
	foundContentDisposition := false
	gotValidResponse := false
	var lastErr error
	var lastStatus int
	var lastStatusText string

	// 1. Tenta requisição HEAD padrão.
	headReq, err := http.NewRequestWithContext(probeCtx, http.MethodHead, rawURL, nil)
	if err == nil {
		ApplyRequestHeadersWithReferrer(headReq, login, referrer)
		resp, hErr := client.Do(headReq)
		if hErr != nil {
			lastErr = hErr
		} else {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				gotValidResponse = true
				if resp.ContentLength > 0 {
					res.TotalSize = resp.ContentLength
				}
				if strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes") {
					res.AcceptRanges = true
				}
				res.ETag = resp.Header.Get("ETag")
				res.LastModified = resp.Header.Get("Last-Modified")
				// Se houve redirecionamento HTTP (301/302/303/307/308), aproveita o nome da URL final apenas se possuir extensão válida.
				if resp.Request != nil && resp.Request.URL != nil {
					res.ResolvedURL = resp.Request.URL.String()
					if redirName := ExtractFileNameFromURL(res.ResolvedURL); shouldUseRedirectFileName(res.FileName, redirName) {
						res.FileName = redirName
					}
				}
				if name := parseContentDispositionFileName(resp.Header.Get("Content-Disposition")); name != "" {
					res.FileName = name
					foundContentDisposition = true
				}
			} else {
				lastStatus = resp.StatusCode
				lastStatusText = resp.Status
			}
		}
	}

	// 2. Faz probe com Range: bytes=0-0 caso o HEAD não confirme Accept-Ranges, tamanho, nome com extensão ou tenha falhado.
	needsFileNameProbe := !foundContentDisposition && IsGenericOrScriptFileName(res.FileName)
	if !gotValidResponse || !res.AcceptRanges || res.TotalSize <= 0 || needsFileNameProbe {
		rangeReq, err := http.NewRequestWithContext(probeCtx, http.MethodGet, rawURL, nil)
		if err == nil {
			ApplyRequestHeadersWithReferrer(rangeReq, login, referrer)
			rangeReq.Header.Set("Range", "bytes=0-0")
			rResp, rErr := client.Do(rangeReq)
			if rErr != nil {
				lastErr = rErr
			} else {
				_, _ = io.Copy(io.Discard, io.LimitReader(rResp.Body, 64))
				_ = rResp.Body.Close()

				if rResp.StatusCode >= 200 && rResp.StatusCode < 400 {
					gotValidResponse = true
					if rResp.StatusCode == http.StatusPartialContent {
						res.AcceptRanges = true
						if total := parseTotalFromContentRange(rResp.Header.Get("Content-Range")); total > 0 {
							res.TotalSize = total
						}
					} else if rResp.StatusCode == http.StatusOK && res.TotalSize <= 0 && rResp.ContentLength > 0 {
						res.TotalSize = rResp.ContentLength
					}
					if res.ETag == "" {
						res.ETag = rResp.Header.Get("ETag")
					}
					if res.LastModified == "" {
						res.LastModified = rResp.Header.Get("Last-Modified")
					}
					if !foundContentDisposition && rResp.Request != nil && rResp.Request.URL != nil {
						res.ResolvedURL = rResp.Request.URL.String()
						if redirName := ExtractFileNameFromURL(res.ResolvedURL); shouldUseRedirectFileName(res.FileName, redirName) {
							res.FileName = redirName
						}
					}
					if name := parseContentDispositionFileName(rResp.Header.Get("Content-Disposition")); name != "" {
						res.FileName = name
						foundContentDisposition = true
					}
				} else {
					lastStatus = rResp.StatusCode
					lastStatusText = rResp.Status
				}
			}
		}
	}

	// 3. Caso o servidor rejeite tanto HEAD quanto cabeçalho Range (ex: HTTP 400/405/416/501), tenta GET simples lendo apenas cabeçalhos.
	if !gotValidResponse && (lastStatus == http.StatusRequestedRangeNotSatisfiable || lastStatus == http.StatusMethodNotAllowed || lastStatus == http.StatusBadRequest || lastStatus == http.StatusNotImplemented) {
		getReq, err := http.NewRequestWithContext(probeCtx, http.MethodGet, rawURL, nil)
		if err == nil {
			ApplyRequestHeadersWithReferrer(getReq, login, referrer)
			gResp, gErr := client.Do(getReq)
			if gErr != nil {
				lastErr = gErr
			} else {
				_ = gResp.Body.Close()
				if gResp.StatusCode >= 200 && gResp.StatusCode < 400 {
					gotValidResponse = true
					if gResp.ContentLength > 0 && res.TotalSize <= 0 {
						res.TotalSize = gResp.ContentLength
					}
					if strings.EqualFold(gResp.Header.Get("Accept-Ranges"), "bytes") {
						res.AcceptRanges = true
					}
					if gResp.Request != nil && gResp.Request.URL != nil {
						res.ResolvedURL = gResp.Request.URL.String()
						if redirName := ExtractFileNameFromURL(res.ResolvedURL); shouldUseRedirectFileName(res.FileName, redirName) {
							res.FileName = redirName
						}
					}
					if name := parseContentDispositionFileName(gResp.Header.Get("Content-Disposition")); name != "" {
						res.FileName = name
					}
				} else {
					lastStatus = gResp.StatusCode
					lastStatusText = gResp.Status
				}
			}
		}
	}

	if !gotValidResponse {
		if lastStatus >= 400 {
			return nil, fmt.Errorf("HTTP %d (%s)", lastStatus, lastStatusText)
		}
		if lastErr != nil {
			return nil, lastErr
		}
	}

	return res, nil
}

// IsGenericOrScriptFileName identifica nomes vazios, sem extensão ou com extensão de script web (.php, .aspx, .jsp, .html)
// que devem ser substituídos pelo nome real do arquivo após o probe HTTP.
func IsGenericOrScriptFileName(name string) bool {
	clean := strings.TrimSpace(name)
	if clean == "" || strings.EqualFold(clean, "download.bin") {
		return true
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(clean), "."))
	if ext == "" {
		return true
	}
	return ext == "html" || ext == "htm" || ext == "php" || ext == "asp" || ext == "aspx" || ext == "jsp" || ext == "cgi" || ext == "do" || ext == "action"
}

// shouldUseRedirectFileName evita que hashes sem extensão de CDNs (ex: /blobs/69dc9a3d5279...) sobrescrevam nomes válidos.
func shouldUseRedirectFileName(currentName, redirName string) bool {
	if redirName == "" || redirName == "download.bin" {
		return false
	}
	if !IsGenericOrScriptFileName(redirName) {
		return true
	}
	// Se o nome pós-redirect NÃO tem extensão válida, só usa se o nome atual estiver vazio ou genérico.
	return currentName == "" || currentName == "download.bin"
}

// runDownloadTask executa o ciclo completo de um download com segmentação dinâmica e checkpoints.
func (m *Manager) runDownloadTask(ctx context.Context, dl *models.Download) {
	settings, _ := m.store.GetSettings()
	normalizedURL := NormalizeDownloadURL(dl.URL)
	if normalizedURL != "" && normalizedURL != dl.URL {
		dl.URL = normalizedURL
		m.updateDownloadState(dl.ID, func(d *models.Download) {
			d.URL = normalizedURL
		})
	}
	login := m.findLoginForURL(dl.URL)
	siteRule := m.findRuleForURL(dl.URL)

	maxConns := dl.MaxConnections
	if maxConns <= 0 {
		maxConns = settings.MaxConnectionsPerDownload
	}
	if siteRule != nil && siteRule.MaxConnections > 0 && siteRule.MaxConnections < maxConns {
		maxConns = siteRule.MaxConnections
	}
	if maxConns < 1 {
		maxConns = 1
	}
	if maxConns > 32 {
		maxConns = 32
	}

	retryLimit := 10
	retryInterval := 30 * time.Second
	if siteRule != nil {
		if siteRule.RetryLimit > 0 {
			retryLimit = siteRule.RetryLimit
		}
		if siteRule.RetryIntervalSec > 0 {
			retryInterval = time.Duration(siteRule.RetryIntervalSec) * time.Second
		}
	}

	client := BuildHTTPClient(settings, maxConns)

	// Atualiza estado inicial para Probing.
	m.updateDownloadState(dl.ID, func(d *models.Download) {
		d.Status = models.StatusProbing
		d.ErrorMessage = ""
		m.appendEventLocked(d, 0, "info", fmt.Sprintf("Conectando a %s...", extractHost(d.URL)))
	})

	probe, err := ProbeURLWithReferrer(ctx, client, dl.URL, login, dl.Referrer)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		m.handleTaskFailure(dl.ID, fmt.Sprintf("Falha ao sondar servidor: %v", err))
		return
	}

	// Prepara diretórios e nomes de arquivo.
	m.mu.Lock()
	currentDl := m.downloads[dl.ID]
	if currentDl == nil {
		m.mu.Unlock()
		return
	}
	if IsGenericOrScriptFileName(currentDl.FileName) && !IsGenericOrScriptFileName(probe.FileName) {
		currentDl.FileName = probe.FileName
		// Reclassifica a categoria e pasta caso o arquivo tenha ganhado extensão real após o probe do servidor.
		if currentDl.CategoryID == "" || currentDl.CategoryID == "general" {
			cats, _ := m.store.ListCategories()
			matchedCat := matchCategoryByFileName(currentDl.FileName, cats)
			if matchedCat.ID != "" && matchedCat.ID != "general" {
				var generalDefaultDir string
				for _, c := range cats {
					if c.ID == "general" {
						generalDefaultDir = c.DefaultDir
						break
					}
				}
				currentDl.CategoryID = matchedCat.ID
				if matchedCat.DefaultDir != "" && (currentDl.SaveDir == "" ||
					strings.EqualFold(currentDl.SaveDir, settings.DefaultSaveDir) ||
					(generalDefaultDir != "" && strings.EqualFold(currentDl.SaveDir, generalDefaultDir))) {
					currentDl.SaveDir = matchedCat.DefaultDir
				}
			}
		}
	} else if (currentDl.FileName == "" || currentDl.FileName == "download.bin") && probe.FileName != "" {
		currentDl.FileName = probe.FileName
	}
	if currentDl.SaveDir == "" {
		currentDl.SaveDir = settings.DefaultSaveDir
	}
	_ = os.MkdirAll(currentDl.SaveDir, 0o755)
	currentDl.TargetPath = filepath.Join(currentDl.SaveDir, currentDl.FileName)

	minSplit := settings.MinSplitSizeBytes
	if minSplit <= 0 {
		minSplit = 1024 * 1024
	}

	// Se não aceita Range ou o tamanho remoto mudou no servidor, recomeça do zero.
	var existingSegments []models.Segment
	if probe.AcceptRanges && probe.TotalSize > 0 && len(currentDl.Segments) > 0 {
		if currentDl.TotalSize > 0 && currentDl.TotalSize != probe.TotalSize {
			existingSegments = nil
		} else {
			existingSegments = currentDl.Segments
		}
	}

	// Atualiza TotalSize quando for um novo download, reinício ou atualização de sincronização.
	if probe.TotalSize > 0 && (currentDl.TotalSize <= 0 || len(existingSegments) == 0) {
		currentDl.TotalSize = probe.TotalSize
	}
	currentDl.AcceptRanges = probe.AcceptRanges && currentDl.TotalSize > 0
	if probe.ETag != "" {
		currentDl.ETag = probe.ETag
	}
	if probe.LastModified != "" {
		currentDl.LastModified = probe.LastModified
	}

	if !currentDl.AcceptRanges {
		maxConns = 1
		m.appendEventLocked(currentDl, 0, "warn", "Servidor não suporta retomada (Range) ou tamanho indefinido. Usando 1 conexão.")
	} else {
		m.appendEventLocked(currentDl, 0, "info", fmt.Sprintf("Suporte a Range confirmado (%d bytes). Iniciando até %d conexões dinâmicas.", currentDl.TotalSize, maxConns))
	}

	segTable := NewSegmentTable(currentDl.ID, currentDl.TotalSize, minSplit, existingSegments)
	currentDl.Status = models.StatusDownloading
	currentDl.Segments, currentDl.DownloadedSize, _, _, _ = segTable.Snapshot()
	dlSnapshot := *currentDl
	m.mu.Unlock()

	_ = m.store.SaveDownload(&dlSnapshot)
	m.emitUpdate(dl.ID)

	partPath := dlSnapshot.TargetPath + ".oidmpart"
	legacyPartPath := dlSnapshot.TargetPath + ".tdmpart"
	if _, err := os.Stat(partPath); os.IsNotExist(err) {
		if _, errLegacy := os.Stat(legacyPartPath); errLegacy == nil {
			_ = os.Rename(legacyPartPath, partPath)
		}
	}
	openFlags := os.O_CREATE | os.O_RDWR
	if !dlSnapshot.AcceptRanges || len(existingSegments) == 0 {
		openFlags |= os.O_TRUNC
	}
	partFile, err := os.OpenFile(partPath, openFlags, 0o644)
	if err != nil {
		m.handleTaskFailure(dl.ID, fmt.Sprintf("Erro ao criar arquivo parcial: %v", err))
		return
	}
	if dlSnapshot.AcceptRanges && dlSnapshot.TotalSize > 0 {
		_ = partFile.Truncate(dlSnapshot.TotalSize)
	}

	localLimiter := NewSpeedLimiter(dlSnapshot.SpeedLimitBps)
	m.setLocalLimiter(dl.ID, localLimiter)

	// Sub-contexto para a fase multi-conexão: cancelado automaticamente caso ocorra degradação para 1 conexão.
	multiConnCtx, cancelMultiConn := context.WithCancel(ctx)
	defer cancelMultiConn()
	segTable.SetOnDegrade(func() {
		cancelMultiConn()
	})

	// Ticker de checkpoints periódicos no SQLite para garantia de retomada íntegra dos downloads.
	checkpointSec := settings.CheckpointIntervalSec
	if checkpointSec <= 0 {
		checkpointSec = 2
	}
	tickerCtx, stopTicker := context.WithCancel(ctx)
	checkpointDone := make(chan struct{})
	go func() {
		defer close(checkpointDone)
		uiTicker := time.NewTicker(350 * time.Millisecond)
		dbTicker := time.NewTicker(time.Duration(checkpointSec) * time.Second)
		defer uiTicker.Stop()
		defer dbTicker.Stop()

		var prevDownloaded int64 = dlSnapshot.DownloadedSize
		var pendingTraffic int64
		var smoothedSpeed int64
		var zeroStreak int
		lastTick := time.Now()
		for {
			select {
			case <-tickerCtx.Done():
				if pendingTraffic > 0 && m.isDownloadExists(dl.ID) {
					_ = m.store.AddQueueTraffic(dlSnapshot.QueueID, pendingTraffic)
				}
				return
			case <-uiTicker.C:
				if !m.isDownloadExists(dl.ID) {
					return
				}
				now := time.Now()
				dt := now.Sub(lastTick).Seconds()
				lastTick = now

				segs, downloaded, segSpeed, activeW, _ := segTable.Snapshot()
				degraded, degReason := segTable.IsDegraded()
				deltaBytes := downloaded - prevDownloaded
				if deltaBytes > 0 {
					prevDownloaded = downloaded
					pendingTraffic += deltaBytes
					zeroStreak = 0

					var tickSpeed int64
					if dt > 0.05 {
						tickSpeed = int64(float64(deltaBytes) / dt)
					}
					rawSpeed := tickSpeed
					if rawSpeed <= 0 {
						rawSpeed = segSpeed
					} else if smoothedSpeed == 0 && segSpeed > 0 {
						rawSpeed = (rawSpeed + segSpeed) / 2
					}

					if smoothedSpeed > 0 {
						// Média móvel exponencial equilibrada (50/50) para leitura estável sem saltos bruscos
						smoothedSpeed = (smoothedSpeed*5 + rawSpeed*5) / 10
					} else {
						smoothedSpeed = rawSpeed
					}
				} else {
					zeroStreak++
					if activeW > 0 && smoothedSpeed > 0 && zeroStreak <= 6 {
						// Mantém decaimento suave por até ~2s entre leituras de socket para não piscar a velocidade para zero na UI,
						// mas zera caso a conexão realmente estagne por mais de 6 ticks (~2.1s).
						smoothedSpeed = (smoothedSpeed * 78) / 100
						if smoothedSpeed < 512 {
							smoothedSpeed = 0
						}
					} else {
						smoothedSpeed = 0
					}
				}

				m.updateDownloadState(dl.ID, func(d *models.Download) {
					d.Segments = segs
					d.DownloadedSize = downloaded
					d.SpeedBps = smoothedSpeed
					d.ActiveWorkers = activeW
					d.DegradedSingle = degraded
					if degReason != "" {
						d.DegradedReason = degReason
					}
					if d.TotalSize > 0 {
						d.Progress = float64(downloaded) * 100.0 / float64(d.TotalSize)
						if smoothedSpeed > 0 && d.TotalSize > downloaded {
							d.ETASeconds = (d.TotalSize - downloaded) / smoothedSpeed
						} else {
							d.ETASeconds = 0
						}
					}
				})
				m.emitUpdate(dl.ID)
			case <-dbTicker.C:
				if !m.isDownloadExists(dl.ID) {
					return
				}
				if pendingTraffic > 0 {
					_ = m.store.AddQueueTraffic(dlSnapshot.QueueID, pendingTraffic)
					pendingTraffic = 0
				}
				m.persistSnapshot(dl.ID)
			}
		}
	}()

	// Executa o pool de workers de Segmentação Dinâmica.
	var wg sync.WaitGroup
	var workerErrMu sync.Mutex
	var fatalErr error

	for wID := 1; wID <= maxConns; wID++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			reusedFlag := false
			consecutiveRetries := 0

			for {
				if multiConnCtx.Err() != nil || ctx.Err() != nil {
					return
				}
				if isDeg, _ := segTable.IsDegraded(); isDeg {
					return
				}

				seg, ok := segTable.AcquireNextSegment(workerID, reusedFlag)
				if !ok {
					// Verifica se todos os segmentos já foram concluídos ou se degradou.
					_, _, _, activeW, allDone := segTable.Snapshot()
					if allDone {
						return
					}
					if isDeg, _ := segTable.IsDegraded(); isDeg {
						return
					}
					if activeW == 0 {
						// Nenhum segmento pendente nem workers ativos, mas não concluiu: encerra loop.
						return
					}
					// Aguarda oportunidade para dividir um segmento lento ou retomar segmento que falhou.
					select {
					case <-multiConnCtx.Done():
						return
					case <-time.After(150 * time.Millisecond):
						continue
					}
				}

				if reusedFlag {
					m.logConnectionEvent(dl.ID, workerID, "reuse",
						fmt.Sprintf("Conexão #%d reutilizada para novo segmento [%d - %d] sem novo login/TLS", workerID, seg.Current, seg.End))
				} else {
					m.logConnectionEvent(dl.ID, workerID, "info",
						fmt.Sprintf("Conexão #%d iniciada no range [%d - %d]", workerID, seg.Current, seg.End))
				}

				var err error
				var connWasReused bool
				if strings.HasPrefix(strings.ToLower(dlSnapshot.URL), "ftp://") {
					err = downloadFTPSegment(multiConnCtx, dlSnapshot.URL, login, partFile, seg.Start, segTable, m.globalLimiter, localLimiter)
				} else {
					connWasReused, err = m.downloadHTTPSegment(
						multiConnCtx, client, dlSnapshot.URL, login, dlSnapshot.Referrer, partFile,
						seg.Start, dlSnapshot.AcceptRanges, workerID, segTable, localLimiter,
					)
				}

				if err == nil {
					reusedFlag = true
					if connWasReused {
						m.logConnectionEvent(dl.ID, workerID, "reuse",
							fmt.Sprintf("Conexão #%d: socket Keep-Alive reutilizado confirmado pelo transporte HTTP", workerID))
					}
					consecutiveRetries = 0
					continue
				}

				if ctx.Err() != nil {
					segTable.ReleaseSegmentOnError(seg.Start, "pausado")
					return
				}

				// Se a falha ocorreu porque o download degradou para 1 conexão, encerra o worker paralelo imediatamente.
				if isDeg, _ := segTable.IsDegraded(); isDeg || multiConnCtx.Err() != nil {
					return
				}

				// Em falha de rede, devolve o segmento para Pending a partir do último byte gravado (Current).
				segTable.ReleaseSegmentOnError(seg.Start, err.Error())
				consecutiveRetries++
				reusedFlag = false

				m.updateDownloadState(dl.ID, func(d *models.Download) {
					m.appendEventLocked(d, workerID, "warn",
						fmt.Sprintf("Conexão #%d falhou (%v). Tentativa %d/%d...", workerID, err, consecutiveRetries, retryLimit))
				})

				if retryLimit > 0 && consecutiveRetries >= retryLimit {
					workerErrMu.Lock()
					if fatalErr == nil {
						fatalErr = fmt.Errorf("limite de tentativas excedido na conexão #%d: %w", workerID, err)
					}
					workerErrMu.Unlock()
					return
				}

				// Backoff rápido nas primeiras falhas transitórias e retryInterval (30s) nas persistentes.
				waitDur := time.Duration(consecutiveRetries) * time.Second
				if waitDur > retryInterval {
					waitDur = retryInterval
				}
				select {
				case <-multiConnCtx.Done():
					return
				case <-time.After(waitDur):
				}
			}
		}(wID)
	}

	wg.Wait()

	// Se o download degradou para 1 conexão durante a execução paralela, aguarda todos os workers saírem (wg.Wait acima)
	// e reinicia o fluxo único [0, totalSize) de forma limpa na conexão #1, sem concorrência de escrita.
	if degraded, degReason := segTable.IsDegraded(); degraded && ctx.Err() == nil {
		fatalErr = nil
		segTable.ResetSingleSegment()
		m.updateDownloadState(dl.ID, func(d *models.Download) {
			d.DegradedSingle = true
			d.DegradedReason = degReason
			d.AcceptRanges = false
		})
		m.logConnectionEvent(dl.ID, 1, "info", "Reiniciando download em modo seguro de 1 conexão do início do arquivo.")

		for attempt := 0; ; attempt++ {
			if ctx.Err() != nil {
				break
			}
			segTable.ResetSingleSegment()
			var err error
			if strings.HasPrefix(strings.ToLower(dlSnapshot.URL), "ftp://") {
				err = downloadFTPSegment(ctx, dlSnapshot.URL, login, partFile, 0, segTable, m.globalLimiter, localLimiter)
			} else {
				_, err = m.downloadHTTPSegment(ctx, client, dlSnapshot.URL, login, dlSnapshot.Referrer, partFile, 0, false, 1, segTable, localLimiter)
			}
			if err == nil {
				break
			}
			if ctx.Err() != nil {
				break
			}
			if retryLimit > 0 && attempt+1 >= retryLimit {
				fatalErr = fmt.Errorf("limite de tentativas excedido em modo 1 conexão: %w", err)
				break
			}
			waitDur := time.Duration(attempt+1) * time.Second
			if waitDur > retryInterval {
				waitDur = retryInterval
			}
			select {
			case <-ctx.Done():
			case <-time.After(waitDur):
			}
		}
	}

	stopTicker()
	<-checkpointDone
	if !m.isDownloadExists(dl.ID) {
		_ = partFile.Close()
		return
	}
	_ = partFile.Sync()
	_ = partFile.Close()

	// Se foi pausado ou cancelado pelo usuário, salva o checkpoint exato e sai.
	if ctx.Err() != nil {
		segs, downloaded, _, _, _ := segTable.Snapshot()
		m.updateDownloadState(dl.ID, func(d *models.Download) {
			d.Segments = segs
			d.DownloadedSize = downloaded
			d.SpeedBps = 0
			d.ActiveWorkers = 0
			if d.TotalSize > 0 {
				d.Progress = float64(downloaded) * 100.0 / float64(d.TotalSize)
			}
			if d.Status == models.StatusDownloading || d.Status == models.StatusProbing {
				d.Status = models.StatusPaused
			}
		})
		m.persistSnapshot(dl.ID)
		m.emitUpdate(dl.ID)
		return
	}

	// Verifica se todos os segmentos foram concluídos e se a integridade dos ranges está 100% íntegra.
	segs, downloaded, _, _, allDone := segTable.Snapshot()
	if !allDone || !segTable.ValidateIntegrity() {
		errMsg := "download interrompido antes de completar todos os segmentos"
		if fatalErr != nil {
			errMsg = fatalErr.Error()
		}
		m.handleTaskFailure(dl.ID, errMsg)
		return
	}

	// Fase de montagem final: renomeia arquivo .oidmpart para o destino definitivo.
	m.updateDownloadState(dl.ID, func(d *models.Download) {
		d.Status = models.StatusAssembling
		d.Segments = segs
		d.DownloadedSize = downloaded
		d.Progress = 100
		d.SpeedBps = 0
		d.ActiveWorkers = 0
		m.appendEventLocked(d, 0, "info", "Todos os segmentos concluídos. Finalizando arquivo...")
	})
	m.emitUpdate(dl.ID)

	_ = os.Remove(dlSnapshot.TargetPath)
	if err := os.Rename(partPath, dlSnapshot.TargetPath); err != nil {
		m.handleTaskFailure(dl.ID, fmt.Sprintf("Erro ao finalizar arquivo: %v", err))
		return
	}

	now := time.Now()
	degraded, degReason := segTable.IsDegraded()
	m.updateDownloadState(dl.ID, func(d *models.Download) {
		d.Status = models.StatusCompleted
		d.CompletedAt = &now
		if d.TotalSize <= 0 {
			d.TotalSize = downloaded
		}
		d.DownloadedSize = d.TotalSize
		d.Progress = 100
		d.SpeedBps = 0
		d.ETASeconds = 0
		d.ActiveWorkers = 0
		d.ErrorMessage = ""
		if degraded {
			d.DegradedSingle = true
			if degReason != "" {
				d.DegradedReason = degReason
			}
		}
		m.appendEventLocked(d, 0, "info", "Download concluído com sucesso!")
	})
	m.persistSnapshot(dl.ID)
	m.emitUpdate(dl.ID)

	// Executa pós-processamento opcional (Antivírus externo / abrir arquivo).
	go m.runPostDownloadActions(dl.ID)
}

// downloadHTTPSegment baixa os bytes do segmento via HTTP/HTTPS monitorando reuso de conexão e integridade de Range.
func (m *Manager) downloadHTTPSegment(
	ctx context.Context,
	client *http.Client,
	rawURL string,
	login *models.SiteLogin,
	referrer string,
	file *os.File,
	segStart int64,
	acceptRanges bool,
	workerID int,
	table *SegmentTable,
	localLimiter *SpeedLimiter,
) (bool, error) {
	cur, end, ok := table.GetSegmentBound(segStart)
	if !ok {
		return false, nil
	}
	if end > 0 && cur >= end {
		table.CompleteSegment(segStart)
		return false, nil
	}

	var connReused bool
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			connReused = info.Reused
		},
	}

	reqCtx, cancelReq := context.WithCancel(httptrace.WithClientTrace(ctx, trace))
	defer cancelReq()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, err
	}
	ApplyRequestHeadersWithReferrer(req, login, referrer)

	if acceptRanges && end > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", cur, end-1))
	} else if acceptRanges && cur > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", cur))
	}

	resp, err := client.Do(req)
	if err != nil {
		return connReused, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return connReused, fmt.Errorf("HTTP %d (%s)", resp.StatusCode, resp.Status)
	}

	// Detecção de corrupção/incompatibilidade de Range (servidor ignorou Range num offset > 0 e mandou 200 OK).
	if acceptRanges && cur > 0 && resp.StatusCode == http.StatusOK {
		reason := "Servidor retornou 200 OK para um Range parcial (inconsistência detectada). Degradando para 1 conexão."
		table.DegradeToSingleConnection(reason)
		m.logConnectionEvent(table.downloadID, workerID, "error", reason)
		return connReused, errors.New("incompatibilidade de Range detectada")
	}

	// Valida se o Content-Range retornado bate com o offset solicitado (detecção de corrupção entre conexões).
	if acceptRanges && resp.StatusCode == http.StatusPartialContent {
		if crStart, ok := parseStartFromContentRange(resp.Header.Get("Content-Range")); ok && crStart != cur {
			reason := fmt.Sprintf("Content-Range divergente do esperado (esperado %d, recebido %d). Degradando para 1 conexão.", cur, crStart)
			table.DegradeToSingleConnection(reason)
			m.logConnectionEvent(table.downloadID, workerID, "error", reason)
			return connReused, errors.New("corrupção de Content-Range detectada")
		}
	}

	// Em modo de conexão única (!acceptRanges), sincroniza o tamanho real do corpo GET (ou stream chunked sem Content-Length).
	if !acceptRanges && segStart == 0 {
		if resp.ContentLength > 0 {
			table.UpdateSingleStreamTotalSize(resp.ContentLength)
			m.updateDownloadState(table.downloadID, func(d *models.Download) {
				d.TotalSize = resp.ContentLength
			})
		} else if resp.ContentLength < 0 {
			table.UpdateSingleStreamTotalSize(0)
			m.updateDownloadState(table.downloadID, func(d *models.Download) {
				d.TotalSize = 0
			})
		}
	}

	buf := make([]byte, 32*1024)
	windowStart := time.Now()
	var windowBytes int64
	var currentSpeed int64

	for {
		select {
		case <-ctx.Done():
			return connReused, ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if err := m.globalLimiter.Wait(ctx, n); err != nil {
				return connReused, err
			}
			if err := localLimiter.Wait(ctx, n); err != nil {
				return connReused, err
			}

			writeOffset, _, valid := table.GetSegmentBound(segStart)
			if !valid {
				return connReused, nil
			}

			windowBytes += int64(n)
			elapsed := time.Since(windowStart).Seconds()
			if elapsed >= 0.2 {
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
				return connReused, nil
			}
			if accepted > 0 {
				if _, wErr := file.WriteAt(buf[:accepted], writeOffset); wErr != nil {
					return connReused, wErr
				}
			}

			// Se o segmento atingiu o fim (ou porque terminou naturalmente ou porque foi dividido ao meio por outro worker):
			if reachedEnd {
				// Se restarem poucos bytes no corpo HTTP, drena para preservar o socket Keep-Alive na pool.
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
				return connReused, nil
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				// Verifica se realmente chegou ao fim esperado do segmento.
				curNow, endNow, ok := table.GetSegmentBound(segStart)
				if !ok {
					return connReused, nil
				}
				if endNow <= 0 || curNow >= endNow {
					table.CompleteSegment(segStart)
					return connReused, nil
				}
				return connReused, io.ErrUnexpectedEOF
			}
			return connReused, readErr
		}
	}
}

// NormalizeDownloadURL converte links de visualização de arquivos (HuggingFace /blob/, GitHub /blob/, Google Drive, Dropbox)
// para a URL direta do binário.
func NormalizeDownloadURL(rawURL string) string {
	trimmed := strings.Trim(strings.TrimSpace(rawURL), "\"'<>")
	u, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	host := strings.ToLower(u.Hostname())
	if (host == "huggingface.co" || strings.HasSuffix(host, ".huggingface.co")) && strings.Contains(u.Path, "/blob/") {
		u.Path = strings.Replace(u.Path, "/blob/", "/resolve/", 1)
		return u.String()
	}
	if host == "github.com" && strings.Contains(u.Path, "/blob/") {
		u.Path = strings.Replace(u.Path, "/blob/", "/raw/", 1)
		return u.String()
	}
	if host == "dropbox.com" || strings.HasSuffix(host, ".dropbox.com") {
		q := u.Query()
		if q.Get("dl") == "0" {
			q.Set("dl", "1")
			u.RawQuery = q.Encode()
			return u.String()
		}
		if (strings.HasPrefix(u.Path, "/scl/fi/") || strings.HasPrefix(u.Path, "/s/")) && q.Get("dl") == "" && q.Get("raw") == "" {
			q.Set("dl", "1")
			u.RawQuery = q.Encode()
			return u.String()
		}
	}
	if host == "drive.google.com" {
		if strings.HasPrefix(u.Path, "/file/d/") {
			rest := strings.TrimPrefix(u.Path, "/file/d/")
			if slashIdx := strings.Index(rest, "/"); slashIdx > 0 {
				fileID := rest[:slashIdx]
				if fileID != "" {
					return fmt.Sprintf("https://drive.usercontent.google.com/download?id=%s&export=download&confirm=t", url.QueryEscape(fileID))
				}
			} else if rest != "" {
				return fmt.Sprintf("https://drive.usercontent.google.com/download?id=%s&export=download&confirm=t", url.QueryEscape(rest))
			}
		}
		if (u.Path == "/open" || u.Path == "/uc") && u.Query().Get("id") != "" {
			fileID := strings.TrimSpace(u.Query().Get("id"))
			if fileID != "" {
				return fmt.Sprintf("https://drive.usercontent.google.com/download?id=%s&export=download&confirm=t", url.QueryEscape(fileID))
			}
		}
	}
	return trimmed
}

// ExtractFileNameFromURL extrai um nome de arquivo seguro a partir da URL (incluindo query strings de CDNs assinadas e GitHub codeload).
func ExtractFileNameFromURL(rawURL string) string {
	u, err := url.Parse(strings.Trim(strings.TrimSpace(rawURL), "\"'<>"))
	if err != nil {
		return "download.bin"
	}

	// 1. Verifica se o nome do arquivo está em parâmetros de query string (ex: S3/HuggingFace/GitHub response-content-disposition).
	if u.RawQuery != "" {
		q := u.Query()
		for _, dispKey := range []string{"response-content-disposition", "rscd", "content-disposition"} {
			if dispVal := strings.TrimSpace(q.Get(dispKey)); dispVal != "" {
				if fn := parseContentDispositionFileName(dispVal); fn != "" && fn != "download.bin" {
					return fn
				}
			}
		}
		for _, fnKey := range []string{"filename", "file", "name"} {
			if fnVal := SanitizeDownloadFileName(q.Get(fnKey)); fnVal != "" && fnVal != "download.bin" && filepath.Ext(fnVal) != "" {
				return fnVal
			}
		}
	}

	// 2. Tratamento especial para downloads de repositórios GitHub (codeload.github.com/<owner>/<repo>/zip/refs/heads/<ref> ou github.com/<owner>/<repo>/archive/refs/heads/<ref>.zip).
	host := strings.ToLower(u.Hostname())
	if host == "codeload.github.com" {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 4 {
			repo := parts[1]
			kind := strings.ToLower(parts[2])
			ref := parts[len(parts)-1]
			if strings.Contains(kind, "zip") {
				return sanitizeFileName(repo + "-" + ref + ".zip")
			}
			if strings.Contains(kind, "tar.gz") || strings.Contains(kind, "tgz") {
				return sanitizeFileName(repo + "-" + ref + ".tar.gz")
			}
		}
	}
	if host == "github.com" && strings.Contains(u.Path, "/archive/") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 4 {
			repo := parts[1]
			last := parts[len(parts)-1]
			if filepath.Ext(last) != "" {
				return sanitizeFileName(repo + "-" + last)
			}
		}
	}

	cleanPath := strings.Trim(u.Path, "/\\")
	if cleanPath == "" || cleanPath == "." {
		return "download.bin"
	}
	// Tratamento especial para URLs do SourceForge (/projects/<proj>/files/<path>/download) e MediaFire (/file/<key>/<name>/file)
	if (host == "sourceforge.net" || strings.HasSuffix(host, ".sourceforge.net")) && strings.HasSuffix(strings.ToLower(cleanPath), "/download") {
		trimmedSF := cleanPath[:len(cleanPath)-len("/download")]
		if trimmedSF != "" {
			cleanPath = trimmedSF
		}
	}
	if (host == "mediafire.com" || strings.HasSuffix(host, ".mediafire.com")) && strings.HasSuffix(strings.ToLower(cleanPath), "/file") {
		trimmedMF := cleanPath[:len(cleanPath)-len("/file")]
		if trimmedMF != "" {
			cleanPath = trimmedMF
		}
	}

	base := filepath.Base(cleanPath)
	if base == "" || base == "." || base == "/" || base == "\\" {
		return "download.bin"
	}
	if decoded, err := url.PathUnescape(base); err == nil && decoded != "" {
		base = decoded
	}
	return sanitizeFileName(base)
}

func cleanDispositionValue(val string) string {
	val = strings.TrimSpace(val)
	val = strings.Trim(val, `"'`)
	// Remove prefixo RFC 5987 (ex: UTF-8''nome.safetensors)
	if idx := strings.Index(val, "''"); idx >= 0 && idx <= 16 {
		val = val[idx+2:]
	}
	if decoded, err := url.PathUnescape(val); err == nil && decoded != "" {
		val = decoded
	}
	return SanitizeDownloadFileName(val)
}

func parseContentDispositionFileName(headerVal string) string {
	headerVal = strings.TrimRight(strings.TrimSpace(headerVal), "; \t\r\n")
	if headerVal == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(headerVal)
	if err != nil {
		_, params, err = mime.ParseMediaType("attachment; " + headerVal)
	}
	if err == nil && params != nil {
		if fn := cleanDispositionValue(params["filename"]); fn != "" && fn != "download.bin" {
			return fn
		}
		if fn := cleanDispositionValue(params["filename*"]); fn != "" && fn != "download.bin" {
			return fn
		}
	}

	// Fallback tolerante para cabeçalhos não estritos (ex: múltiplos pontos-e-vírgula ou filename*="UTF-8''..." no HuggingFace/S3).
	parts := strings.Split(headerVal, ";")
	var starCandidate string
	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(kv[0]))
		v := cleanDispositionValue(kv[1])
		if v == "" || v == "download.bin" {
			continue
		}
		if k == "filename" {
			return v
		}
		if k == "filename*" {
			starCandidate = v
		}
	}
	return starCandidate
}

func parseTotalFromContentRange(cr string) int64 {
	// Formato esperado: "bytes 0-0/123456"
	slash := strings.LastIndex(cr, "/")
	if slash < 0 || slash+1 >= len(cr) {
		return 0
	}
	totalStr := strings.TrimSpace(cr[slash+1:])
	if totalStr == "*" {
		return 0
	}
	v, err := strconv.ParseInt(totalStr, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseStartFromContentRange(cr string) (int64, bool) {
	// Formato esperado: "bytes 1024-2047/4096"
	cr = strings.TrimSpace(cr)
	if !strings.HasPrefix(strings.ToLower(cr), "bytes ") {
		return 0, false
	}
	spec := strings.TrimSpace(cr[6:])
	dash := strings.Index(spec, "-")
	if dash <= 0 {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.TrimSpace(spec[:dash]), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// SanitizeDownloadFileName extrai apenas o nome base de um caminho (ex: vindo do navegador) e remove caracteres inválidos.
func SanitizeDownloadFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// Normaliza separadores Windows/Unix para extrair somente o nome base do arquivo.
	normalized := strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(normalized, "/"); idx >= 0 {
		name = normalized[idx+1:]
	}
	if name == "" || name == "." || name == ".." {
		return ""
	}
	return sanitizeFileName(name)
}

func sanitizeFileName(name string) string {
	invalid := []string{"<", ">", ":", "\"", "/", "\\", "|", "?", "*"}
	for _, ch := range invalid {
		name = strings.ReplaceAll(name, ch, "_")
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "download.bin"
	}
	return name
}

func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}
