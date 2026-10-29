package match

import (
	"context"
	"testing"
	"time"

	"intruder/internal/engine"
)

func TestFlag_TagsEveryResultWithoutDroppingOrReordering(t *testing.T) {
	in := make(chan engine.Result)
	go func() {
		defer close(in)
		in <- engine.Result{Status: 200}
		in <- engine.Result{Status: 404}
		in <- engine.Result{Status: 200}
		in <- engine.Result{Status: 500}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out := Flag(ctx, in, StatusMatcher{Op: Equal, Value: 200})

	var got []Flagged
	for f := range out {
		got = append(got, f)
	}

	wantStatuses := []int{200, 404, 200, 500}
	wantMatched := []bool{true, false, true, false}
	if len(got) != len(wantStatuses) {
		t.Fatalf("expected %d flagged results, got %d", len(wantStatuses), len(got))
	}
	for i, f := range got {
		if f.Result.Status != wantStatuses[i] {
			t.Errorf("index %d: expected status %d, got %d (order not preserved)", i, wantStatuses[i], f.Result.Status)
		}
		if f.Matched != wantMatched[i] {
			t.Errorf("index %d: expected Matched=%v, got %v", i, wantMatched[i], f.Matched)
		}
	}
}

func TestFlag_ClosesOnContextCancellation(t *testing.T) {
	in := make(chan engine.Result) // never written to, never closed
	ctx, cancel := context.WithCancel(context.Background())

	out := Flag(ctx, in, StatusMatcher{Op: Equal, Value: 200})
	cancel()

	select {
	case _, ok := <-out:
		if ok {
			t.Fatalf("expected no results after cancellation")
		}
		// channel closed: correct
	case <-time.After(2 * time.Second):
		t.Fatalf("Flag did not close its output channel after ctx cancellation")
	}
}
