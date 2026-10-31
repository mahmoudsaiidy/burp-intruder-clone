package config

import (
	"fmt"
	"time"

	"intruder/internal/chain"
	"intruder/internal/match"
	"intruder/internal/payload"
	"intruder/internal/template"
)

// Built is an AttackConfig after Build has parsed, constructed and
// validated every piece of it — a parsed Template, live Generators and
// Matcher, resolved defaults — ready to Run.
type Built struct {
	Template   *template.Template
	Target     template.TargetConfig
	Mode       string
	Generators []payload.Generator
	Matcher    match.Matcher
	Stages     []chain.Stage

	Workers       int
	RatePerSecond float64
	JitterMax     time.Duration
	Timeout       time.Duration
}

// Build validates c and constructs every live object it describes:
// parsing Template, building each Generator and the Matcher, checking that
// Mode and the number of Generators agree with each other and with the
// template's own marker count, and applying defaults for Workers/Timeout.
// All validation happens here, in one place, rather than being left for
// Run (or a caller) to discover mid-attack.
func (c AttackConfig) Build() (*Built, error) {
	if c.Template == "" {
		return nil, fmt.Errorf("config: \"template\" is required")
	}
	tmpl, err := template.Parse(c.Template)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	switch c.Mode {
	case ModeSniper, ModeBatteringRam, ModePitchfork, ModeClusterBomb:
	case "":
		return nil, fmt.Errorf("config: \"mode\" is required")
	default:
		return nil, fmt.Errorf("config: unknown mode %q", c.Mode)
	}

	if len(c.Generators) == 0 {
		return nil, fmt.Errorf("config: at least one generator is required")
	}
	gens := make([]payload.Generator, 0, len(c.Generators))
	for i, gs := range c.Generators {
		g, err := gs.Build()
		if err != nil {
			return nil, fmt.Errorf("config: generator %d: %w", i, err)
		}
		gens = append(gens, g)
	}

	switch c.Mode {
	case ModeSniper, ModeBatteringRam:
		if len(gens) != 1 {
			return nil, fmt.Errorf("config: mode %q needs exactly 1 generator, got %d", c.Mode, len(gens))
		}
	case ModePitchfork, ModeClusterBomb:
		if len(gens) != len(tmpl.Markers) {
			return nil, fmt.Errorf("config: mode %q needs one generator per marker (template has %d), got %d",
				c.Mode, len(tmpl.Markers), len(gens))
		}
	}

	// No Matcher configured means "flag nothing" — the same "no conditions
	// enabled" default Burp's own filter has, not an error: a run can be
	// useful purely to eyeball every response.
	matcher := match.Matcher(match.Or{})
	if c.Matcher != nil {
		matcher, err = c.Matcher.Build()
		if err != nil {
			return nil, fmt.Errorf("config: matcher: %w", err)
		}
	}

	stages := make([]chain.Stage, 0, len(c.Chain))
	for i, ss := range c.Chain {
		st, err := ss.Build()
		if err != nil {
			return nil, fmt.Errorf("config: chain stage %d: %w", i, err)
		}
		stages = append(stages, st)
	}

	workers := c.Workers
	if workers <= 0 {
		workers = 5
	}
	timeout := time.Duration(c.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &Built{
		Template:      tmpl,
		Target:        c.Target.Build(),
		Mode:          c.Mode,
		Generators:    gens,
		Matcher:       matcher,
		Stages:        stages,
		Workers:       workers,
		RatePerSecond: c.RatePerSecond,
		JitterMax:     time.Duration(c.JitterMaxMS) * time.Millisecond,
		Timeout:       timeout,
	}, nil
}
