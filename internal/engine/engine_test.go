package engine

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"openidm/internal/models"
	"openidm/internal/store"
)

// TestDynamicSegmentationSplitAndIntegrity valida a divisão adaptativa do maior segmento em downloads concorrentes.
func TestDynamicSegmentationSplitAndIntegrity(t *testing.T) {
	totalSize := int64(8 * 1024 * 1024) // 8 MB
	minSplit := int64(1 * 1024 * 1024)  // 1 MB

	st := NewSegmentTable("dl_test", totalSize, minSplit, nil)

	// Worker 1 pega o arquivo inteiro inicialmente [0, 8MB).
	seg1, ok := st.AcquireNextSegment(1, false)
	if !ok || seg1.Start != 0 || seg1.End != totalSize {
		t.Fatalf("esperado segmento inicial [0, %d), obtido: %+v (ok=%v)", totalSize, seg1, ok)
	}

	// Worker 2 entra ocioso e divide o segmento do Worker 1 ao meio: [0, 4MB) e [4MB, 8MB).
	seg2, ok := st.AcquireNextSegment(2, false)
	if !ok || seg2.Start != 4*1024*1024 || seg2.End != totalSize {
		t.Fatalf("esperado segundo segmento [4MB, 8MB), obtido: %+v", seg2)
	}

	// Worker 3 entra e divide o maior segmento disponível em [2MB, 4MB) ou [6MB, 8MB).
	seg3, ok := st.AcquireNextSegment(3, false)
	if !ok {
		t.Fatalf("esperado terceiro segmento dividido com sucesso")
	}

	// Simula conclusão rápida do seg3 pelo Worker 3 enquanto seg1 ainda tem bytes pendentes.
	rem3 := seg3.End - seg3.Start
	_, reachedEnd, _ := st.AdvanceSegment(seg3.Start, rem3, 1000)
	if !reachedEnd {
		t.Fatalf("esperado reachedEnd=true para seg3")
	}

	// Worker 3 agora reutiliza sua conexão (reused=true) para ajudar outro worker dividindo o maior segmento restante!
	seg4, ok := st.AcquireNextSegment(3, true)
	if !ok || !seg4.ReusedConn {
		t.Fatalf("esperado que Worker 3 reutilizasse conexão e dividisse segmento restante: %+v (ok=%v)", seg4, ok)
	}

	// Conclui todos os segmentos atuais no mapa.
	segs, _, _, _, _ := st.Snapshot()
	for _, s := range segs {
		rem := s.End - s.Current
		if rem > 0 {
			st.AdvanceSegment(s.Start, rem, 500)
		}
	}

	if !st.ValidateIntegrity() {
		t.Fatalf("ValidateIntegrity deveria ser verdadeiro após conclusão de todos os segmentos")
	}
}

// TestMinSplitBoundary garante que segmentos menores que MIN_SPLIT não sejam divididos.
func TestMinSplitBoundary(t *testing.T) {
	totalSize := int64(512 * 1024)     // 512 KB
	minSplit := int64(1 * 1024 * 1024) // 1 MB

	st := NewSegmentTable("dl_small", totalSize, minSplit, nil)
	_, ok1 := st.AcquireNextSegment(1, false)
	if !ok1 {
		t.Fatalf("primeiro worker deveria adquirir o segmento")
	}

	// Segundo worker não deve dividir porque 512 KB < 1 MB (MIN_SPLIT).
	_, ok2 := st.AcquireNextSegment(2, false)
	if ok2 {
		t.Fatalf("não deveria dividir segmento menor que MIN_SPLIT")
	}
}

// TestManagerMultiConnectionDownloadAndResume testa um download real HTTP 206 com múltiplas conexões e verificação SHA-256.
func TestManagerMultiConnectionDownloadAndResume(t *testing.T) {
	// Gera um payload determinístico de 4 MB.
	payloadSize := 4 * 1024 * 1024
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte((i * 31) % 251)
	}
	expectedHash := sha256.Sum256(payload)

	var rangeRequests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"v1-etag"`)
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(payloadSize))
			w.WriteHeader(http.StatusOK)
			return
		}
		rangeHdr := r.Header.Get("Range")
		if strings.HasPrefix(rangeHdr, "bytes=") {
			atomic.AddInt32(&rangeRequests, 1)
			spec := strings.TrimPrefix(rangeHdr, "bytes=")
			parts := strings.Split(spec, "-")
			start, _ := strconv.Atoi(parts[0])
			end := payloadSize - 1
			if len(parts) > 1 && parts[1] != "" {
				end, _ = strconv.Atoi(parts[1])
			}
			if start < 0 || start >= payloadSize || end >= payloadSize || start > end {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			chunk := payload[start : end+1]
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, payloadSize))
			w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(chunk)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(payloadSize))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.sqlite")
	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("erro ao criar store: %v", err)
	}
	defer st.Close()

	// Ajusta MIN_SPLIT para 256 KB para exercitar múltiplas divisões dinâmicas num arquivo de 4 MB.
	cfg, _ := st.GetSettings()
	cfg.MinSplitSizeBytes = 256 * 1024
	cfg.MaxConnectionsPerDownload = 6
	_ = st.SaveSettings(cfg)

	mgr, err := NewManager(st)
	if err != nil {
		t.Fatalf("erro ao criar manager: %v", err)
	}

	dl, err := mgr.AddDownload(models.AddDownloadRequest{
		URL:            srv.URL + "/arquivo-teste.bin",
		FileName:       "arquivo-teste.bin",
		SaveDir:        tmpDir,
		MaxConnections: 6,
		StartNow:       true,
	})
	if err != nil {
		t.Fatalf("erro ao adicionar download: %v", err)
	}

	// Aguarda conclusão do download.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := mgr.GetDownload(dl.ID)
		if cur.Status == models.StatusCompleted {
			break
		}
		if cur.Status == models.StatusError {
			t.Fatalf("download falhou: %s", cur.ErrorMessage)
		}
		time.Sleep(50 * time.Millisecond)
	}

	finalDl, _ := mgr.GetDownload(dl.ID)
	if finalDl.Status != models.StatusCompleted {
		t.Fatalf("download não concluiu no tempo esperado, status=%s", finalDl.Status)
	}

	if atomic.LoadInt32(&rangeRequests) < 2 {
		t.Fatalf("esperado múltiplas requisições Range paralelas, obtido=%d", rangeRequests)
	}

	// Verifica integridade SHA-256 do arquivo montado.
	diskBytes, err := os.ReadFile(finalDl.TargetPath)
	if err != nil {
		t.Fatalf("erro ao ler arquivo final: %v", err)
	}
	actualHash := sha256.Sum256(diskBytes)
	if !bytes.Equal(expectedHash[:], actualHash[:]) {
		t.Fatalf("hash SHA-256 do arquivo final não confere com o original")
	}
}

// TestFallbackWithoutRangeSupport verifica o fallback para 1 conexão quando o servidor não suporta Range.
func TestFallbackWithoutRangeSupport(t *testing.T) {
	payload := []byte("conteudo-sem-suporte-a-range-1234567890")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Servidor ignora cabeçalho Range e responde sempre 200 OK sem Accept-Ranges.
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(payload)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	st, err := store.NewStore(filepath.Join(tmpDir, "fallback.sqlite"))
	if err != nil {
		t.Fatalf("erro store: %v", err)
	}
	defer st.Close()

	mgr, _ := NewManager(st)
	dl, err := mgr.AddDownload(models.AddDownloadRequest{
		URL:            srv.URL + "/stream.txt",
		FileName:       "stream.txt",
		SaveDir:        tmpDir,
		MaxConnections: 8,
		StartNow:       true,
	})
	if err != nil {
		t.Fatalf("erro AddDownload: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := mgr.GetDownload(dl.ID)
		if cur.Status == models.StatusCompleted {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}

	cur, _ := mgr.GetDownload(dl.ID)
	if cur.Status != models.StatusCompleted {
		t.Fatalf("esperado completed no modo fallback, obtido %s (%s)", cur.Status, cur.ErrorMessage)
	}
	if cur.AcceptRanges {
		t.Fatalf("esperado AcceptRanges=false")
	}
	got, _ := os.ReadFile(cur.TargetPath)
	if !bytes.Equal(got, payload) {
		t.Fatalf("conteúdo baixado difere do esperado")
	}
}

// TestPauseAndResumeFromCheckpoint simula a interrupção (crash/pause) com checkpoint parcial salvo no SQLite e retomada exata.
func TestPauseAndResumeFromCheckpoint(t *testing.T) {
	payloadSize := 2 * 1024 * 1024 // 2 MB
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte((i * 17) % 253)
	}
	expectedHash := sha256.Sum256(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(payloadSize))
			w.WriteHeader(http.StatusOK)
			return
		}
		rangeHdr := r.Header.Get("Range")
		if strings.HasPrefix(rangeHdr, "bytes=") {
			spec := strings.TrimPrefix(rangeHdr, "bytes=")
			parts := strings.Split(spec, "-")
			start, _ := strconv.Atoi(parts[0])
			end := payloadSize - 1
			if len(parts) > 1 && parts[1] != "" {
				end, _ = strconv.Atoi(parts[1])
			}
			chunk := payload[start : end+1]
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, payloadSize))
			w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(chunk)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "resume.sqlite")
	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("erro store: %v", err)
	}
	defer st.Close()

	targetPath := filepath.Join(tmpDir, "retomada.bin")
	partPath := targetPath + ".oidmpart"

	// Simula que metade inicial [0, 1MB) já foi gravada no arquivo parcial .oidmpart antes de uma queda de energia.
	half := payloadSize / 2
	if err := os.WriteFile(partPath, payload[:half], 0o644); err != nil {
		t.Fatalf("erro ao criar .oidmpart: %v", err)
	}

	dl := &models.Download{
		ID:             "dl_resume_test",
		URL:            srv.URL + "/retomada.bin",
		FileName:       "retomada.bin",
		SaveDir:        tmpDir,
		TargetPath:     targetPath,
		CategoryID:     "general",
		QueueID:        "main",
		TotalSize:      int64(payloadSize),
		DownloadedSize: int64(half),
		Progress:       50.0,
		Status:         models.StatusDownloading, // Simula crash enquanto estava "downloading"
		AcceptRanges:   true,
		MaxConnections: 4,
		Segments: []models.Segment{
			{
				DownloadID: "dl_resume_test",
				Index:      0,
				Start:      0,
				Current:    int64(half),
				End:        int64(half),
				Status:     models.SegmentCompleted,
			},
			{
				DownloadID: "dl_resume_test",
				Index:      1,
				Start:      int64(half),
				Current:    int64(half),
				End:        int64(payloadSize),
				Status:     models.SegmentDownloading,
			},
		},
	}
	if err := st.SaveDownload(dl); err != nil {
		t.Fatalf("erro SaveDownload: %v", err)
	}

	// Ao criar um novo Manager sob o mesmo banco SQLite, o download interrompido deve ir para Paused preservando os checkpoints.
	mgr, err := NewManager(st)
	if err != nil {
		t.Fatalf("erro NewManager: %v", err)
	}
	restored, err := mgr.GetDownload("dl_resume_test")
	if err != nil || restored.Status != models.StatusPaused {
		t.Fatalf("esperado StatusPaused após crash recovery, obtido: %+v", restored)
	}

	// Retoma o download a partir do checkpoint salvo.
	if err := mgr.StartDownload("dl_resume_test"); err != nil {
		t.Fatalf("erro StartDownload: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := mgr.GetDownload("dl_resume_test")
		if cur.Status == models.StatusCompleted {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}

	finalDl, _ := mgr.GetDownload("dl_resume_test")
	if finalDl.Status != models.StatusCompleted {
		t.Fatalf("download retomado não concluiu: %s (%s)", finalDl.Status, finalDl.ErrorMessage)
	}

	diskBytes, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("erro ao ler arquivo retomado: %v", err)
	}
	actualHash := sha256.Sum256(diskBytes)
	if !bytes.Equal(expectedHash[:], actualHash[:]) {
		t.Fatalf("hash SHA-256 após retomada não confere")
	}
}

// TestDegradeToSingleConnectionOn200InSecondaryRange testa o cenário em que o servidor anuncia Accept-Ranges,
// mas retorna 200 OK (corpo completo) quando uma conexão secundária solicita um Range com offset > 0.
func TestDegradeToSingleConnectionOn200InSecondaryRange(t *testing.T) {
	payloadSize := 2 * 1024 * 1024 // 2 MB
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte((i * 43) % 251)
	}
	expectedHash := sha256.Sum256(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(payloadSize))
			w.WriteHeader(http.StatusOK)
			return
		}
		rangeHdr := r.Header.Get("Range")
		// Responde 206 apenas para bytes=0-..., mas ignora Range e manda 200 OK quando start > 0!
		if strings.HasPrefix(rangeHdr, "bytes=0-") {
			spec := strings.TrimPrefix(rangeHdr, "bytes=0-")
			end := payloadSize - 1
			if spec != "" {
				end, _ = strconv.Atoi(spec)
			}
			chunk := payload[:end+1]
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", end, payloadSize))
			w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(chunk)
			return
		}
		// Conexão secundária com offset > 0 recebe 200 OK (inconsistência de servidor quebrado).
		w.Header().Set("Content-Length", strconv.Itoa(payloadSize))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	st, err := store.NewStore(filepath.Join(tmpDir, "degrade.sqlite"))
	if err != nil {
		t.Fatalf("erro store: %v", err)
	}
	defer st.Close()

	cfg, _ := st.GetSettings()
	cfg.MinSplitSizeBytes = 256 * 1024
	cfg.MaxConnectionsPerDownload = 4
	_ = st.SaveSettings(cfg)

	mgr, _ := NewManager(st)
	dl, err := mgr.AddDownload(models.AddDownloadRequest{
		URL:            srv.URL + "/degrade.bin",
		FileName:       "degrade.bin",
		SaveDir:        tmpDir,
		MaxConnections: 4,
		StartNow:       true,
	})
	if err != nil {
		t.Fatalf("erro AddDownload: %v", err)
	}

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := mgr.GetDownload(dl.ID)
		if cur.Status == models.StatusCompleted {
			break
		}
		if cur.Status == models.StatusError {
			t.Fatalf("download não deveria falhar ao degradar para 1 conexão: %s", cur.ErrorMessage)
		}
		time.Sleep(40 * time.Millisecond)
	}

	finalDl, _ := mgr.GetDownload(dl.ID)
	if finalDl.Status != models.StatusCompleted {
		t.Fatalf("esperado StatusCompleted após degradação para 1 conexão, obtido: %s (%s)", finalDl.Status, finalDl.ErrorMessage)
	}
	if !finalDl.DegradedSingle {
		t.Fatalf("esperado DegradedSingle=true no download finalizado")
	}

	diskBytes, err := os.ReadFile(finalDl.TargetPath)
	if err != nil {
		t.Fatalf("erro ao ler arquivo degradado: %v", err)
	}
	actualHash := sha256.Sum256(diskBytes)
	if !bytes.Equal(expectedHash[:], actualHash[:]) {
		t.Fatalf("hash SHA-256 do arquivo degradado para 1 conexão não confere")
	}
}

// TestFTPMultiSegmentDownload inicia um servidor FTP PASV/EPSV em memória e valida download multi-conexão com REST.
func TestFTPMultiSegmentDownload(t *testing.T) {
	payloadSize := 1024 * 1024 // 1 MB
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte((i * 29) % 251)
	}
	expectedHash := sha256.Sum256(payload)

	ftpAddr, stopFTP := startMockFTPServer(t, payload)
	defer stopFTP()

	tmpDir := t.TempDir()
	st, err := store.NewStore(filepath.Join(tmpDir, "ftp.sqlite"))
	if err != nil {
		t.Fatalf("erro store: %v", err)
	}
	defer st.Close()

	cfg, _ := st.GetSettings()
	cfg.MinSplitSizeBytes = 256 * 1024
	_ = st.SaveSettings(cfg)

	mgr, _ := NewManager(st)
	dl, err := mgr.AddDownload(models.AddDownloadRequest{
		URL:            "ftp://" + ftpAddr + "/arquivo-ftp.bin",
		FileName:       "arquivo-ftp.bin",
		SaveDir:        tmpDir,
		MaxConnections: 4,
		StartNow:       true,
	})
	if err != nil {
		t.Fatalf("erro AddDownload FTP: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := mgr.GetDownload(dl.ID)
		if cur.Status == models.StatusCompleted {
			break
		}
		if cur.Status == models.StatusError {
			t.Fatalf("download FTP falhou: %s", cur.ErrorMessage)
		}
		time.Sleep(40 * time.Millisecond)
	}

	finalDl, _ := mgr.GetDownload(dl.ID)
	if finalDl.Status != models.StatusCompleted {
		t.Fatalf("download FTP não concluiu no tempo esperado, status=%s", finalDl.Status)
	}

	diskBytes, err := os.ReadFile(finalDl.TargetPath)
	if err != nil {
		t.Fatalf("erro ao ler arquivo FTP: %v", err)
	}
	actualHash := sha256.Sum256(diskBytes)
	if !bytes.Equal(expectedHash[:], actualHash[:]) {
		t.Fatalf("hash SHA-256 do arquivo baixado via FTP PASV não confere")
	}
}

// TestSOCKS5AndPACProxyDownload valida downloads através de um servidor proxy SOCKS5 autenticado e via script PAC.
func TestSOCKS5AndPACProxyDownload(t *testing.T) {
	payload := []byte("conteudo-transferido-via-proxy-socks5-e-pac-com-sucesso")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(payload)
		}
	}))
	defer srv.Close()

	socksHost, socksPort, socksHits, stopSocks := startMockSOCKS5Server(t, "proxyuser", "proxypass")
	defer stopSocks()

	pacSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `function FindProxyForURL(url, host) { return "SOCKS5 %s:%d; DIRECT"; }`, socksHost, socksPort)
	}))
	defer pacSrv.Close()

	tmpDir := t.TempDir()
	st, err := store.NewStore(filepath.Join(tmpDir, "proxy.sqlite"))
	if err != nil {
		t.Fatalf("erro store: %v", err)
	}
	defer st.Close()

	// Configura Proxy via PAC apontando para o SOCKS5 autenticado.
	cfg, _ := st.GetSettings()
	cfg.ProxyType = "pac"
	cfg.ProxyPACUrl = pacSrv.URL + "/proxy.pac"
	cfg.ProxyUser = "proxyuser"
	cfg.ProxyPass = "proxypass"
	_ = st.SaveSettings(cfg)

	mgr, _ := NewManager(st)
	dl, err := mgr.AddDownload(models.AddDownloadRequest{
		URL:            srv.URL + "/via-proxy.bin",
		FileName:       "via-proxy.bin",
		SaveDir:        tmpDir,
		MaxConnections: 1,
		StartNow:       true,
	})
	if err != nil {
		t.Fatalf("erro AddDownload via proxy: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := mgr.GetDownload(dl.ID)
		if cur.Status == models.StatusCompleted {
			break
		}
		if cur.Status == models.StatusError {
			t.Fatalf("download via proxy falhou: %s", cur.ErrorMessage)
		}
		time.Sleep(40 * time.Millisecond)
	}

	if atomic.LoadInt32(socksHits) == 0 {
		t.Fatalf("esperado que o tráfego passasse pelo servidor SOCKS5 configurado via PAC")
	}

	got, err := os.ReadFile(filepath.Join(tmpDir, "via-proxy.bin"))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("conteúdo baixado via proxy difere do esperado")
	}
}

// TestPauseImmediateResumeAndDeleteActiveFileLock testa Pause seguido imediatamente de Start e exclusão de arquivo ativo no Windows.
func TestPauseImmediateResumeAndDeleteActiveFileLock(t *testing.T) {
	payloadSize := 512 * 1024
	payload := bytes.Repeat([]byte("A"), payloadSize)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(payloadSize))
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(payloadSize))
		w.WriteHeader(http.StatusOK)
		// Envia em pequenos pedaços com pausa para simular download longo em andamento
		for off := 0; off < len(payload); off += 16384 {
			if _, err := w.Write(payload[off : off+16384]); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(15 * time.Millisecond)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	st, _ := store.NewStore(filepath.Join(tmpDir, "lifecycle.sqlite"))
	defer st.Close()

	mgr, _ := NewManager(st)
	dl, err := mgr.AddDownload(models.AddDownloadRequest{
		URL:            srv.URL + "/lento.bin",
		FileName:       "C:\\Users\\rafam\\Downloads\\lento.bin", // Testa também sanitização de caminho completo do Chrome
		SaveDir:        tmpDir,
		MaxConnections: 2,
		StartNow:       true,
	})
	if err != nil {
		t.Fatalf("erro AddDownload: %v", err)
	}
	if dl.FileName != "lento.bin" {
		t.Fatalf("esperado FileName sanitizado 'lento.bin', obtido '%s'", dl.FileName)
	}

	time.Sleep(60 * time.Millisecond)

	// Pausa e retoma imediatamente (sem sleep) para garantir que StartDownload não ignore a chamada.
	if err := mgr.PauseDownload(dl.ID); err != nil {
		t.Fatalf("erro PauseDownload: %v", err)
	}
	if err := mgr.StartDownload(dl.ID); err != nil {
		t.Fatalf("erro StartDownload imediato: %v", err)
	}

	time.Sleep(60 * time.Millisecond)

	// Exclui o download enquanto está ativo e verifica se o .oidmpart foi fechado e removido no Windows.
	partPath := dl.TargetPath + ".oidmpart"
	if err := mgr.DeleteDownload(dl.ID, true); err != nil {
		t.Fatalf("erro DeleteDownload: %v", err)
	}
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Fatalf("arquivo parcial .oidmpart deveria ter sido removido após DeleteDownload no Windows, err=%v", err)
	}
}

// TestParseCommandArgsWithSpaces garante que caminhos com espaços (ex: diretórios Windows) não sejam quebrados.
func TestParseCommandArgsWithSpaces(t *testing.T) {
	target := `d:\RAFATECH PROJETOS\APLICATIVOS\DOWNLOAD-MANAGER\meu arquivo.zip`
	args := ParseCommandArgs(`-Scan -ScanType 3 -File "[file]"`, target)
	if len(args) != 5 || args[4] != target {
		t.Fatalf("ParseCommandArgs falhou para caminho com espaços: %#v", args)
	}
}

func startMockFTPServer(t *testing.T, payload []byte) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("erro ao iniciar mock FTP: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleMockFTPConn(conn, payload)
		}
	}()

	return ln.Addr().String(), func() { _ = ln.Close() }
}

func handleMockFTPConn(conn net.Conn, payload []byte) {
	defer conn.Close()
	_, _ = conn.Write([]byte("220 MockFTP Ready\r\n"))

	scanner := bufio.NewScanner(conn)
	var restOffset int64
	var dataLn net.Listener
	defer func() {
		if dataLn != nil {
			_ = dataLn.Close()
		}
	}()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, " ", 2)
		cmd := strings.ToUpper(parts[0])
		arg := ""
		if len(parts) > 1 {
			arg = parts[1]
		}

		switch cmd {
		case "USER":
			_, _ = conn.Write([]byte("331 User OK\r\n"))
		case "PASS":
			_, _ = conn.Write([]byte("230 Login OK\r\n"))
		case "FEAT":
			_, _ = conn.Write([]byte("211-Features:\r\n SIZE\r\n REST STREAM\r\n EPSV\r\n PASV\r\n211 End\r\n"))
		case "TYPE":
			_, _ = conn.Write([]byte("200 Type set\r\n"))
		case "SIZE":
			_, _ = fmt.Fprintf(conn, "213 %d\r\n", len(payload))
		case "REST":
			restOffset, _ = strconv.ParseInt(arg, 10, 64)
			_, _ = fmt.Fprintf(conn, "350 Restarting at %d\r\n", restOffset)
		case "EPSV":
			if dataLn != nil {
				_ = dataLn.Close()
			}
			dataLn, _ = net.Listen("tcp", "127.0.0.1:0")
			port := dataLn.Addr().(*net.TCPAddr).Port
			_, _ = fmt.Fprintf(conn, "229 Entering Extended Passive Mode (|||%d|)\r\n", port)
		case "PASV":
			if dataLn != nil {
				_ = dataLn.Close()
			}
			dataLn, _ = net.Listen("tcp", "127.0.0.1:0")
			port := dataLn.Addr().(*net.TCPAddr).Port
			_, _ = fmt.Fprintf(conn, "227 Entering Passive Mode (127,0,0,1,%d,%d)\r\n", port/256, port%256)
		case "RETR":
			_, _ = conn.Write([]byte("150 Opening binary data connection\r\n"))
			if dataLn != nil {
				dConn, err := dataLn.Accept()
				if err == nil {
					if restOffset >= 0 && restOffset < int64(len(payload)) {
						_, _ = dConn.Write(payload[restOffset:])
					}
					_ = dConn.Close()
				}
				_ = dataLn.Close()
				dataLn = nil
			}
			restOffset = 0
			_, _ = conn.Write([]byte("226 Transfer complete\r\n"))
		case "QUIT":
			_, _ = conn.Write([]byte("221 Goodbye\r\n"))
			return
		default:
			_, _ = conn.Write([]byte("200 OK\r\n"))
		}
	}
}

func startMockSOCKS5Server(t *testing.T, expectedUser, expectedPass string) (string, int, *int32, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("erro ao iniciar mock SOCKS5: %v", err)
	}
	var hits int32
	addr := ln.Addr().(*net.TCPAddr)

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				hdr := make([]byte, 2)
				if _, err := io.ReadFull(conn, hdr); err != nil {
					return
				}
				methods := make([]byte, hdr[1])
				if _, err := io.ReadFull(conn, methods); err != nil {
					return
				}
				// Exige autenticação Username/Password (0x02)
				_, _ = conn.Write([]byte{0x05, 0x02})

				authHdr := make([]byte, 2)
				if _, err := io.ReadFull(conn, authHdr); err != nil {
					return
				}
				uBuf := make([]byte, authHdr[1])
				_, _ = io.ReadFull(conn, uBuf)
				pLen := make([]byte, 1)
				_, _ = io.ReadFull(conn, pLen)
				pBuf := make([]byte, pLen[0])
				_, _ = io.ReadFull(conn, pBuf)

				if string(uBuf) != expectedUser || string(pBuf) != expectedPass {
					_, _ = conn.Write([]byte{0x01, 0x01})
					return
				}
				_, _ = conn.Write([]byte{0x01, 0x00})

				req := make([]byte, 4)
				if _, err := io.ReadFull(conn, req); err != nil {
					return
				}
				var targetHost string
				switch req[3] {
				case 0x01: // IPv4
					ip := make([]byte, 4)
					_, _ = io.ReadFull(conn, ip)
					targetHost = net.IP(ip).String()
				case 0x03: // Domain
					dLen := make([]byte, 1)
					_, _ = io.ReadFull(conn, dLen)
					dom := make([]byte, dLen[0])
					_, _ = io.ReadFull(conn, dom)
					targetHost = string(dom)
				}
				portBuf := make([]byte, 2)
				_, _ = io.ReadFull(conn, portBuf)
				targetPort := binary.BigEndian.Uint16(portBuf)

				dst, err := net.Dial("tcp", net.JoinHostPort(targetHost, strconv.Itoa(int(targetPort))))
				if err != nil {
					return
				}
				defer dst.Close()
				atomic.AddInt32(&hits, 1)

				_, _ = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0, 0})
				go func() { _, _ = io.Copy(dst, conn) }()
				_, _ = io.Copy(conn, dst)
			}(c)
		}
	}()

	return "127.0.0.1", addr.Port, &hits, func() { _ = ln.Close() }
}


