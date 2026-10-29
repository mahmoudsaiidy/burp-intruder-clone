package match

import (
	"regexp"

	"intruder/internal/engine"
)

// Extractor pulls one string value out of a Result — a session token, a
// CSRF nonce, anything a later request (a chained attack, a follow-up
// request outside this package's scope today) might need. Matching
// ("is this result interesting") and extraction ("here's a value from it")
// are deliberately separate: callers typically match first, then extract
// only from the results that matched.
type Extractor interface {
	// Extract returns the extracted value and true, or "" and false if
	// nothing could be extracted (the Result errored, or the pattern
	// didn't match).
	Extract(r engine.Result) (value string, ok bool)
}

// BodyRegexExtractor pulls a value out of the response body using Re. If
// Re has at least one capture group, the first group's text is returned;
// otherwise the whole match is returned. This mirrors Burp's "extract
// grammar" by regex, where a capture group (if present) narrows the
// extracted value down from the full match.
type BodyRegexExtractor struct {
	Re *regexp.Regexp
}

func (e BodyRegexExtractor) Extract(r engine.Result) (string, bool) {
	if r.Err != nil || e.Re == nil {
		return "", false
	}
	return extractFrom(e.Re, r.Body)
}

// HeaderRegexExtractor pulls a value out of the named response header's
// value using Re, with the same capture-group convention as
// BodyRegexExtractor.
type HeaderRegexExtractor struct {
	Header string
	Re     *regexp.Regexp
}

func (e HeaderRegexExtractor) Extract(r engine.Result) (string, bool) {
	if r.Err != nil || e.Re == nil {
		return "", false
	}
	return extractFrom(e.Re, []byte(headerValue(r.Headers, e.Header)))
}

func extractFrom(re *regexp.Regexp, data []byte) (string, bool) {
	m := re.FindSubmatch(data)
	if m == nil {
		return "", false
	}
	if len(m) > 1 {
		// group 1 may legitimately match empty text ("" is still a real,
		// found value); only a nil submatch slice means "group didn't
		// participate in the match" and falls back to the full match
		if m[1] != nil {
			return string(m[1]), true
		}
	}
	return string(m[0]), true
}
