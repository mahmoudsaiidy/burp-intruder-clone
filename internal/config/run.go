package config

import (
	"context"
	"fmt"

	"intruder/internal/attack"
	"intruder/internal/chain"
	"intruder/internal/engine"
	"intruder/internal/match"
	"intruder/internal/template"
)

// jobBuffer is how many job-channel slots Run gives each attack mode ahead
// of the worker pool, same multiplier cmd/cli and the web demo already
// used by hand.
const jobBufferPerWorker = 2

// Run builds c and runs the attack it describes end to end: resolving any
// chain stages first, selecting the job sequence for Mode, firing it
// through a freshly built engine.Pool, and tagging every result against
// Matcher. This is the one place cmd/cli and the web UI's "run a custom
// attack" endpoint both call into, replacing what each used to hand-wire
// separately for the hardcoded demo.
//
// total is the number of requests this run is expected to make. The
// returned channel closes when the attack completes or ctx is cancelled.
func (c AttackConfig) Run(ctx context.Context) (results <-chan match.Flagged, total int, err error) {
	built, err := c.Build()
	if err != nil {
		return nil, 0, err
	}
	return built.Run(ctx)
}

// Run starts the attack b describes. See AttackConfig.Run.
func (b *Built) Run(ctx context.Context) (<-chan match.Flagged, int, error) {
	client := engine.NewClient(engine.DefaultClientConfig(b.Workers))

	ctx, cancel := context.WithTimeout(ctx, b.Timeout)
	// cancel is deliberately not deferred here: it must stay live for the
	// whole attack, which keeps running on goroutines after Run returns.
	// match.Flag's output channel closing (ctx cancelled or results
	// drained) is the right moment to release it, so it's wrapped around
	// the result stream below instead of deferred in this function.

	tmpl := b.Template
	if len(b.Stages) > 0 {
		resolvedRaw, _, err := chain.Resolve(ctx, client, b.Stages, tmpl.Raw)
		if err != nil {
			cancel()
			return nil, 0, fmt.Errorf("config: resolving chain: %w", err)
		}
		// Re-parse: chain.Resolve never touches the attack's own
		// §markers§, but substituting placeholders elsewhere in the text
		// shifts their byte offsets, so Template needs rebuilding from the
		// resolved text rather than having its Markers patched in place.
		resolved, err := template.Parse(resolvedRaw)
		if err != nil {
			cancel()
			return nil, 0, fmt.Errorf("config: re-parsing chain-resolved template: %w", err)
		}
		tmpl = resolved
	}

	buffer := b.Workers * jobBufferPerWorker

	var jobs <-chan engine.Job
	var total int
	switch b.Mode {
	case ModeSniper:
		cfg := attack.SniperConfig{Template: tmpl, Target: b.Target, Generator: b.Generators[0]}
		total = cfg.TotalJobs()
		jobs = attack.Jobs(ctx, cfg, buffer)
	case ModeBatteringRam:
		cfg := attack.BatteringRamConfig{Template: tmpl, Target: b.Target, Generator: b.Generators[0]}
		total = cfg.TotalJobs()
		jobs = attack.BatteringRamJobs(ctx, cfg, buffer)
	case ModePitchfork:
		cfg := attack.PitchforkConfig{Template: tmpl, Target: b.Target, Generators: b.Generators}
		total = cfg.TotalJobs()
		jobs = attack.PitchforkJobs(ctx, cfg, buffer)
	case ModeClusterBomb:
		cfg := attack.ClusterBombConfig{Template: tmpl, Target: b.Target, Generators: b.Generators}
		total = cfg.TotalJobs()
		jobs = attack.ClusterBombJobs(ctx, cfg, buffer)
	default:
		// Unreachable: Build already rejects any other Mode.
		cancel()
		return nil, 0, fmt.Errorf("config: unknown mode %q", b.Mode)
	}

	pool := engine.NewPool(client, engine.PoolConfig{
		Workers:       b.Workers,
		RatePerSecond: b.RatePerSecond,
		JitterMax:     b.JitterMax,
	})
	results := pool.Run(ctx, jobs)
	flagged := match.Flag(ctx, results, b.Matcher)

	return releaseOnClose(flagged, cancel), total, nil
}

// releaseOnClose wraps in so cancel runs exactly once, right after in
// closes, instead of leaking the timeout context's timer until its own
// deadline regardless of whether the attack finished early.
func releaseOnClose(in <-chan match.Flagged, cancel context.CancelFunc) <-chan match.Flagged {
	out := make(chan match.Flagged)
	go func() {
		defer close(out)
		defer cancel()
		for f := range in {
			out <- f
		}
	}()
	return out
}
