// Package match implements Burp's "match/filter" concept: deciding which
// of an attack run's engine.Results are "interesting" (a login that
// succeeded, a response that's suspiciously short, a status code that
// stands out from the baseline) and, separately, pulling values back out
// of a response body or header for use elsewhere (a session token, a CSRF
// nonce). Matching and extraction are kept as separate concerns — a
// Matcher answers yes/no, an Extractor answers "here's the string" — since
// a result can be worth flagging without anything needing extracting from
// it, and vice versa.
package match

import (
	"cmp"
	"net/http"
	"regexp"

	"intruder/internal/engine"
)

// Matcher decides whether one Result is "interesting" by whatever
// criterion it implements.
type Matcher interface {
	Match(r engine.Result) bool
}

// CompareOp is a comparison operator for the numeric matchers (status code,
// response length). Burp's own match rules offer exactly these six.
type CompareOp int

const (
	Equal CompareOp = iota
	NotEqual
	LessThan
	GreaterThan
	LessOrEqual
	GreaterOrEqual
)

func compare[T cmp.Ordered](a T, op CompareOp, b T) bool {
	switch op {
	case Equal:
		return a == b
	case NotEqual:
		return a != b
	case LessThan:
		return a < b
	case GreaterThan:
		return a > b
	case LessOrEqual:
		return a <= b
	case GreaterOrEqual:
		return a >= b
	default:
		return false
	}
}

// StatusMatcher matches on HTTP response status code (e.g. "== 200",
// "!= 404"). A Result that errored before getting a status (r.Err != nil,
// r.Status == 0) never matches, regardless of Op/Value — there's no status
// to compare.
type StatusMatcher struct {
	Op    CompareOp
	Value int
}

func (m StatusMatcher) Match(r engine.Result) bool {
	if r.Err != nil {
		return false
	}
	return compare(r.Status, m.Op, m.Value)
}

// LengthMatcher matches on response body length in bytes — useful for
// spotting the one response that's shorter/longer than all the others
// (a common tell for a successful login, a different error page, etc.).
// Like StatusMatcher, a Result that errored never matches.
type LengthMatcher struct {
	Op    CompareOp
	Value int64
}

func (m LengthMatcher) Match(r engine.Result) bool {
	if r.Err != nil {
		return false
	}
	return compare(r.Length, m.Op, m.Value)
}

// BodyRegexMatcher matches when Re finds at least one match anywhere in
// the response body.
type BodyRegexMatcher struct {
	Re *regexp.Regexp
}

func (m BodyRegexMatcher) Match(r engine.Result) bool {
	if r.Err != nil || m.Re == nil {
		return false
	}
	return m.Re.Match(r.Body)
}

// HeaderRegexMatcher matches when Re finds at least one match in the named
// response header's value. Header lookup is case-insensitive, same as
// http.Header.Get.
type HeaderRegexMatcher struct {
	Header string
	Re     *regexp.Regexp
}

func (m HeaderRegexMatcher) Match(r engine.Result) bool {
	if r.Err != nil || m.Re == nil {
		return false
	}
	return m.Re.MatchString(headerValue(r.Headers, m.Header))
}

func headerValue(h http.Header, name string) string {
	if h == nil {
		return ""
	}
	return h.Get(name)
}

// ErrorMatcher matches Results where the request itself failed (timeout,
// connection refused, ...) rather than completing with any HTTP status.
// There's no corresponding "not an error" matcher needed — wrap this one
// in Not.
type ErrorMatcher struct{}

func (ErrorMatcher) Match(r engine.Result) bool { return r.Err != nil }

// And matches only when every one of its Matchers matches (short-circuits
// on the first that doesn't).
type And []Matcher

func (a And) Match(r engine.Result) bool {
	for _, m := range a {
		if !m.Match(r) {
			return false
		}
	}
	return true
}

// Or matches when any one of its Matchers matches (short-circuits on the
// first that does). An empty Or matches nothing, same as Burp's filter
// with no conditions enabled matching no responses.
type Or []Matcher

func (o Or) Match(r engine.Result) bool {
	for _, m := range o {
		if m.Match(r) {
			return true
		}
	}
	return false
}

// Not inverts another Matcher.
type Not struct {
	Matcher Matcher
}

func (n Not) Match(r engine.Result) bool { return !n.Matcher.Match(r) }
