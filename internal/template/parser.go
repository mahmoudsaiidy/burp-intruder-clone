// Package template handles raw HTTP request templates: parsing pasted
// request text, finding §payload§ injection points, and resolving a
// concrete payload set into a sendable *http.Request.
package template

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
)

// Template is a parsed request template: the raw HTTP text plus the
// injection markers found in it. It intentionally does not require the
// text to be a fully valid HTTP request at parse time — that's checked
// when a concrete request is built via Build, after payloads are injected.
type Template struct {
	Raw     string
	Markers []Marker
}

// Parse normalizes line endings and indexes markers in a raw HTTP request
// template. It does not attempt to fully validate the HTTP syntax yet.
func Parse(raw string) (*Template, error) {
	raw = normalizeLineEndings(raw)
	markers, err := FindMarkers(raw)
	if err != nil {
		return nil, err
	}
	return &Template{Raw: raw, Markers: markers}, nil
}

// normalizeLineEndings makes sure the template uses CRLF, since that's
// what HTTP/1.1 requires but pasted requests (browsers' "copy as curl",
// editors, this very chat) commonly use bare LF.
func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "\r\n")
	return s
}

// TargetConfig carries the connection info Burp keeps on its separate
// "Target" tab: scheme/host/port aren't recoverable from raw request text
// alone (the request line only has a path, and the Host header is just
// text) unless the caller says where to actually connect.
type TargetConfig struct {
	Host   string // hostname or IP, no port
	Port   int
	UseTLS bool
}

// Build resolves one payload set into a concrete, ready-to-send
// *http.Request. values must have exactly len(t.Markers) entries, in
// marker order — see Inject for how "leave this marker alone" positions
// should be filled (pass the marker's own Original value back in).
func (t *Template) Build(values []string, target TargetConfig) (*http.Request, error) {
	resolved, err := Inject(t.Raw, t.Markers, values)
	if err != nil {
		return nil, err
	}

	req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(resolved)))
	if err != nil {
		return nil, fmt.Errorf("template: resolved text is not a valid HTTP request: %w", err)
	}

	// http.ReadRequest parses as a server would: req.URL only carries the
	// path/query, and req.RequestURI is set (which http.Client refuses to
	// send on an outgoing request). Scheme/host come from TargetConfig, not
	// from the request line or even the Host header, since a pasted request
	// is frequently proxied/rewritten and the Host header is just a text
	// field at that point, not a connection instruction.
	scheme := "http"
	if target.UseTLS {
		scheme = "https"
	}
	host := target.Host
	if target.Port != 0 && !isDefaultPort(target.Port, target.UseTLS) {
		host = fmt.Sprintf("%s:%d", target.Host, target.Port)
	}

	req.URL.Scheme = scheme
	req.URL.Host = host
	req.RequestURI = ""
	if req.Host == "" {
		req.Host = host
	}
	if req.ContentLength == 0 {
		req.Body = http.NoBody
	}

	return req, nil
}

func isDefaultPort(port int, tls bool) bool {
	return (tls && port == 443) || (!tls && port == 80)
}
