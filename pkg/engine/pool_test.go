package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPool_FiresAllJobsConcurrently(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := NewClient(DefaultClientConfig(10))
	pool := NewPool(client, PoolConfig{Workers: 10})

	const n = 50
	jobs := make(chan Job, n)
	for i := 0; i < n; i++ {
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		jobs <- Job{Request: req}
	}
	close(jobs)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results := pool.Run(ctx, jobs)
	count := 0
	for r := range results {
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
		if r.Status != http.StatusOK {
			t.Fatalf("unexpected status: %d", r.Status)
		}
		if r.Length != 2 {
			t.Fatalf("unexpected length: %d", r.Length)
		}
		count++
	}
	if count != n {
		t.Fatalf("expected %d results, got %d", n, count)
	}
	if atomic.LoadInt32(&hits) != n {
		t.Fatalf("expected %d server hits, got %d", n, hits)
	}
}

func TestPool_CancellationStopsNewJobs(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // hang until the test releases it
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(DefaultClientConfig(2))
	pool := NewPool(client, PoolConfig{Workers: 2})

	jobs := make(chan Job, 100)
	for i := 0; i < 100; i++ {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		jobs <- Job{Request: req}
	}
	close(jobs)

	ctx, cancel := context.WithCancel(context.Background())
	results := pool.Run(ctx, jobs)

	// let the 2 workers pick up their first (now-hanging) requests, then
	// cancel before the server ever unblocks them
	time.Sleep(50 * time.Millisecond)
	cancel()

	// release the handler so the in-flight requests can finish, then drain
	close(block)

	deadline := time.After(2 * time.Second)
	count := 0
drain:
	for {
		select {
		case _, ok := <-results:
			if !ok {
				break drain
			}
			count++
		case <-deadline:
			t.Fatal("timed out waiting for results channel to close after cancellation")
		}
	}

	// only the handful of requests the 2 workers had already picked up
	// before cancel should have been fired — nowhere near all 100
	if count >= 100 {
		t.Fatalf("expected cancellation to stop new jobs, but got %d results", count)
	}
	t.Logf("completed %d requests before/around cancellation (expected a small number)", count)
}
