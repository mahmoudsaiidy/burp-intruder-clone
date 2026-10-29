// Package attack turns a template + payload generator(s) into the job
// sequence a particular attack mode defines. Sniper is implemented first;
// Battering ram, Pitchfork and Cluster bomb are additional files in this
// package following the same Jobs(ctx, cfg, buffer) <-chan engine.Job shape.
package attack

import (
	"context"
	"strconv"

	"intruder/internal/engine"
	"intruder/internal/payload"
	"intruder/internal/template"
)

// SniperConfig configures a Sniper-mode run: one payload generator cycled
// through each marker position in turn, one marker "live" at a time, with
// every other marker held at its original template value.
type SniperConfig struct {
	Template  *template.Template
	Target    template.TargetConfig
	Generator payload.Generator
}

// TotalJobs returns the number of requests a Sniper run with this config
// will produce: markers × payloads, since the same generator is replayed
// once per marker position.
func (c SniperConfig) TotalJobs() int {
	return len(c.Template.Markers) * c.Generator.Len()
}

// Jobs streams the full Sniper job sequence onto a channel. It closes the
// channel when the sequence is exhausted or when ctx is cancelled, and
// never blocks past ctx cancellation trying to send into a full channel.
func Jobs(ctx context.Context, cfg SniperConfig, buffer int) <-chan engine.Job {
	out := make(chan engine.Job, buffer)

	go func() {
		defer close(out)

		markers := cfg.Template.Markers
		for markerIdx := range markers {
			cfg.Generator.Reset()
			for {
				val, ok := cfg.Generator.Next()
				if !ok {
					break
				}

				values := make([]string, len(markers))
				for i, m := range markers {
					if i == markerIdx {
						values[i] = val
					} else {
						values[i] = m.Original
					}
				}

				req, err := cfg.Template.Build(values, cfg.Target)
				if err != nil {
					// a build failure here means the template+payload
					// combination can't form a valid HTTP request at all —
					// a setup problem, not a per-request condition — so we
					// stop the run rather than silently skipping jobs
					return
				}

				job := engine.Job{
					Request: req,
					Meta: map[string]string{
						"marker":  strconv.Itoa(markerIdx),
						"payload": val,
					},
				}

				select {
				case out <- job:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}
