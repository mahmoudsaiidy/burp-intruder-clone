package config

import (
	"fmt"

	"intruder/internal/payload"
)

// GeneratorSpec is the JSON form of one payload.Generator: a base
// generator type plus its type-specific fields, optionally wrapped in a
// processing-rule Chain. Only the fields relevant to Type need be set —
// Build ignores the rest, the same convention JSON omitempty already
// implies on the way out.
type GeneratorSpec struct {
	// Type selects the base generator: "wordlist", "numeric_range", or
	// "brute_force".
	Type string `json:"type"`

	// Words is the payload list for Type == "wordlist".
	Words []string `json:"words,omitempty"`

	// From, To, Step and MinDigits configure Type == "numeric_range" —
	// see payload.NewNumericRange.
	From      int64 `json:"from,omitempty"`
	To        int64 `json:"to,omitempty"`
	Step      int64 `json:"step,omitempty"`
	MinDigits int   `json:"min_digits,omitempty"`

	// Charset, MinLen and MaxLen configure Type == "brute_force" — see
	// payload.NewBruteForce.
	Charset string `json:"charset,omitempty"`
	MinLen  int    `json:"min_len,omitempty"`
	MaxLen  int    `json:"max_len,omitempty"`

	// Rules, if non-empty, wraps the base generator above in a
	// payload.Chain, applying each rule in order to every value the base
	// generator produces — same order-sensitivity as payload.Chain itself.
	Rules []RuleSpec `json:"rules,omitempty"`
}

// Build turns the spec into a live payload.Generator. A GeneratorSpec with
// Rules set returns a *payload.Chain wrapping the base generator; one with
// none returns the base generator directly.
func (g GeneratorSpec) Build() (payload.Generator, error) {
	var base payload.Generator
	switch g.Type {
	case "wordlist":
		if len(g.Words) == 0 {
			return nil, fmt.Errorf("wordlist generator needs at least one word")
		}
		base = payload.NewWordlist(g.Words)
	case "numeric_range":
		base = payload.NewNumericRange(g.From, g.To, g.Step, g.MinDigits)
	case "brute_force":
		base = payload.NewBruteForce(g.Charset, g.MinLen, g.MaxLen)
	case "":
		return nil, fmt.Errorf("generator missing \"type\"")
	default:
		return nil, fmt.Errorf("unknown generator type %q", g.Type)
	}

	if len(g.Rules) == 0 {
		return base, nil
	}
	rules := make([]payload.Rule, 0, len(g.Rules))
	for i, rs := range g.Rules {
		r, err := rs.Build()
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		rules = append(rules, r)
	}
	return payload.NewChain(base, rules...), nil
}

// RuleSpec is the JSON form of one payload.Rule.
type RuleSpec struct {
	// Type selects the rule: "url_encode", "base64_encode", "prefix",
	// "suffix", "upper", or "lower".
	Type string `json:"type"`
	// Value is the text to prepend/append for "prefix"/"suffix"; unused by
	// every other rule type.
	Value string `json:"value,omitempty"`
}

// Build turns the spec into a live payload.Rule.
func (r RuleSpec) Build() (payload.Rule, error) {
	switch r.Type {
	case "url_encode":
		return payload.URLEncodeRule{}, nil
	case "base64_encode":
		return payload.Base64EncodeRule{}, nil
	case "prefix":
		return payload.PrefixRule{Prefix: r.Value}, nil
	case "suffix":
		return payload.SuffixRule{Suffix: r.Value}, nil
	case "upper":
		return payload.UpperCaseRule{}, nil
	case "lower":
		return payload.LowerCaseRule{}, nil
	case "":
		return nil, fmt.Errorf("rule missing \"type\"")
	default:
		return nil, fmt.Errorf("unknown rule type %q", r.Type)
	}
}
