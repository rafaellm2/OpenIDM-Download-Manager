package scheduler

import (
	"context"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"openidm/internal/engine"
	"openidm/internal/models"
	"openidm/internal/store"
	"openidm/internal/sysproc"
)

// Scheduler gerencia o agendamento de filas, sincronização periódica, quotas de tráfego e ações pós-conclusão.
type Scheduler struct {
	mu      sync.Mutex
	store   *store.Store
	manager *engine.Manager
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewScheduler cria um novo agendador vinculado ao Store e ao Manager de downloads.
func NewScheduler(st *store.Store, mgr *engine.Manager) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		store:   st,
		manager: mgr,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start inicia o loop de verificação de horários, filas ativas e quotas a cada 5 segundos.
func (s *Scheduler) Start() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.tick()
			}
		}
	}()
}

// Stop encerra o loop do agendador.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// StartQueue ativa manualmente uma fila e dispara imediatamente seus downloads até o limite MaxConcurrent.
func (s *Scheduler) StartQueue(queueID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	queues, err := s.store.ListQueues()
	if err != nil {
		return err
	}
	for _, q := range queues {
		if q.ID == queueID {
			q.Active = true
			if err := s.store.SaveQueue(q); err != nil {
				return err
			}
			// Se for uma fila de sincronização, verifica primeiro se os arquivos concluídos mudaram no servidor.
			if q.IsSync {
				s.checkSyncQueueUpdates(q)
			}
			s.processQueue(q)
			return nil
		}
	}
	return nil
}

// StopQueue desativa uma fila e pausa todos os downloads que pertencem a ela.
func (s *Scheduler) StopQueue(queueID string) error {
	queues, err := s.store.ListQueues()
	if err != nil {
		return err
	}
	for _, q := range queues {
		if q.ID == queueID {
			q.Active = false
			if err := s.store.SaveQueue(q); err != nil {
				return err
			}
			break
		}
	}

	downloads := s.manager.ListDownloads()
	for _, d := range downloads {
		if d.QueueID == queueID && (d.Status == models.StatusDownloading || d.Status == models.StatusProbing) {
			_ = s.manager.PauseDownload(d.ID)
		}
	}
	return nil
}

// tick avalia gatilhos de horário (StartTime/StopTime), quotas de tráfego e avança filas ativas.
func (s *Scheduler) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	queues, err := s.store.ListQueues()
	if err != nil {
		return
	}

	nowHHMM := time.Now().Format("15:04")

	for _, q := range queues {
		// Verifica horário de início agendado.
		if !q.Active && q.StartTime != "" && q.StartTime == nowHHMM {
			q.Active = true
			_ = s.store.SaveQueue(q)
			if q.IsSync {
				s.checkSyncQueueUpdates(q)
			}
		}

		// Verifica horário de parada agendado.
		if q.Active && q.StopTime != "" && q.StopTime == nowHHMM {
			_ = s.StopQueue(q.ID)
			continue
		}

		// Verifica limite/quota de tráfego da fila (TrafficQuotaMB).
		if q.Active && q.TrafficQuotaMB > 0 {
			quotaBytes := q.TrafficQuotaMB * 1024 * 1024
			if q.UsedTrafficBytes >= quotaBytes {
				_ = s.StopQueue(q.ID)
				continue
			}
		}

		if q.Active {
			s.processQueue(q)
		}
	}
}

// processQueue garante que até MaxConcurrent itens da fila estejam baixando e executa pós-ações ao concluir.
func (s *Scheduler) processQueue(q models.Queue) {
	maxConc := q.MaxConcurrent
	if maxConc <= 0 {
		maxConc = 4
	}

	activeCount := s.manager.ActiveCountForQueue(q.ID)
	if activeCount >= maxConc {
		return
	}

	allDownloads := s.manager.ListDownloads()

	var normalQueue []models.Download
	var errorQueue []models.Download
	incompleteCount := activeCount

	for _, d := range allDownloads {
		if d.QueueID != q.ID {
			continue
		}
		switch d.Status {
		case models.StatusQueued, models.StatusPaused:
			normalQueue = append(normalQueue, d)
			incompleteCount++
		case models.StatusError:
			errorQueue = append(errorQueue, d)
			incompleteCount++
		}
	}

	// Ordena itens normais por CreatedAt (FIFO) e itens que falharam por UpdatedAt (vão para o fim da fila).
	sort.Slice(normalQueue, func(i, j int) bool {
		return normalQueue[i].CreatedAt.Before(normalQueue[j].CreatedAt)
	})
	sort.Slice(errorQueue, func(i, j int) bool {
		return errorQueue[i].UpdatedAt.Before(errorQueue[j].UpdatedAt)
	})

	candidates := make([]models.Download, 0, len(normalQueue)+len(errorQueue))
	candidates = append(candidates, normalQueue...)

	hasPendingOrRunning := activeCount > 0 || len(normalQueue) > 0
	lonelyInQueue := incompleteCount == 1

	retryWait := time.Duration(q.RetryIntervalSec) * time.Second
	if retryWait < 0 {
		retryWait = 30 * time.Second
	}

	for _, d := range errorQueue {
		// Se estiver sozinho na fila ou dentro do limite de tentativas da fila, agenda retry após RetryIntervalSec.
		if q.MaxRetries == 0 || lonelyInQueue || d.RetryCount < q.MaxRetries {
			hasPendingOrRunning = true
			if time.Since(d.UpdatedAt) >= retryWait {
				candidates = append(candidates, d)
			}
		}
	}

	slots := maxConc - activeCount
	for i := 0; i < len(candidates) && i < slots; i++ {
		_ = s.manager.StartDownload(candidates[i].ID)
		hasPendingOrRunning = true
	}

	// Se a fila estava ativa e não há mais nenhum download pendente ou rodando, conclui a fila e dispara ações finais.
	if !hasPendingOrRunning {
		q.Active = false
		_ = s.store.SaveQueue(q)
		s.executeQueueFinishActions(q)
	}
}

// checkSyncQueueUpdates verifica se os arquivos na Fila de Sincronização sofreram alteração no servidor.
func (s *Scheduler) checkSyncQueueUpdates(q models.Queue) {
	settings, _ := s.store.GetSettings()
	client := engine.BuildHTTPClient(settings, 2)
	downloads := s.manager.ListDownloads()

	for _, d := range downloads {
		if d.QueueID != q.ID || d.Status != models.StatusCompleted {
			continue
		}
		login := s.manager.FindLoginForURL(d.URL)
		probe, err := engine.ProbeURL(s.ctx, client, d.URL, login)
		if err != nil {
			continue
		}
		changed := false
		if probe.ETag != "" && d.ETag != "" && probe.ETag != d.ETag {
			changed = true
		} else if probe.LastModified != "" && d.LastModified != "" && probe.LastModified != d.LastModified {
			changed = true
		} else if probe.TotalSize > 0 && d.TotalSize > 0 && probe.TotalSize != d.TotalSize {
			changed = true
		}
		if changed {
			// Reinicia o download pois o arquivo remoto foi atualizado no servidor.
			_ = s.manager.StartDownload(d.ID)
		}
	}
}

// executeQueueFinishActions executa comando customizado ou desligamento agendado do PC ao término da fila.
func (s *Scheduler) executeQueueFinishActions(q models.Queue) {
	if cmdStr := strings.TrimSpace(q.RunCommandOnFinish); cmdStr != "" {
		parts := engine.ParseCommandArgs(cmdStr, "")
		if len(parts) > 0 {
			cmd := sysproc.HiddenCommand(parts[0], parts[1:]...)
			_ = cmd.Start()
		}
	}

	if q.ShutdownOnFinish {
		_ = TriggerSystemShutdown(60)
	}
}

// TriggerSystemShutdown agenda o desligamento do sistema operacional (com 60s de segurança para cancelamento).
func TriggerSystemShutdown(delaySeconds int) error {
	if delaySeconds <= 0 {
		delaySeconds = 60
	}
	if runtime.GOOS == "windows" {
		return sysproc.HiddenCommand("shutdown", "/s", "/t", strconv.Itoa(delaySeconds), "/c", "OpenIDM: Fila de downloads concluída. Desligando o computador.").Start()
	}
	return nil
}

// CancelSystemShutdown cancela um desligamento agendado no Windows.
func CancelSystemShutdown() error {
	if runtime.GOOS == "windows" {
		return sysproc.HiddenCommand("shutdown", "/a").Start()
	}
	return nil
}
