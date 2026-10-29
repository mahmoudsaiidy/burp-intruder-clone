package template

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MarkerDelim is the character Burp-style templates use to bound an
// injection point: §payload§.
const MarkerDelim = '§'

// Marker is one §...§ delimited injection point found in a raw template.
type Marker struct {
	// Start and End are byte offsets into the ORIGINAL raw template text.
	// Start points at the opening § delimiter, End is just past the
	// closing § delimiter.
	Start, End int
	// Original is the sample text that sat between the delimiters. Burp
	// shows this as the pre-filled value in the UI; an attack run replaces
	// the whole §...§ span (delimiters and original text) with a payload.
	Original string
}

// FindMarkers scans raw for §...§ delimited pairs, in order, and returns
// them. Markers do not nest — each § toggles between "opening" and
// "closing" a marker, so "§a§§b§" is two markers, not one. An unmatched §
// (odd count) is a template error.
func FindMarkers(raw string) ([]Marker, error) {
	var markers []Marker
	openIdx := -1

	for idx := 0; idx < len(raw); {
		r, size := utf8.DecodeRuneInString(raw[idx:])
		if r == MarkerDelim {
			if openIdx == -1 {
				openIdx = idx
			} else {
				markers = append(markers, Marker{
					Start:    openIdx,
					End:      idx + size,
					Original: raw[openIdx+utf8.RuneLen(MarkerDelim) : idx],
				})
				openIdx = -1
			}
		}
		idx += size
	}

	if openIdx != -1 {
		return nil, fmt.Errorf("template: unmatched %q at byte offset %d", string(MarkerDelim), openIdx)
	}
	return markers, nil
}

// Inject returns raw with each marker's span replaced by the corresponding
// entry in values (same order as markers). len(values) must equal
// len(markers): callers decide what goes in "untouched" marker positions
// (pass marker.Original back through) — this function has no implicit
// behavior for that, since what belongs there differs by attack mode.
func Inject(raw string, markers []Marker, values []string) (string, error) {
	if len(values) != len(markers) {
		return "", fmt.Errorf("template: have %d marker(s) but %d payload value(s)", len(markers), len(values))
	}

	var b strings.Builder
	b.Grow(len(raw))
	prev := 0
	for i, m := range markers {
		b.WriteString(raw[prev:m.Start])
		b.WriteString(values[i])
		prev = m.End
	}
	b.WriteString(raw[prev:])
	return b.String(), nil
}
