// Package chain resolves multi-stage request chaining: firing one or more
// setup requests before the main attack, pulling a value out of each
// response (a CSRF token, a session cookie, ...), and substituting those
// values into text — a later stage's own request template, and finally the
// main attack's raw template — before that text is handed to
// template.Parse.
//
// This is deliberately orchestrated one level above both internal/template
// and internal/attack rather than being a feature of either: a chain Stage
// is just another request+extract step (template.Template to build it,
// match.Extractor to read it, exactly the primitives those packages already
// expose), and what it produces is plain text that the attack's own
// §marker§ parsing has not seen yet. So an attack mode's Jobs() needs zero
// changes to run against a chain-resolved template — it's given the same
// kind of raw text it already knows how to parse, just with the chain's
// placeholders already swapped out, and the attack's own §payload§ markers
// (which chain resolution never touches) still intact for it to cycle
// through as usual.
package chain

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"intruder/internal/engine"
	"intruder/internal/match"
	"intruder/internal/template"
)

// maxBodyBytes caps how much of a stage response body Resolve reads, same
// default engine.Pool uses for attack responses — a setup request has no
// reason to need more.
const maxBodyBytes = 5 << 20

// Stage is one prerequisite request fired before the main attack to obtain
// a value the main attack's template (or a later Stage's own template)
// needs: a CSRF token, a fresh session cookie, anything a login or
// token-fetch step would return.
type Stage struct {
	// Name identifies the stage in errors and in Resolve's returned values
	// map. Must be non-empty and unique among the stages passed to one
	// Resolve call.
	Name string
	// Raw is this stage's own raw HTTP request template text — the same
	// format template.Parse accepts, §marker§ delimiters included if this
	// stage's own request needs one. Before being parsed, every earlier
	// stage's Placeholder occurring in Raw is substituted for that stage's
	// extracted value, so a later stage can depend on an earlier one (e.g.
	// a CSRF-token stage that must present the session cookie a login
	// stage obtained).
	Raw string
	// MarkerValues fills any §markers§ Raw contains, in marker order — same
	// contract as template.Template.Build. Most setup requests have none,
	// so this is usually nil.
	MarkerValues []string
	// Target is the connection info for this stage's own request. Stages
	// commonly target the same host as the main attack, but nothing
	// requires that (an OAuth token endpoint on a different host, say).
	Target template.TargetConfig
	// Extractor pulls this stage's value out of its response. Required —
	// a stage with no Extractor can't produce anything for later steps to
	// use, which means it isn't doing chaining work at all.
	Extractor match.Extractor
	// Placeholder is the literal substring that gets replaced with this
	// stage's extracted value, in every later stage's Raw and in the final
	// attack template text. Leave it empty to extract a value purely for
	// inspection (it still comes back in Resolve's values map, keyed by
	// Name) without substituting it anywhere.
	Placeholder string
}

// Resolve runs every stage in order — sequentially, never concurrently,
// since a later stage's own request text may depend on an earlier stage's
// extracted value — then substitutes every stage's Placeholder for its
// extracted value in attackRaw (the main attack's own raw template text,
// §payload§ markers and all) and returns the result. attackRaw's own
// §markers§ are never touched: chain placeholders and attack markers are
// different substitution mechanisms by design, precisely so the attack's
// per-request payload markers and the chain's once-per-run setup values
// can't be confused with each other.
//
// A stage that fails to build, fails to fire, or fails to extract a value
// stops the whole chain immediately with an error identifying that stage —
// consistent with how the rest of this codebase treats a setup problem
// (as opposed to a per-request condition like a non-2xx status): there's no
// sensible way to continue with a placeholder that has no value.
//
// On success, the returned map holds every stage's extracted value, keyed
// by Stage.Name, regardless of whether that stage had a Placeholder — useful
// for logging/debugging a chain even when not every extracted value is
// meant to be substituted into request text.
func Resolve(ctx context.Context, client *http.Client, stages []Stage, attackRaw string) (resolvedAttackRaw string, extracted map[string]string, err error) {
	extracted = make(map[string]string, len(stages))
	substitutions := make(map[string]string, len(stages))
	placeholderOwner := make(map[string]string, len(stages)) // placeholder -> the stage Name that claimed it

	for _, s := range stages {
		if s.Name == "" {
			return "", nil, fmt.Errorf("chain: a stage has no Name")
		}
		if _, dup := extracted[s.Name]; dup {
			return "", nil, fmt.Errorf("chain: duplicate stage name %q", s.Name)
		}
		if s.Extractor == nil {
			return "", nil, fmt.Errorf("chain: stage %q has no Extractor configured", s.Name)
		}

		raw := substitute(s.Raw, substitutions)

		tmpl, err := template.Parse(raw)
		if err != nil {
			return "", nil, fmt.Errorf("chain: stage %q: parsing template: %w", s.Name, err)
		}

		req, err := tmpl.Build(s.MarkerValues, s.Target)
		if err != nil {
			return "", nil, fmt.Errorf("chain: stage %q: building request: %w", s.Name, err)
		}

		result := execute(ctx, client, req)
		if result.Err != nil {
			return "", nil, fmt.Errorf("chain: stage %q: request failed: %w", s.Name, result.Err)
		}

		val, ok := s.Extractor.Extract(result)
		if !ok {
			return "", nil, fmt.Errorf("chain: stage %q: extractor found no value in the response (status %d)", s.Name, result.Status)
		}

		extracted[s.Name] = val
		if s.Placeholder != "" {
			if prev, dup := placeholderOwner[s.Placeholder]; dup {
				return "", nil, fmt.Errorf("chain: stage %q and stage %q both use placeholder %q", prev, s.Name, s.Placeholder)
			}
			placeholderOwner[s.Placeholder] = s.Name
			substitutions[s.Placeholder] = val
		}
	}

	return substitute(attackRaw, substitutions), extracted, nil
}

// substitute replaces every placeholder key in raw with its value. Order
// across keys is unspecified (map iteration), which is only safe because
// placeholders are expected to be disjoint literal substrings — the same
// assumption reverseLookup enforces by rejecting two stages that share one.
func substitute(raw string, substitutions map[string]string) string {
	for placeholder, val := range substitutions {
		raw = strings.ReplaceAll(raw, placeholder, val)
	}
	return raw
}

// execute fires a single, already-built request and reports it as an
// engine.Result, the same shape attack results use, so a Stage's Extractor
// is exactly the match.Extractor the main attack's results would use — no
// separate "setup response" type. It intentionally doesn't go through
// engine.Pool: a chain stage is one request fired once, not a batch to run
// concurrently and rate-limit the way an attack's job stream is.
func execute(ctx context.Context, client *http.Client, req *http.Request) engine.Result {
	start := time.Now()
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return engine.Result{Duration: time.Since(start), Err: err}
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	duration := time.Since(start)
	if readErr != nil {
		return engine.Result{Status: resp.StatusCode, Duration: duration, Headers: resp.Header, Err: readErr}
	}

	return engine.Result{
		Status:   resp.StatusCode,
		Length:   int64(len(body)),
		Duration: duration,
		Headers:  resp.Header,
		Body:     body,
	}
}
