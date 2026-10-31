package config

import (
	"fmt"
	"regexp"

	"intruder/internal/match"
)

// MatcherSpec is the JSON form of one match.Matcher, including the three
// combinators (and/or/not), so a whole match-rule tree can be described as
// data. Only the fields relevant to Type need be set.
type MatcherSpec struct {
	// Type selects the matcher: "status", "length", "body_regex",
	// "header_regex", "error", "and", "or", or "not".
	Type string `json:"type"`

	// Op is the comparison operator for "status"/"length": one of "eq",
	// "ne", "lt", "gt", "le", "ge" (also accepts "==", "!=", "<", ">",
	// "<=", ">="). Empty defaults to "eq".
	Op string `json:"op,omitempty"`
	// Value is the number "status"/"length" compares against.
	Value int64 `json:"value,omitempty"`

	// Pattern is the regular expression for "body_regex"/"header_regex".
	Pattern string `json:"pattern,omitempty"`
	// Header is the response header name for "header_regex".
	Header string `json:"header,omitempty"`

	// Children holds the sub-matchers for "and"/"or" (at least one each).
	Children []MatcherSpec `json:"children,omitempty"`
	// Child holds the single sub-matcher "not" inverts.
	Child *MatcherSpec `json:"child,omitempty"`
}

// Build turns the spec into a live match.Matcher, recursing into Children/
// Child for the and/or/not combinators.
func (m MatcherSpec) Build() (match.Matcher, error) {
	switch m.Type {
	case "status":
		op, err := parseOp(m.Op)
		if err != nil {
			return nil, err
		}
		return match.StatusMatcher{Op: op, Value: int(m.Value)}, nil
	case "length":
		op, err := parseOp(m.Op)
		if err != nil {
			return nil, err
		}
		return match.LengthMatcher{Op: op, Value: m.Value}, nil
	case "body_regex":
		re, err := regexp.Compile(m.Pattern)
		if err != nil {
			return nil, fmt.Errorf("body_regex pattern: %w", err)
		}
		return match.BodyRegexMatcher{Re: re}, nil
	case "header_regex":
		if m.Header == "" {
			return nil, fmt.Errorf("header_regex matcher needs \"header\"")
		}
		re, err := regexp.Compile(m.Pattern)
		if err != nil {
			return nil, fmt.Errorf("header_regex pattern: %w", err)
		}
		return match.HeaderRegexMatcher{Header: m.Header, Re: re}, nil
	case "error":
		return match.ErrorMatcher{}, nil
	case "and":
		children, err := buildMatchers(m.Children)
		if err != nil {
			return nil, err
		}
		if len(children) == 0 {
			return nil, fmt.Errorf("\"and\" matcher needs at least one child")
		}
		return match.And(children), nil
	case "or":
		children, err := buildMatchers(m.Children)
		if err != nil {
			return nil, err
		}
		if len(children) == 0 {
			return nil, fmt.Errorf("\"or\" matcher needs at least one child")
		}
		return match.Or(children), nil
	case "not":
		if m.Child == nil {
			return nil, fmt.Errorf("\"not\" matcher needs \"child\"")
		}
		inner, err := m.Child.Build()
		if err != nil {
			return nil, err
		}
		return match.Not{Matcher: inner}, nil
	case "":
		return nil, fmt.Errorf("matcher missing \"type\"")
	default:
		return nil, fmt.Errorf("unknown matcher type %q", m.Type)
	}
}

func buildMatchers(specs []MatcherSpec) ([]match.Matcher, error) {
	out := make([]match.Matcher, 0, len(specs))
	for i, s := range specs {
		m, err := s.Build()
		if err != nil {
			return nil, fmt.Errorf("child %d: %w", i, err)
		}
		out = append(out, m)
	}
	return out, nil
}

// parseOp maps a JSON comparison-operator string onto match.CompareOp.
// Empty string defaults to Equal, so a MatcherSpec that only cares about
// equality can omit Op entirely.
func parseOp(op string) (match.CompareOp, error) {
	switch op {
	case "", "eq", "==":
		return match.Equal, nil
	case "ne", "!=":
		return match.NotEqual, nil
	case "lt", "<":
		return match.LessThan, nil
	case "gt", ">":
		return match.GreaterThan, nil
	case "le", "<=":
		return match.LessOrEqual, nil
	case "ge", ">=":
		return match.GreaterOrEqual, nil
	default:
		return 0, fmt.Errorf("unknown comparison op %q", op)
	}
}
