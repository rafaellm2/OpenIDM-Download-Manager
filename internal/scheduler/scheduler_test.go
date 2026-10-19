package scheduler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"openidm/internal/engine"
	"openidm/internal/models"
	"openidm/internal/store"
)

// TestSchedulerTrafficQuotaStopsQueue verifica se a fila é pausada automaticamente ao atingir a quota de tráfego em MB.
func TestSchedulerTrafficQuotaStopsQueue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sched_test.sqlite")
	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("erro store: %v", err)
	}
	defer st.Close()

	mgr, err := engine.NewManager(st)
	if err != nil {
		t.Fatalf("erro manager: %v", err)
	}

	sched := NewScheduler(st, mgr)

	// Configura a fila principal como ativa com quota de 2 MB e tráfego já consumido de 3 MB.
	q := models.Queue{
		ID:               "main",
		Name:             "Fila Principal",
		IsMain:           true,
		MaxConcurrent:    2,
		TrafficQuotaMB:   2,
		UsedTrafficBytes: 3 * 1024 * 1024,
		Active:           true,
	}
	if err := st.SaveQueue(q); err != nil {
		t.Fatalf("erro SaveQueue: %v", err)
	}

	// Executa um ciclo do scheduler: deve detectar que UsedTrafficBytes >= TrafficQuotaMB e desativar a fila.
	sched.tick()

	queues, _ := st.ListQueues()
	for _, item := range queues {
		if item.ID == "main" && item.Active {
			t.Fatalf("esperado que a fila fosse parada ao exceder a quota de tráfego")
		}
	}
}

// TestSchedulerSyncQueueDetectsRemoteUpdate verifica se StartQueue("sync") detecta mudança de ETag/tamanho
// em arquivos já concluídos e os re-baixa antes de encerrar a fila de sincronização.
func TestSchedulerSyncQueueDetectsRemoteUpdate(t *testing.T) {
	var version atomic.Int32
	version.Store(1)

	v1Data := []byte("versao-1-conteudo-antigo")
	v2Data := []byte("versao-2-conteudo-novo-atualizado-maior")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := version.Load()
		data := v1Data
		etag := `"etag-v1"`
		if v == 2 {
			data = v2Data
			etag = `"etag-v2"`
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2025 07:28:00 GMT")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sched_sync.sqlite")
	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("erro store: %v", err)
	}
	defer st.Close()

	mgr, err := engine.NewManager(st)
	if err != nil {
		t.Fatalf("erro manager: %v", err)
	}
	sched := NewScheduler(st, mgr)

	// 1. Adiciona e conclui o download da versão 1 na fila "sync".
	dl, err := mgr.AddDownload(models.AddDownloadRequest{
		URL:            srv.URL + "/syncfile.bin",
		FileName:       "syncfile.bin",
		SaveDir:        tmpDir,
		QueueID:        "sync",
		MaxConnections: 1,
		StartNow:       true,
	})
	if err != nil {
		t.Fatalf("erro AddDownload: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := st.GetDownload(dl.ID)
		if cur != nil && cur.Status == models.StatusCompleted {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	// 2. Muda a versão no servidor para v2 (ETag e tamanho diferentes) e inicia a fila "sync".
	version.Store(2)
	if err := sched.StartQueue("sync"); err != nil {
		t.Fatalf("erro StartQueue(sync): %v", err)
	}

	// Aguarda re-download concluir com o novo conteúdo e novo TotalSize.
	deadline = time.Now().Add(5 * time.Second)
	var updated *models.Download
	for time.Now().Before(deadline) {
		cur, _ := st.GetDownload(dl.ID)
		if cur != nil && cur.Status == models.StatusCompleted && cur.ETag == `"etag-v2"` {
			updated = cur
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	if updated == nil {
		cur, _ := st.GetDownload(dl.ID)
		t.Fatalf("esperado que a fila sync atualizasse o arquivo para etag-v2, estado atual: %+v", cur)
	}
	if updated.TotalSize != int64(len(v2Data)) {
		t.Fatalf("TotalSize incorreto após sync: esperado %d, obtido %d", len(v2Data), updated.TotalSize)
	}
	diskBytes, err := os.ReadFile(filepath.Join(updated.SaveDir, updated.FileName))
	if err != nil || string(diskBytes) != string(v2Data) {
		t.Fatalf("conteúdo em disco após sync incorreto: %q (err=%v)", string(diskBytes), err)
	}
}

// TestSchedulerRetryMovesToEndOfQueue verifica se itens com StatusError vão para o fim da fila
// (atrás de itens StatusQueued) e são retentados apenas depois dos itens pendentes normais.
func TestSchedulerRetryMovesToEndOfQueue(t *testing.T) {
	var failItem1 atomic.Bool
	failItem1.Store(true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/item1.bin" && failItem1.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("12345"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	st, err := store.NewStore(filepath.Join(tmpDir, "sched_order.sqlite"))
	if err != nil {
		t.Fatalf("erro store: %v", err)
	}
	defer st.Close()

	_ = st.SaveSiteRule(models.SiteRule{
		HostPattern: "*",
		RetryLimit:  1,
	})

	mgr, err := engine.NewManager(st)
	if err != nil {
		t.Fatalf("erro manager: %v", err)
	}
	sched := NewScheduler(st, mgr)

	// 1. Cria dl1 (que falha na 1ª tentativa e entra em StatusError) e dl2 (em StatusQueued).
	dl1, _ := mgr.AddDownload(models.AddDownloadRequest{
		URL:      srv.URL + "/item1.bin",
		FileName: "item1.bin",
		SaveDir:  tmpDir,
		QueueID:  "main",
		StartNow: true,
	})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c1, _ := mgr.GetDownload(dl1.ID)
		if c1 != nil && c1.Status == models.StatusError {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	dl2, _ := mgr.AddDownload(models.AddDownloadRequest{
		URL:      srv.URL + "/item2.bin",
		FileName: "item2.bin",
		SaveDir:  tmpDir,
		QueueID:  "main",
		StartNow: false,
	})

	// Agora o servidor volta a funcionar para item1.bin.
	failItem1.Store(false)

	q := models.Queue{
		ID:               "main",
		Name:             "Fila Principal",
		IsMain:           true,
		MaxConcurrent:    1,
		RetryIntervalSec: 0,
		MaxRetries:       5,
		Active:           true,
	}
	_ = st.SaveQueue(q)

	// No primeiro tick com MaxConcurrent=1, dl2 (StatusQueued) deve ter prioridade sobre dl1 (StatusError).
	sched.processQueue(q)

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c2, _ := st.GetDownload(dl2.ID)
		if c2 != nil && c2.Status == models.StatusCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	c2, _ := st.GetDownload(dl2.ID)
	if c2 == nil || c2.Status != models.StatusCompleted {
		t.Fatalf("esperado que dl2 (StatusQueued) fosse processado antes do item em erro, status=%v", c2)
	}

	// No próximo tick, como dl1 é o único restante na fila, ele deve ser retentado e concluir.
	sched.processQueue(q)
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c1, _ := st.GetDownload(dl1.ID)
		if c1 != nil && c1.Status == models.StatusCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	c1, _ := st.GetDownload(dl1.ID)
	if c1 == nil || c1.Status != models.StatusCompleted {
		t.Fatalf("esperado que dl1 (StatusError) fosse retentado após os demais itens da fila, status=%v", c1)
	}
}

