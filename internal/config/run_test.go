package config

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

// testTargetSpec turns an httptest.Server's URL into a TargetSpec, the
// same way a submitted config's "target" field would name a real server.
func testTargetSpec(t *testing.T, ts *httptest.Server) TargetSpec {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host:port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return TargetSpec{Host: host, Port: port}
}

// TestRun_Sniper is an end-to-end run built entirely from JSON: a Sniper
// attack against a mock /login endpoint, flagging the one payload that
// returns 200 — the same scenario cmd/cli's hardcoded demo proves, but
// driven by a config.AttackConfig instead of Go struct literals.
func TestRun_Sniper(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("password") == "hunter2" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "welcome")
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	target := testTargetSpec(t, ts)
	cfg := AttackConfig{
		Template: "GET /login?password=§pass§ HTTP/1.1\r\nHost: x\r\n\r\n",
		Target:   target,
		Mode:     ModeSniper,
		Generators: []GeneratorSpec{
			{Type: "wordlist", Words: []string{"wrong1", "hunter2", "wrong2"}},
		},
		Matcher: &MatcherSpec{Type: "status", Value: 200},
		Workers: 3,
	}

	flagged, total, err := cfg.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}

	count, hits := 0, 0
	for f := range flagged {
		count++
		if f.Matched {
			hits++
			if f.Result.Job.Meta["payload"] != "hunter2" {
				t.Errorf("flagged payload = %q, want hunter2", f.Result.Job.Meta["payload"])
			}
		}
	}
	if count != 3 {
		t.Fatalf("got %d results, want 3", count)
	}
	if hits != 1 {
		t.Fatalf("got %d hits, want 1", hits)
	}
}

// TestRun_Pitchfork proves the two-generator mode works from JSON too:
// a two-field template, one generator per marker, walked in lockstep.
func TestRun_Pitchfork(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("user") == "admin" && q.Get("pass") == "s3cret" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	target := testTargetSpec(t, ts)
	cfg := AttackConfig{
		Template: "GET /login?user=§u§&pass=§p§ HTTP/1.1\r\nHost: x\r\n\r\n",
		Target:   target,
		Mode:     ModePitchfork,
		Generators: []GeneratorSpec{
			{Type: "wordlist", Words: []string{"admin", "root"}},
			{Type: "wordlist", Words: []string{"s3cret", "toor"}},
		},
		Matcher: &MatcherSpec{Type: "status", Value: 200},
	}

	flagged, total, err := cfg.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}

	hits := 0
	for f := range flagged {
		if f.Matched {
			hits++
		}
	}
	if hits != 1 {
		t.Fatalf("got %d hits, want 1", hits)
	}
}

// TestRun_WithChain proves a config-described chain stage really runs
// before, and feeds into, the main attack: a CSRF-token endpoint whose
// value must be present on /login for any password to be accepted.
func TestRun_WithChain(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/csrf", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "token=abc123")
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("csrf") != "abc123" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if q.Get("password") == "hunter2" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	target := testTargetSpec(t, ts)
	cfg := AttackConfig{
		Template: "GET /login?csrf={{csrf}}&password=§pass§ HTTP/1.1\r\nHost: x\r\n\r\n",
		Target:   target,
		Mode:     ModeSniper,
		Generators: []GeneratorSpec{
			{Type: "wordlist", Words: []string{"wrong", "hunter2"}},
		},
		Matcher: &MatcherSpec{Type: "status", Value: 200},
		Chain: []StageSpec{
			{
				Name:        "csrf",
				Raw:         "GET /csrf HTTP/1.1\r\nHost: x\r\n\r\n",
				Target:      target,
				Extractor:   ExtractorSpec{Type: "body_regex", Pattern: `token=(\w+)`},
				Placeholder: "{{csrf}}",
			},
		},
	}

	flagged, _, err := cfg.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	hits := 0
	for f := range flagged {
		if f.Result.Err != nil {
			t.Fatalf("unexpected request error: %v", f.Result.Err)
		}
		if f.Matched {
			hits++
		}
	}
	if hits != 1 {
		t.Fatalf("got %d hits, want 1 (chain's csrf token must have reached every request)", hits)
	}
}

func TestRun_BuildErrorSurfacesBeforeAnyRequest(t *testing.T) {
	cfg := AttackConfig{Template: "", Mode: ModeSniper}
	if _, _, err := cfg.Run(context.Background()); err == nil {
		t.Fatal("expected an error for an invalid config, got nil")
	}
}
