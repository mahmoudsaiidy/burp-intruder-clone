package template

import (
	"io"
	"strconv"
	"testing"
)

func TestFindMarkers_Single(t *testing.T) {
	raw := "GET /login?password=§hunter2§ HTTP/1.1\r\nHost: x\r\n\r\n"
	markers, err := FindMarkers(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(markers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(markers))
	}
	if markers[0].Original != "hunter2" {
		t.Fatalf("expected original %q, got %q", "hunter2", markers[0].Original)
	}
}

func TestFindMarkers_Multiple(t *testing.T) {
	raw := "GET /login?u=§admin§&p=§pass§ HTTP/1.1\r\n\r\n"
	markers, err := FindMarkers(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(markers) != 2 {
		t.Fatalf("expected 2 markers, got %d", len(markers))
	}
	if markers[0].Original != "admin" || markers[1].Original != "pass" {
		t.Fatalf("unexpected marker contents: %+v", markers)
	}
}

func TestFindMarkers_Unmatched(t *testing.T) {
	raw := "GET /login?u=§admin HTTP/1.1\r\n\r\n"
	if _, err := FindMarkers(raw); err == nil {
		t.Fatal("expected error for unmatched marker, got nil")
	}
}

func TestInject(t *testing.T) {
	raw := "GET /login?u=§admin§&p=§pass§ HTTP/1.1\r\n\r\n"
	markers, err := FindMarkers(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := Inject(raw, markers, []string{"root", "toor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "GET /login?u=root&p=toor HTTP/1.1\r\n\r\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestBuild(t *testing.T) {
	raw := "GET /login?password=§pass§ HTTP/1.1\r\nHost: placeholder\r\n\r\n"
	tmpl, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	req, err := tmpl.Build([]string{"hunter2"}, TargetConfig{Host: "localhost", Port: 8089})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if req.URL.Scheme != "http" || req.URL.Host != "localhost:8089" {
		t.Fatalf("unexpected URL: %+v", req.URL)
	}
	if req.URL.RawQuery != "password=hunter2" {
		t.Fatalf("unexpected query: %q", req.URL.RawQuery)
	}
	if req.RequestURI != "" {
		t.Fatalf("RequestURI must be empty for client requests, got %q", req.RequestURI)
	}
}

// TestBuild_RecalculatesContentLengthForBodyMarker is a regression test for
// the bug documented in STATUS.md: a marker in the body whose payload is a
// different length than the original placeholder must not leave a stale
// Content-Length that truncates (or under-promises) the outgoing body.
func TestBuild_RecalculatesContentLengthForBodyMarker(t *testing.T) {
	raw := "POST /submit HTTP/1.1\r\nHost: placeholder\r\n" +
		"Content-Length: 17\r\n\r\n" +
		`{"token":"§t§"}`
	tmpl, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	payload := "a_much_longer_payload_value"
	req, err := tmpl.Build([]string{payload}, TargetConfig{Host: "localhost", Port: 8089})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	wantBody := `{"token":"` + payload + `"}`
	if req.ContentLength != int64(len(wantBody)) {
		t.Fatalf("req.ContentLength = %d, want %d", req.ContentLength, len(wantBody))
	}
	wantLen := strconv.Itoa(len(wantBody))
	if got := req.Header.Get("Content-Length"); got != wantLen {
		t.Fatalf("Content-Length header = %q, want %q", got, wantLen)
	}

	gotBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if string(gotBody) != wantBody {
		t.Fatalf("body = %q, want %q (payload must not be truncated)", gotBody, wantBody)
	}

	// req.Body must be re-readable via GetBody too, since http.Client's
	// Transport can call it (e.g. for redirects).
	if req.GetBody == nil {
		t.Fatal("expected GetBody to be set after body recalculation")
	}
	rc, err := req.GetBody()
	if err != nil {
		t.Fatalf("GetBody: %v", err)
	}
	gotBody2, _ := io.ReadAll(rc)
	if string(gotBody2) != wantBody {
		t.Fatalf("GetBody body = %q, want %q", gotBody2, wantBody)
	}
}

// TestBuild_ContentLengthUntouchedWhenBodyUnchanged guards against the fix
// above over-firing: when the marker is in the query string (not the
// body) and the body itself doesn't change size, Content-Length should
// come out exactly as it would have before the fix.
func TestBuild_ContentLengthUntouchedWhenBodyUnchanged(t *testing.T) {
	raw := "POST /login?password=§pass§ HTTP/1.1\r\nHost: placeholder\r\n" +
		"Content-Length: 11\r\n\r\n" +
		"same-length"
	tmpl, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	req, err := tmpl.Build([]string{"a-totally-different-length-payload"}, TargetConfig{Host: "localhost", Port: 8089})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if req.ContentLength != 11 {
		t.Fatalf("req.ContentLength = %d, want 11 (body never changed)", req.ContentLength)
	}
	if got := req.Header.Get("Content-Length"); got != "11" {
		t.Fatalf("Content-Length header = %q, want %q", got, "11")
	}

	gotBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if string(gotBody) != "same-length" {
		t.Fatalf("body = %q, want %q", gotBody, "same-length")
	}
}
