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

func TestBatteringRam_SameValueInEveryMarker(t *testing.T) {
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

	raw := "GET /x?u=§admin§&p=§admin§ HTTP/1.1\r\nHost: placeholder\r\n\r\n"
	tmpl, err := template.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tmpl.Markers) != 2 {
		t.Fatalf("expected 2 markers, got %d", len(tmpl.Markers))
	}

	gen := payload.NewWordlist([]string{"X", "Y", "Z"})
	cfg := BatteringRamConfig{
		Template:  tmpl,
		Target:    template.TargetConfig{Host: u.Hostname(), Port: port},
		Generator: gen,
	}

	if cfg.TotalJobs() != 3 { // one request per payload value, not per marker
		t.Fatalf("expected 3 total jobs, got %d", cfg.TotalJobs())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jobs := BatteringRamJobs(ctx, cfg, 4)
	client := engine.NewClient(engine.DefaultClientConfig(2))
	pool := engine.NewPool(client, engine.PoolConfig{Workers: 2})

	count := 0
	for r := range pool.Run(ctx, jobs) {
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
		count++
	}
	if count != 3 {
		t.Fatalf("expected 3 results, got %d", count)
	}

	expected := map[string]bool{
		"u=X&p=X": true,
		"u=Y&p=Y": true,
		"u=Z&p=Z": true,
	}
	if len(seenQueries) != 3 {
		t.Fatalf("expected 3 requests reaching the server, got %d", len(seenQueries))
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
