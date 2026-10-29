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

func TestClusterBomb_FullCombinatorialProduct(t *testing.T) {
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

	userGen := payload.NewWordlist([]string{"alice", "bob"})
	passGen := payload.NewWordlist([]string{"p1", "p2", "p3"})

	cfg := ClusterBombConfig{
		Template:   tmpl,
		Target:     template.TargetConfig{Host: u.Hostname(), Port: port},
		Generators: []payload.Generator{userGen, passGen},
	}

	if cfg.TotalJobs() != 6 { // 2 * 3, every combination
		t.Fatalf("expected 6 total jobs, got %d", cfg.TotalJobs())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jobs := ClusterBombJobs(ctx, cfg, 8)
	client := engine.NewClient(engine.DefaultClientConfig(2))
	pool := engine.NewPool(client, engine.PoolConfig{Workers: 2})

	count := 0
	for r := range pool.Run(ctx, jobs) {
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
		count++
	}
	if count != 6 {
		t.Fatalf("expected 6 results, got %d", count)
	}

	expected := map[string]bool{
		"u=alice&p=p1": true, "u=alice&p=p2": true, "u=alice&p=p3": true,
		"u=bob&p=p1": true, "u=bob&p=p2": true, "u=bob&p=p3": true,
	}
	if len(seenQueries) != 6 {
		t.Fatalf("expected 6 requests reaching the server, got %d", len(seenQueries))
	}
	for _, q := range seenQueries {
		if !expected[q] {
			t.Errorf("unexpected query seen: %q", q)
		}
		delete(expected, q)
	}
	if len(expected) != 0 {
		t.Errorf("expected combinations never seen: %v", expected)
	}
}

func TestClusterBomb_EmptyGeneratorProducesNoJobs(t *testing.T) {
	raw := "GET /x?u=§admin§&p=§pass§ HTTP/1.1\r\nHost: placeholder\r\n\r\n"
	tmpl, err := template.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	cfg := ClusterBombConfig{
		Template: tmpl,
		Target:   template.TargetConfig{Host: "127.0.0.1", Port: 1},
		Generators: []payload.Generator{
			payload.NewWordlist([]string{"alice", "bob"}),
			payload.NewWordlist(nil), // empty: product must be empty
		},
	}

	if cfg.TotalJobs() != 0 {
		t.Fatalf("expected 0 total jobs, got %d", cfg.TotalJobs())
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	jobs := ClusterBombJobs(ctx, cfg, 4)
	count := 0
	for range jobs {
		count++
	}
	if count != 0 {
		t.Fatalf("expected 0 jobs with an empty generator, got %d", count)
	}
}
