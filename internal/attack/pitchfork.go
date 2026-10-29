package attack

import (
	"context"
	"fmt"

	"intruder/internal/engine"
	"intruder/internal/payload"
	"intruder/internal/template"
)

// PitchforkConfig configures a Pitchfork run: one payload generator per
// marker, all walked in lockstep (iteration i of every generator feeds
// marker i on the same request). The run stops when the shortest generator
// runs dry, matching Burp's own Pitchfork behavior — any extra values in
// longer generators are never used.
type PitchforkConfig struct {
	Template   *template.Template
	Target     template.TargetConfig
	Generators []payload.Generator // must have exactly len(Template.Markers) entries, in marker order
}

// TotalJobs returns the number of requests a Pitchfork run with this config
// will produce: the length of the shortest generator, since lockstep
// iteration stops the moment any one generator is exhausted.
func (c PitchforkConfig) TotalJobs() int {
	return shortestLen(c.Generators)
}

func shortestLen(gens []payload.Generator) int {
	if len(gens) == 0 {
		return 0
	}
	min := gens[0].Len()
	for _, g := range gens[1:] {
		if l := g.Len(); l < min {
			min = l
		}
	}
	return min
}

// Jobs streams the full Pitchfork job sequence onto a channel. It closes
// the channel when the shortest generator is exhausted or when ctx is
// cancelled, and never blocks past ctx cancellation trying to send into a
// full channel.
func PitchforkJobs(ctx context.Context, cfg PitchforkConfig, buffer int) <-chan engine.Job {
	out := make(chan engine.Job, buffer)

	go func() {
		defer close(out)

		markers := cfg.Template.Markers
		if len(cfg.Generators) != len(markers) {
			// mirrors Inject's own validation style: a mismatch here is a
			// setup error the caller should have caught before starting a
			// run, so bail out rather than guess what to do with it
			return
		}
		for _, g := range cfg.Generators {
			g.Reset()
		}

		for {
			values := make([]string, len(markers))
			metaPayloads := make([]string, len(markers))
			allOk := true
			for i, g := range cfg.Generators {
				val, ok := g.Next()
				if !ok {
					allOk = false
					break
				}
				values[i] = val
				metaPayloads[i] = val
			}
			if !allOk {
				return
			}

			req, err := cfg.Template.Build(values, cfg.Target)
			if err != nil {
				return
			}

			meta := map[string]string{}
			for i, p := range metaPayloads {
				meta[fmt.Sprintf("payload%d", i)] = p
			}

			job := engine.Job{Request: req, Meta: meta}

			select {
			case out <- job:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}
