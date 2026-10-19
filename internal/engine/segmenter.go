package engine

import (
	"sort"
	"sync"

	"openidm/internal/models"
)

// SegmentTable gerencia o mapa de segmentos dinâmicos de um download com segurança concorrente.
// Implementa o algoritmo de segmentação dinâmica com divisão adaptativa do maior segmento pendente.
type SegmentTable struct {
	mu         sync.Mutex
	downloadID string
	totalSize  int64
	minSplit   int64
	segments   []models.Segment
	degraded   bool
	degradeMsg string
	onDegrade  func()
}

// SetOnDegrade registra um callback acionado quando o download degrada para 1 conexão.
func (st *SegmentTable) SetOnDegrade(fn func()) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.onDegrade = fn
}

// NewSegmentTable cria ou restaura o mapa de segmentos para um arquivo.
func NewSegmentTable(downloadID string, totalSize int64, minSplit int64, existing []models.Segment) *SegmentTable {
	if minSplit <= 0 {
		minSplit = 1024 * 1024 // 1 MB padrão
	}
	st := &SegmentTable{
		downloadID: downloadID,
		totalSize:  totalSize,
		minSplit:   minSplit,
	}

	if len(existing) > 0 {
		st.segments = make([]models.Segment, len(existing))
		copy(st.segments, existing)
		// Ao retomar (resume), qualquer segmento que não terminou volta para "pending" a partir do byte Current salvo no checkpoint.
		for i := range st.segments {
			st.segments[i].Index = i
			st.segments[i].SpeedBps = 0
			if st.segments[i].End > 0 && st.segments[i].Current >= st.segments[i].End {
				st.segments[i].Status = models.SegmentCompleted
			} else if st.segments[i].Status != models.SegmentCompleted {
				st.segments[i].Status = models.SegmentPending
				st.segments[i].WorkerID = 0
			}
		}
		return st
	}

	// Inicializa com 1 segmento cobrindo o arquivo inteiro [0, totalSize).
	// Os demais workers dividirão dinamicamente este segmento ao meio ao entrarem em operação.
	st.segments = []models.Segment{
		{
			DownloadID: downloadID,
			Index:      0,
			Start:      0,
			Current:    0,
			End:        totalSize,
			Status:     models.SegmentPending,
		},
	}
	return st
}

// AcquireNextSegment atribui um segmento pendente ou divide ao meio o MAIOR segmento em andamento.
// Quando reused=true, sinaliza que o worker reutilizou a conexão TCP/TLS persistente sem novo handshake.
func (st *SegmentTable) AcquireNextSegment(workerID int, reused bool) (*models.Segment, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()

	// Se o download degradou para 1 conexão, apenas o worker #1 pode operar.
	if st.degraded {
		if workerID > 1 {
			return nil, false
		}
		for _, seg := range st.segments {
			if seg.Status == models.SegmentDownloading && seg.WorkerID != workerID {
				return nil, false
			}
		}
	}

	// 1. Procura o maior segmento com status Pending.
	bestPendingIdx := -1
	var bestPendingRem int64 = -1
	for i := range st.segments {
		seg := &st.segments[i]
		if seg.Status == models.SegmentPending {
			rem := seg.RemainingBytes()
			// Caso stream sem tamanho conhecido (End <= 0)
			if seg.End <= 0 && seg.Current == 0 {
				bestPendingIdx = i
				break
			}
			if rem > bestPendingRem && rem > 0 {
				bestPendingRem = rem
				bestPendingIdx = i
			}
		}
	}

	if bestPendingIdx >= 0 {
		seg := &st.segments[bestPendingIdx]
		seg.Status = models.SegmentDownloading
		seg.WorkerID = workerID
		seg.ReusedConn = reused
		seg.LastError = ""
		cp := *seg
		return &cp, true
	}

	// Se não suporta divisão (tamanho desconhecido ou modo degradado de 1 conexão), não divide.
	if st.totalSize <= 0 || st.degraded {
		return nil, false
	}

	// 2. Nenhum segmento pendente: procura o MAIOR segmento em download para dividir ao meio (Dynamic Split).
	largestIdx := -1
	var largestRem int64 = 0
	for i := range st.segments {
		seg := &st.segments[i]
		if seg.Status == models.SegmentDownloading {
			rem := seg.RemainingBytes()
			if rem > largestRem {
				largestRem = rem
				largestIdx = i
			}
		}
	}

	// Só divide se o restante do maior segmento for maior que minSplit (garantindo fatias significativas).
	if largestIdx < 0 || largestRem < st.minSplit {
		return nil, false
	}

	target := &st.segments[largestIdx]
	mid := target.Current + (target.End-target.Current)/2
	// Alinha em múltiplo de 4 KB quando possível para escrita limpa em disco, sem violar limites.
	if aligned := (mid / 4096) * 4096; aligned > target.Current && aligned < target.End {
		mid = aligned
	}
	if mid <= target.Current || mid >= target.End {
		return nil, false
	}

	oldEnd := target.End
	target.End = mid // Reduz o fim do segmento atual; o worker ativo nele irá parar ao atingir mid.

	newSeg := models.Segment{
		DownloadID: st.downloadID,
		Start:      mid,
		Current:    mid,
		End:        oldEnd,
		Status:     models.SegmentDownloading,
		WorkerID:   workerID,
		ReusedConn: reused,
	}

	// Insere o novo segmento logo após o segmento dividido e reindexa.
	insertPos := largestIdx + 1
	st.segments = append(st.segments, models.Segment{})
	copy(st.segments[insertPos+1:], st.segments[insertPos:])
	st.segments[insertPos] = newSeg
	st.reindexLocked()

	cp := st.segments[insertPos]
	return &cp, true
}

// GetSegmentBound retorna o limite atual (Current, End) de um segmento localizado pelo byte inicial Start.
func (st *SegmentTable) GetSegmentBound(startByte int64) (current int64, end int64, ok bool) {
	st.mu.Lock()
	defer st.mu.Unlock()

	idx := st.findByStartLocked(startByte)
	if idx < 0 {
		return 0, 0, false
	}
	return st.segments[idx].Current, st.segments[idx].End, true
}

// AdvanceSegment avança o ponteiro Current de um segmento identificado por Start.
// Retorna quantos bytes do lote pertencem efetivamente ao segmento (caso End tenha sido reduzido por um split dinâmico).
func (st *SegmentTable) AdvanceSegment(startByte int64, delta int64, speedBps int64) (acceptedBytes int64, reachedEnd bool, found bool) {
	st.mu.Lock()
	defer st.mu.Unlock()

	idx := st.findByStartLocked(startByte)
	if idx < 0 {
		return 0, true, false
	}
	seg := &st.segments[idx]
	if speedBps > 0 {
		seg.SpeedBps = speedBps
	}

	// Se o tamanho total é desconhecido (stream de 1 conexão com End == 0).
	if seg.End <= 0 {
		seg.Current += delta
		return delta, false, true
	}

	remaining := seg.End - seg.Current
	if remaining <= 0 {
		seg.Current = seg.End
		seg.Status = models.SegmentCompleted
		seg.SpeedBps = 0
		return 0, true, true
	}

	if delta >= remaining {
		acceptedBytes = remaining
		seg.Current = seg.End
		seg.Status = models.SegmentCompleted
		seg.SpeedBps = 0
		return acceptedBytes, true, true
	}

	seg.Current += delta
	return delta, false, true
}

// CompleteSegment marca explicitamente um segmento como concluído.
func (st *SegmentTable) CompleteSegment(startByte int64) {
	st.mu.Lock()
	defer st.mu.Unlock()

	idx := st.findByStartLocked(startByte)
	if idx < 0 {
		return
	}
	seg := &st.segments[idx]
	if seg.End <= 0 {
		seg.End = seg.Current
	} else {
		seg.Current = seg.End
	}
	seg.Status = models.SegmentCompleted
	seg.SpeedBps = 0
}

// ReleaseSegmentOnError devolve um segmento interrompido para o estado Pending (mantendo Current no último byte gravado).
func (st *SegmentTable) ReleaseSegmentOnError(startByte int64, errMsg string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	idx := st.findByStartLocked(startByte)
	if idx < 0 {
		return
	}
	seg := &st.segments[idx]
	seg.SpeedBps = 0
	if seg.End > 0 && seg.Current >= seg.End {
		seg.Status = models.SegmentCompleted
		return
	}
	seg.Status = models.SegmentPending
	seg.WorkerID = 0
	seg.LastError = errMsg
}

// DegradeToSingleConnection ativa o modo de segurança de 1 conexão (quando corrupção ou falha de Range é detectada),
// colapsando o mapa de segmentos para um fluxo único [0, totalSize) e interrompendo conexões secundárias.
func (st *SegmentTable) DegradeToSingleConnection(reason string) {
	st.mu.Lock()
	if st.degraded {
		st.mu.Unlock()
		return
	}
	st.degraded = true
	st.degradeMsg = reason
	st.segments = []models.Segment{
		{
			DownloadID: st.downloadID,
			Index:      0,
			Start:      0,
			Current:    0,
			End:        st.totalSize,
			Status:     models.SegmentPending,
			WorkerID:   0,
		},
	}
	cb := st.onDegrade
	st.mu.Unlock()

	if cb != nil {
		cb()
	}
}

// ResetSingleSegment reinicia de forma limpa o segmento único [0, totalSize) após o encerramento de todas as conexões paralelas.
func (st *SegmentTable) ResetSingleSegment() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.segments = []models.Segment{
		{
			DownloadID: st.downloadID,
			Index:      0,
			Start:      0,
			Current:    0,
			End:        st.totalSize,
			Status:     models.SegmentDownloading,
			WorkerID:   1,
		},
	}
}

// UpdateSingleStreamTotalSize sincroniza o tamanho total esperado quando o download opera em 1 conexão (!acceptRanges)
// e o servidor informa (ou omite) Content-Length na resposta GET definitiva.
func (st *SegmentTable) UpdateSingleStreamTotalSize(totalSize int64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.segments) != 1 || st.segments[0].Start != 0 {
		return
	}
	if totalSize > 0 {
		st.totalSize = totalSize
		st.segments[0].End = totalSize
	} else {
		st.totalSize = 0
		st.segments[0].End = 0
	}
}

// IsDegraded informa se o mapa de segmentos entrou em modo degradado de 1 conexão.
func (st *SegmentTable) IsDegraded() (bool, string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.degraded, st.degradeMsg
}

// ValidateIntegrity verifica se os segmentos cobrem [0, totalSize) sem buracos ou sobreposições.
func (st *SegmentTable) ValidateIntegrity() bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.totalSize <= 0 {
		return len(st.segments) == 1 && st.segments[0].Status == models.SegmentCompleted
	}
	if len(st.segments) == 0 {
		return false
	}

	sorted := make([]models.Segment, len(st.segments))
	copy(sorted, st.segments)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start < sorted[j].Start
	})

	var expected int64 = 0
	for _, seg := range sorted {
		if seg.Start != expected || seg.Current < seg.End || seg.End <= seg.Start {
			return false
		}
		expected = seg.End
	}
	return expected == st.totalSize
}

// Snapshot retorna uma cópia segura dos segmentos e métricas agregadas para UI e SQLite.
func (st *SegmentTable) Snapshot() (segments []models.Segment, downloaded int64, totalSpeed int64, activeWorkers int, allDone bool) {
	st.mu.Lock()
	defer st.mu.Unlock()

	segments = make([]models.Segment, len(st.segments))
	copy(segments, st.segments)

	allDone = len(segments) > 0
	for _, seg := range segments {
		downloaded += seg.DownloadedBytes()
		if seg.Status == models.SegmentDownloading {
			totalSpeed += seg.SpeedBps
			activeWorkers++
		}
		if seg.Status != models.SegmentCompleted {
			allDone = false
		}
	}
	if st.totalSize > 0 && downloaded > st.totalSize {
		downloaded = st.totalSize
	}
	return segments, downloaded, totalSpeed, activeWorkers, allDone
}

func (st *SegmentTable) findByStartLocked(startByte int64) int {
	for i := range st.segments {
		if st.segments[i].Start == startByte {
			return i
		}
	}
	return -1
}

func (st *SegmentTable) reindexLocked() {
	sort.Slice(st.segments, func(i, j int) bool {
		return st.segments[i].Start < st.segments[j].Start
	})
	for i := range st.segments {
		st.segments[i].Index = i
	}
}
