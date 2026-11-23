package config

import (
	"fmt"
	"time"

	"intruder/pkg/payload"
)

// GeneratorSpec is the JSON form of one payload.Generator: a base
// generator type plus its type-specific fields, optionally wrapped in a
// processing-rule Chain. Only the fields relevant to Type need be set —
// Build ignores the rest, the same convention JSON omitempty already
// implies on the way out.
type GeneratorSpec struct {
	// Type selects the base generator: "wordlist", "numeric_range",
	// "brute_force", "char_substitution", "illegal_unicode", or
	// "date_range".
	Type string `json:"type"`

	// Words is the payload list for Type == "wordlist" or
	// Type == "char_substitution".
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

	// Substitutions configures Type == "char_substitution": each key is
	// exactly one character, and its value is the ordered list of
	// replacement strings payload.NewCharSubstitution should offer at that
	// character's positions, in addition to the original character itself
	// (JSON object keys are always strings, so a single-rune string is how
	// a rune key is spelled here — see payload.CharSubstitution's own
	// Substitutions field for the live map[rune][]string this becomes).
	Substitutions map[string][]string `json:"substitutions,omitempty"`

	// Chars configures Type == "illegal_unicode": every rune of this
	// string becomes one of payload.NewIllegalUnicode's input characters
	// (and a non-ASCII one among them is simply skipped at generation
	// time, same as passing it to that constructor directly).
	Chars string `json:"chars,omitempty"`

	// DateFrom, DateTo (both RFC 3339, e.g. "2024-01-01T00:00:00Z"), Step
	// (reused from above, here meaning seconds rather than an integer
	// step) and Layout (a Go reference-time layout; "" defaults to RFC
	// 3339, matching payload.DateRange's own zero-value default) configure
	// Type == "date_range" — see payload.NewDateRange.
	DateFrom string `json:"date_from,omitempty"`
	DateTo   string `json:"date_to,omitempty"`
	Layout   string `json:"layout,omitempty"`

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
	case "char_substitution":
		if len(g.Words) == 0 {
			return nil, fmt.Errorf("char_substitution generator needs at least one word")
		}
		subs := make(map[rune][]string, len(g.Substitutions))
		for k, v := range g.Substitutions {
			runes := []rune(k)
			if len(runes) != 1 {
				return nil, fmt.Errorf("char_substitution: substitution key %q must be exactly one character", k)
			}
			subs[runes[0]] = v
		}
		base = payload.NewCharSubstitution(g.Words, subs)
	case "illegal_unicode":
		if g.Chars == "" {
			return nil, fmt.Errorf("illegal_unicode generator needs at least one character in \"chars\"")
		}
		base = payload.NewIllegalUnicode([]rune(g.Chars))
	case "date_range":
		from, err := time.Parse(time.RFC3339, g.DateFrom)
		if err != nil {
			return nil, fmt.Errorf("date_range: parsing \"date_from\" %q: %w", g.DateFrom, err)
		}
		to, err := time.Parse(time.RFC3339, g.DateTo)
		if err != nil {
			return nil, fmt.Errorf("date_range: parsing \"date_to\" %q: %w", g.DateTo, err)
		}
		base = payload.NewDateRange(from, to, time.Duration(g.Step)*time.Second, g.Layout)
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
