package payload

import (
	"testing"
	"time"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return tm
}

func TestDateRange_BasicSequence(t *testing.T) {
	d := NewDateRange(mustDate(t, "2024-01-01"), mustDate(t, "2024-01-04"), 24*time.Hour, "2006-01-02")
	if d.Len() != 4 {
		t.Fatalf("expected Len 4, got %d", d.Len())
	}

	var got []string
	for {
		v, ok := d.Next()
		if !ok {
			break
		}
		got = append(got, v)
	}
	want := []string{"2024-01-01", "2024-01-02", "2024-01-03", "2024-01-04"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDateRange_StepSkipsPastTo(t *testing.T) {
	// Jan 1, 3, 5 — Jan 6 (the range end) is never hit, same convention
	// NumericRange's own step test uses.
	d := NewDateRange(mustDate(t, "2024-01-01"), mustDate(t, "2024-01-06"), 48*time.Hour, "2006-01-02")
	if d.Len() != 3 {
		t.Fatalf("expected Len 3, got %d", d.Len())
	}
	var got []string
	for {
		v, ok := d.Next()
		if !ok {
			break
		}
		got = append(got, v)
	}
	want := []string{"2024-01-01", "2024-01-03", "2024-01-05"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDateRange_DefaultLayout(t *testing.T) {
	from := mustDate(t, "2024-01-01")
	d := NewDateRange(from, from, time.Hour, "")
	v, ok := d.Next()
	if !ok {
		t.Fatalf("expected one value")
	}
	if v != from.Format(time.RFC3339) {
		t.Fatalf("got %q, want RFC3339-formatted %q", v, from.Format(time.RFC3339))
	}
}

func TestDateRange_InvalidRangeProducesNothing(t *testing.T) {
	d := NewDateRange(mustDate(t, "2024-01-05"), mustDate(t, "2024-01-01"), time.Hour, "")
	if d.Len() != 0 {
		t.Fatalf("expected Len 0 for From after To, got %d", d.Len())
	}
	if _, ok := d.Next(); ok {
		t.Fatalf("expected no values for From after To")
	}

	zeroStep := NewDateRange(mustDate(t, "2024-01-01"), mustDate(t, "2024-01-05"), 0, "")
	if zeroStep.Len() != 0 {
		t.Fatalf("expected Len 0 for Step 0, got %d", zeroStep.Len())
	}
	if _, ok := zeroStep.Next(); ok {
		t.Fatalf("expected no values for Step 0")
	}
}

func TestDateRange_Reset(t *testing.T) {
	d := NewDateRange(mustDate(t, "2024-01-01"), mustDate(t, "2024-01-03"), 24*time.Hour, "2006-01-02")
	for {
		if _, ok := d.Next(); !ok {
			break
		}
	}
	d.Reset()
	v, ok := d.Next()
	if !ok || v != "2024-01-01" {
		t.Fatalf("expected Reset to rewind to first value, got %q ok=%v", v, ok)
	}
}

func TestDateRange_ComposesWithChain(t *testing.T) {
	d := NewDateRange(mustDate(t, "2024-01-01"), mustDate(t, "2024-01-01"), time.Hour, "2006-01-02")
	c := NewChain(d, PrefixRule{Prefix: "date="})

	v, _ := c.Next()
	if v != "date=2024-01-01" {
		t.Fatalf("got %q, want %q", v, "date=2024-01-01")
	}
}
