package engine

import (
	"context"
	"sync"
	"time"
)

// SpeedLimiter implementa um limitador de banda Token Bucket thread-safe.
// Pode ser usado tanto globalmente quanto individualmente por download.
type SpeedLimiter struct {
	mu       sync.Mutex
	rateBps  int64
	tokens   float64
	lastTime time.Time
}

// NewSpeedLimiter cria um novo limitador de velocidade (0 = sem limite).
func NewSpeedLimiter(rateBps int64) *SpeedLimiter {
	return &SpeedLimiter{
		rateBps:  rateBps,
		tokens:   float64(rateBps),
		lastTime: time.Now(),
	}
}

// SetRate altera dinamicamente o limite em bytes por segundo (0 desativa o limite).
func (l *SpeedLimiter) SetRate(rateBps int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rateBps = rateBps
	if float64(rateBps) < l.tokens {
		l.tokens = float64(rateBps)
	}
}

// Wait bloqueia pelo tempo necessário para respeitar a taxa de bytes por segundo configurada.
func (l *SpeedLimiter) Wait(ctx context.Context, n int) error {
	if l == nil || n <= 0 {
		return nil
	}

	l.mu.Lock()
	rate := l.rateBps
	if rate <= 0 {
		l.mu.Unlock()
		return nil
	}

	now := time.Now()
	elapsed := now.Sub(l.lastTime).Seconds()
	l.lastTime = now

	// Recarrega tokens proporcionalmente ao tempo decorrido (burst máximo de 1s).
	l.tokens += elapsed * float64(rate)
	if l.tokens > float64(rate) {
		l.tokens = float64(rate)
	}

	l.tokens -= float64(n)
	if l.tokens >= 0 {
		l.mu.Unlock()
		return nil
	}

	// Calcula espera necessária para repor os tokens negativos.
	waitSec := (-l.tokens) / float64(rate)
	waitDur := time.Duration(waitSec * float64(time.Second))
	l.mu.Unlock()

	if waitDur <= 0 {
		return nil
	}

	timer := time.NewTimer(waitDur)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
