package payload

import "testing"

func TestBruteForce_SingleLength(t *testing.T) {
	b := NewBruteForce("ab", 2, 2)
	if b.Len() != 4 { // 2^2
		t.Fatalf("expected Len 4, got %d", b.Len())
	}

	var got []string
	for {
		v, ok := b.Next()
		if !ok {
			break
		}
		got = append(got, v)
	}
	want := []string{"aa", "ab", "ba", "bb"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestBruteForce_LengthRangeGoesShortestFirst(t *testing.T) {
	b := NewBruteForce("ab", 1, 2)
	if b.Len() != 2+4 { // 2^1 + 2^2
		t.Fatalf("expected Len 6, got %d", b.Len())
	}

	var got []string
	for {
		v, ok := b.Next()
		if !ok {
			break
		}
		got = append(got, v)
	}
	want := []string{"a", "b", "aa", "ab", "ba", "bb"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestBruteForce_NoDuplicatesOrSkips(t *testing.T) {
	b := NewBruteForce("xyz", 1, 3)
	seen := make(map[string]bool)
	count := 0
	for {
		v, ok := b.Next()
		if !ok {
			break
		}
		count++
		if seen[v] {
			t.Fatalf("duplicate value produced: %q", v)
		}
		seen[v] = true
	}
	if count != b.Len() {
		t.Fatalf("produced %d values, Len() reported %d", count, b.Len())
	}
	// 3 + 9 + 27 = 39
	if count != 39 {
		t.Fatalf("expected 39 total combinations, got %d", count)
	}
}

func TestBruteForce_InvalidConfigProducesNothing(t *testing.T) {
	empty := NewBruteForce("", 1, 2)
	if empty.Len() != 0 {
		t.Fatalf("expected Len 0 for empty charset, got %d", empty.Len())
	}
	if _, ok := empty.Next(); ok {
		t.Fatalf("expected no values for empty charset")
	}

	badRange := NewBruteForce("ab", 3, 1) // MaxLen < MinLen
	if badRange.Len() != 0 {
		t.Fatalf("expected Len 0 for MaxLen < MinLen, got %d", badRange.Len())
	}
	if _, ok := badRange.Next(); ok {
		t.Fatalf("expected no values for MaxLen < MinLen")
	}

	zeroLen := NewBruteForce("ab", 0, 1) // MinLen < 1
	if zeroLen.Len() != 0 {
		t.Fatalf("expected Len 0 for MinLen < 1, got %d", zeroLen.Len())
	}
	if _, ok := zeroLen.Next(); ok {
		t.Fatalf("expected no values for MinLen < 1")
	}
}

func TestBruteForce_Reset(t *testing.T) {
	b := NewBruteForce("ab", 1, 1)
	for {
		if _, ok := b.Next(); !ok {
			break
		}
	}
	b.Reset()
	v, ok := b.Next()
	if !ok || v != "a" {
		t.Fatalf("expected Reset to rewind to first value, got %q ok=%v", v, ok)
	}
}

func TestBruteForce_ComposesWithChain(t *testing.T) {
	b := NewBruteForce("a", 1, 1) // single value: "a"
	c := NewChain(b, UpperCaseRule{})
	v, ok := c.Next()
	if !ok || v != "A" {
		t.Fatalf("got %q ok=%v, want \"A\" ok=true", v, ok)
	}
}
