// Package config is the JSON-serializable description of one attack run —
// template, target, attack mode, payload generator(s), match rule, and an
// optional chain of setup stages — plus the code that turns it into the
// live objects internal/attack, internal/payload, internal/match and
// internal/chain already define, and runs them end to end.
//
// This is the "real attack configuration" step STATUS.md has been
// deferring since the web UI and CLI first ran a hardcoded Sniper demo:
// both of them hand-wired a template.Template, a payload.Generator, an
// attack.SniperConfig and an engine.Pool directly in Go. This package is
// that same wiring, but driven by data (a JSON document) instead of a
// source-code literal, so a request template, payload lists, match rules
// and chain stages submitted from a web form or loaded from a saved file
// can all build and run the exact same way the demo's Go code always did.
//
// AttackConfig is the wire format: every field is a plain JSON-friendly
// value (strings, numbers, nested specs), never a live Go interface value
// like payload.Generator or match.Matcher directly — those only exist
// after Build (or Run, which calls Build for you) turns the spec into the
// real objects. Validation happens in Build, in one place, rather than
// scattered across whatever UI collected the config.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"intruder/internal/template"
)

// Attack mode names, as they appear in AttackConfig.Mode. These match
// internal/attack's own four modes one-to-one.
const (
	ModeSniper       = "sniper"
	ModeBatteringRam = "battering_ram"
	ModePitchfork    = "pitchfork"
	ModeClusterBomb  = "cluster_bomb"
)

// TargetSpec is the JSON form of template.TargetConfig — connection info
// for an attack's own requests, or a chain.Stage's.
type TargetSpec struct {
	Host   string `json:"host"`
	Port   int    `json:"port,omitempty"`
	UseTLS bool   `json:"use_tls,omitempty"`
}

// Build converts the spec into the live template.TargetConfig attack and
// chain code actually takes. It never fails — every TargetSpec is a valid
// (if possibly useless, e.g. an empty Host) TargetConfig.
func (t TargetSpec) Build() template.TargetConfig {
	return template.TargetConfig{Host: t.Host, Port: t.Port, UseTLS: t.UseTLS}
}

// AttackConfig is one complete, submittable attack run: a raw request
// template with §marker§ injection points, where to send it, which attack
// mode cycles the markers, what generates the payloads, which responses
// get flagged, and (optionally) what setup requests must run first.
type AttackConfig struct {
	// Template is the raw HTTP request text, §marker§ delimiters and all
	// — exactly the format template.Parse accepts.
	Template string `json:"template"`
	// Target is the connection info for the attack's own requests (not any
	// chain stage's — each Stage carries its own Target, which may differ).
	Target TargetSpec `json:"target"`
	// Mode selects which of internal/attack's four job orderings runs the
	// generators against the template's markers. One of the Mode constants
	// above.
	Mode string `json:"mode"`
	// Generators supplies one payload.Generator (built from each spec) per
	// marker Mode actually needs: exactly 1 for Sniper/Battering ram
	// (replayed across every marker, or broadcast into all of them, per
	// mode), or exactly len(markers) for Pitchfork/Cluster bomb (marker
	// order), checked in Build once the template's own marker count is
	// known.
	Generators []GeneratorSpec `json:"generators"`
	// Matcher decides which results get flagged. Omit it to flag nothing —
	// the same "no conditions enabled" default Burp's own filter has — not
	// an error, since a run can be useful purely to eyeball every response.
	Matcher *MatcherSpec `json:"matcher,omitempty"`
	// Chain lists setup requests to run, in order, before the attack
	// itself — a login, a CSRF-token fetch — each one able to feed an
	// extracted value into a later stage or into Template itself. Omit it
	// for a run with no prerequisites.
	Chain []StageSpec `json:"chain,omitempty"`

	// Workers caps concurrent in-flight requests. <= 0 defaults to 5.
	Workers int `json:"workers,omitempty"`
	// RatePerSecond caps the whole run's request rate; 0 means unlimited,
	// same as engine.PoolConfig.
	RatePerSecond float64 `json:"rate_per_second,omitempty"`
	// JitterMaxMS adds up to this many extra milliseconds of random delay
	// per request, on top of RatePerSecond.
	JitterMaxMS int `json:"jitter_max_ms,omitempty"`
	// TimeoutSeconds bounds the whole run's wall-clock time, not any one
	// request (engine.ClientConfig.Timeout already bounds that, fixed at
	// 15s via engine.DefaultClientConfig). <= 0 defaults to 30s.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

// Load parses one AttackConfig from r. Unknown JSON fields are rejected —
// a typo in a hand-written or hand-edited config should fail loudly, not
// silently do nothing.
func Load(r io.Reader) (AttackConfig, error) {
	var c AttackConfig
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return AttackConfig{}, fmt.Errorf("config: parsing JSON: %w", err)
	}
	return c, nil
}

// LoadFile reads and parses an AttackConfig from a JSON file on disk.
func LoadFile(path string) (AttackConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return AttackConfig{}, fmt.Errorf("config: %w", err)
	}
	defer f.Close()
	return Load(f)
}
