package attack

import (
	"context"

	"intruder/internal/engine"
	"intruder/internal/payload"
	"intruder/internal/template"
)

// BatteringRamConfig configures a Battering ram run: a single payload
// generator whose value is written into every marker position at once, on
// every iteration (unlike Sniper, which holds all but one marker at its
// original value).
type BatteringRamConfig struct {
	Template  *template.Template
	Target    template.TargetConfig
	Generator payload.Generator
}

// TotalJobs returns the number of requests a Battering ram run with this
// config will produce: one request per payload value, regardless of how
// many markers the template has (every marker gets the same value on each
// request, so the generator is only walked once, not once per marker).
func (c BatteringRamConfig) TotalJobs() int {
	return c.Generator.Len()
}

// Jobs streams the full Battering ram job sequence onto a channel. It
// closes the channel when the sequence is exhausted or when ctx is
// cancelled, and never blocks past ctx cancellation trying to send into a
// full channel.
func BatteringRamJobs(ctx context.Context, cfg BatteringRamConfig, buffer int) <-chan engine.Job {
	out := make(chan engine.Job, buffer)

	go func() {
		defer close(out)

		markers := cfg.Template.Markers
		cfg.Generator.Reset()
		for {
			val, ok := cfg.Generator.Next()
			if !ok {
				break
			}

			values := make([]string, len(markers))
			for i := range markers {
				values[i] = val
			}

			req, err := cfg.Template.Build(values, cfg.Target)
			if err != nil {
				// same reasoning as Sniper: a build failure here is a
				// setup problem (template+payload can't form a valid
				// request at all), not a per-request condition, so stop
				// the run rather than silently skipping jobs
				return
			}

			job := engine.Job{
				Request: req,
				Meta: map[string]string{
					"payload": val,
				},
			}

			select {
			case out <- job:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}
