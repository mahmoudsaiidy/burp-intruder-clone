package attack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"intruder/internal/engine"
	"intruder/internal/payload"
	"intruder/internal/template"
)

func TestSniper_OneMarkerLiveAtATime(t *testing.T) {
	var mu sync.Mutex
	var seenQueries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seenQueries = append(seenQueries, r.URL.RawQuery)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}

	raw := "GET /x?u=§admin§&p=§pass§ HTTP/1.1\r\nHost: placeholder\r\n\r\n"
	tmpl, err := template.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tmpl.Markers) != 2 {
		t.Fatalf("expected 2 markers, got %d", len(tmpl.Markers))
	}

	gen := payload.NewWordlist([]string{"X", "Y"})
	cfg := SniperConfig{
		Template:  tmpl,
		Target:    template.TargetConfig{Host: u.Hostname(), Port: port},
		Generator: gen,
	}

	if cfg.TotalJobs() != 4 { // 2 markers * 2 payloads
		t.Fatalf("expected 4 total jobs, got %d", cfg.TotalJobs())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jobs := Jobs(ctx, cfg, 4)
	client := engine.NewClient(engine.DefaultClientConfig(2))
	pool := engine.NewPool(client, engine.PoolConfig{Workers: 2})

	count := 0
	for r := range pool.Run(ctx, jobs) {
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
		count++
	}
	if count != 4 {
		t.Fatalf("expected 4 results, got %d", count)
	}

	// marker 0 (u) live with p held at its original "pass", then marker 1
	// (p) live with u held at its original "admin" — never both live at once
	expected := map[string]bool{
		"u=X&p=pass":  true,
		"u=Y&p=pass":  true,
		"u=admin&p=X": true,
		"u=admin&p=Y": true,
	}
	if len(seenQueries) != 4 {
		t.Fatalf("expected 4 requests reaching the server, got %d", len(seenQueries))
	}
	for _, q := range seenQueries {
		if !expected[q] {
			t.Errorf("unexpected query seen: %q", q)
		}
		delete(expected, q)
	}
	if len(expected) != 0 {
		t.Errorf("expected queries never seen: %v", expected)
	}
}
