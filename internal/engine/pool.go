package engine

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

// PoolConfig controls concurrency and throttling for one attack run.
type PoolConfig struct {
	Workers       int
	RatePerSecond float64       // 0 = unlimited; global cap across the whole pool, not per-worker
	JitterMax     time.Duration // extra random delay added per request, on top of the rate cap
	MaxBodyBytes  int64         // cap how much of each response body is read/kept; 0 = 5MB default
}

// Pool fires Jobs pulled from an input channel using a bounded worker
// goroutine set and streams Results out as they complete.
type Pool struct {
	client  *http.Client
	cfg     PoolConfig
	limiter *rateLimiter
}

func NewPool(client *http.Client, cfg PoolConfig) *Pool {
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 5 << 20 // 5MB
	}
	return &Pool{
		client:  client,
		cfg:     cfg,
		limiter: newRateLimiter(cfg.RatePerSecond),
	}
}

// Run drains jobs, fires each through the worker pool, and sends every
// completed Result on the returned channel. The channel closes once every
// worker has exited — either jobs was exhausted and closed by the producer,
// or ctx was cancelled. Cancellation stops workers from picking up *new*
// jobs; it does not drop a Result for a request already in flight, and it
// does not leave the result channel open after the last worker exits.
func (p *Pool) Run(ctx context.Context, jobs <-chan Job) <-chan Result {
	workers := p.cfg.Workers
	if workers < 1 {
		workers = 1
	}

	results := make(chan Result, workers*4)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			p.worker(ctx, jobs, results)
		}()
	}

	go func() {
		wg.Wait()
		close(results)
		if p.limiter != nil {
			p.limiter.stop()
		}
	}()

	return results
}

func (p *Pool) worker(ctx context.Context, jobs <-chan Job, results chan<- Result) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			if err := p.limiter.wait(ctx); err != nil {
				return
			}
			if p.cfg.JitterMax > 0 {
				select {
				case <-time.After(jitter(p.cfg.JitterMax)):
				case <-ctx.Done():
					return
				}
			}

			// a plain blocking send: the result channel only closes after
			// every worker has returned, so this can't deadlock, and a
			// request that already happened should always be delivered
			// even if ctx was cancelled the instant it completed
			results <- p.fire(ctx, job)
		}
	}
}

func (p *Pool) fire(ctx context.Context, job Job) Result {
	start := time.Now()

	req := job.Request.WithContext(ctx)
	resp, err := p.client.Do(req)
	if err != nil {
		return Result{Job: job, Duration: time.Since(start), Err: err}
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, p.cfg.MaxBodyBytes))
	duration := time.Since(start)
	if readErr != nil {
		return Result{Job: job, Status: resp.StatusCode, Duration: duration, Headers: resp.Header, Err: readErr}
	}

	return Result{
		Job:      job,
		Status:   resp.StatusCode,
		Length:   int64(len(body)),
		Duration: duration,
		Headers:  resp.Header,
		Body:     body,
	}
}
