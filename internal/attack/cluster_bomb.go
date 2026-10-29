package attack

import (
	"context"
	"fmt"

	"intruder/internal/engine"
	"intruder/internal/payload"
	"intruder/internal/template"
)

// ClusterBombConfig configures a Cluster bomb run: one payload generator
// per marker, walked as a full combinatorial product across all markers
// (every combination of every generator's values), matching Burp's
// behavior. The last generator in the slice varies fastest, same as the
// least-significant digit of an odometer.
type ClusterBombConfig struct {
	Template   *template.Template
	Target     template.TargetConfig
	Generators []payload.Generator // must have exactly len(Template.Markers) entries, in marker order
}

// TotalJobs returns the number of requests a Cluster bomb run with this
// config will produce: the product of every generator's length.
func (c ClusterBombConfig) TotalJobs() int {
	if len(c.Generators) == 0 {
		return 0
	}
	total := 1
	for _, g := range c.Generators {
		total *= g.Len()
	}
	return total
}

// Jobs streams the full Cluster bomb job sequence onto a channel. It
// closes the channel once every combination has been produced or when ctx
// is cancelled, and never blocks past ctx cancellation trying to send into
// a full channel.
func ClusterBombJobs(ctx context.Context, cfg ClusterBombConfig, buffer int) <-chan engine.Job {
	out := make(chan engine.Job, buffer)

	go func() {
		defer close(out)

		markers := cfg.Template.Markers
		gens := cfg.Generators
		n := len(gens)
		if n != len(markers) {
			// same setup-error reasoning as Pitchfork
			return
		}
		if n == 0 {
			return
		}
		for _, g := range gens {
			g.Reset()
		}

		// current holds the value each generator is currently positioned
		// on; it's refilled by advance() below using a classic
		// odometer-style nested-increment pattern, generator[n-1] (last
		// marker) rolling over fastest.
		current := make([]string, n)
		if !primeFirst(gens, current) {
			return // at least one generator is empty: no combinations exist
		}

		for {
			values := make([]string, n)
			copy(values, current)

			req, err := cfg.Template.Build(values, cfg.Target)
			if err != nil {
				return
			}

			meta := map[string]string{}
			for i, p := range values {
				meta[fmt.Sprintf("payload%d", i)] = p
			}

			select {
			case out <- engine.Job{Request: req, Meta: meta}:
			case <-ctx.Done():
				return
			}

			if !advance(gens, current) {
				return // every combination has been produced
			}
		}
	}()

	return out
}

// primeFirst reads the first value from every generator into current. It
// returns false if any generator is already exhausted (Len() == 0), in
// which case the combinatorial product is empty and no jobs should be sent.
func primeFirst(gens []payload.Generator, current []string) bool {
	for i, g := range gens {
		v, ok := g.Next()
		if !ok {
			return false
		}
		current[i] = v
	}
	return true
}

// advance moves current to the next combination in the product, treating
// gens as odometer digits with the last index as least-significant (it
// varies on every call; earlier indices only roll over once everything to
// their right has exhausted and reset). It returns false once every
// combination has been produced.
func advance(gens []payload.Generator, current []string) bool {
	for i := len(gens) - 1; i >= 0; i-- {
		v, ok := gens[i].Next()
		if ok {
			current[i] = v
			return true
		}
		// this digit rolled over: reset it back to its first value and
		// carry into the next-more-significant generator
		gens[i].Reset()
		v, ok = gens[i].Next()
		if !ok {
			// a generator that reports Len() > 0 but yields nothing after
			// Reset() would be a broken Generator implementation; treat it
			// the same as "no more combinations" rather than panicking
			return false
		}
		current[i] = v
	}
	// every generator rolled over, including index 0: product exhausted
	return false
}
