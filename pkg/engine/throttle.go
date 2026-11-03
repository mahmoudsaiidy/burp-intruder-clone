package engine

import (
	"context"
	"math/rand"
	"time"
)

// rateLimiter is a minimal token-bucket limiter: stdlib-only so the engine
// has no external dependencies. One token is added every 1/rps, capacity 1
// — that's a steady cap on requests/sec per pool, not a bursty allowance,
// which matches "requests per second cap" rather than "average rate".
type rateLimiter struct {
	interval time.Duration
	ticker   *time.Ticker
}

func newRateLimiter(rps float64) *rateLimiter {
	if rps <= 0 {
		return nil
	}
	interval := time.Duration(float64(time.Second) / rps)
	if interval <= 0 {
		interval = time.Nanosecond
	}
	return &rateLimiter{interval: interval, ticker: time.NewTicker(interval)}
}

// wait blocks until the next token is available or ctx is done.
func (r *rateLimiter) wait(ctx context.Context) error {
	if r == nil {
		return nil
	}
	select {
	case <-r.ticker.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *rateLimiter) stop() {
	if r != nil {
		r.ticker.Stop()
	}
}

// jitter returns a random duration in [0, max). Used to add noise on top of
// the RPS cap so traffic doesn't look like a metronome to the target.
func jitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(max)))
}
