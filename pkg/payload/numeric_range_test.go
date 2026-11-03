package payload

import "testing"

func TestNumericRange_BasicSequence(t *testing.T) {
	n := NewNumericRange(1, 5, 1, 0)
	if n.Len() != 5 {
		t.Fatalf("expected Len 5, got %d", n.Len())
	}

	var got []string
	for {
		v, ok := n.Next()
		if !ok {
			break
		}
		got = append(got, v)
	}
	want := []string{"1", "2", "3", "4", "5"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestNumericRange_Step(t *testing.T) {
	n := NewNumericRange(0, 10, 3, 0)
	// 0, 3, 6, 9 — 10 is never hit, matching a plain for-loop with step 3
	if n.Len() != 4 {
		t.Fatalf("expected Len 4, got %d", n.Len())
	}
	var got []string
	for {
		v, ok := n.Next()
		if !ok {
			break
		}
		got = append(got, v)
	}
	want := []string{"0", "3", "6", "9"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestNumericRange_ZeroPadding(t *testing.T) {
	n := NewNumericRange(8, 11, 1, 3)
	var got []string
	for {
		v, ok := n.Next()
		if !ok {
			break
		}
		got = append(got, v)
	}
	want := []string{"008", "009", "010", "011"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestNumericRange_NegativeNumbersPadDigitsNotSign(t *testing.T) {
	n := NewNumericRange(-2, 1, 1, 3)
	var got []string
	for {
		v, ok := n.Next()
		if !ok {
			break
		}
		got = append(got, v)
	}
	want := []string{"-002", "-001", "000", "001"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestNumericRange_InvalidRangeProducesNothing(t *testing.T) {
	n := NewNumericRange(10, 1, 1, 0) // From > To
	if n.Len() != 0 {
		t.Fatalf("expected Len 0 for From > To, got %d", n.Len())
	}
	if _, ok := n.Next(); ok {
		t.Fatalf("expected no values for From > To")
	}

	zeroStep := NewNumericRange(1, 10, 0, 0)
	if zeroStep.Len() != 0 {
		t.Fatalf("expected Len 0 for Step 0, got %d", zeroStep.Len())
	}
	if _, ok := zeroStep.Next(); ok {
		t.Fatalf("expected no values for Step 0")
	}
}

func TestNumericRange_Reset(t *testing.T) {
	n := NewNumericRange(1, 3, 1, 0)
	for {
		if _, ok := n.Next(); !ok {
			break
		}
	}
	n.Reset()
	v, ok := n.Next()
	if !ok || v != "1" {
		t.Fatalf("expected Reset to rewind to first value, got %q ok=%v", v, ok)
	}
}

func TestNumericRange_ComposesWithChain(t *testing.T) {
	// numeric ranges should work as a Chain base exactly like Wordlist
	n := NewNumericRange(1, 2, 1, 2) // "01", "02"
	c := NewChain(n, PrefixRule{Prefix: "id-"})

	v, _ := c.Next()
	if v != "id-01" {
		t.Fatalf("got %q, want %q", v, "id-01")
	}
}
