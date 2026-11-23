package payload

import "testing"

func collectAll(g Generator) []string {
	var got []string
	for {
		v, ok := g.Next()
		if !ok {
			break
		}
		got = append(got, v)
	}
	return got
}

func TestCharSubstitution_SingleWordSingleSubstitutable(t *testing.T) {
	c := NewCharSubstitution([]string{"a"}, map[rune][]string{'a': {"4", "@"}})
	if c.Len() != 3 {
		t.Fatalf("expected Len 3, got %d", c.Len())
	}
	got := collectAll(c)
	want := []string{"a", "4", "@"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCharSubstitution_OriginalWordIsFirstVariant(t *testing.T) {
	// "pass" has one substitutable position ('a'), with 2 replacement
	// options, so 1+2 = 3 variants: the original, then each replacement.
	c := NewCharSubstitution([]string{"pass"}, map[rune][]string{'a': {"4", "@"}})
	got := collectAll(c)
	want := []string{"pass", "p4ss", "p@ss"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCharSubstitution_Combinatorics(t *testing.T) {
	// "ab" with a->{4}, b->{8} should produce: ab, a8, 4b, 48
	c := NewCharSubstitution([]string{"ab"}, map[rune][]string{'a': {"4"}, 'b': {"8"}})
	got := collectAll(c)
	want := []string{"ab", "a8", "4b", "48"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCharSubstitution_NoSubstitutionsIsPassthrough(t *testing.T) {
	c := NewCharSubstitution([]string{"foo", "bar"}, nil)
	if c.Len() != 2 {
		t.Fatalf("expected Len 2, got %d", c.Len())
	}
	got := collectAll(c)
	want := []string{"foo", "bar"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCharSubstitution_MultipleWords(t *testing.T) {
	c := NewCharSubstitution([]string{"a", "b"}, map[rune][]string{'a': {"4"}})
	got := collectAll(c)
	want := []string{"a", "4", "b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCharSubstitution_EmptyWordListProducesNothing(t *testing.T) {
	c := NewCharSubstitution(nil, map[rune][]string{'a': {"4"}})
	if c.Len() != 0 {
		t.Fatalf("expected Len 0, got %d", c.Len())
	}
	if _, ok := c.Next(); ok {
		t.Fatalf("expected no values")
	}
}

func TestCharSubstitution_Reset(t *testing.T) {
	c := NewCharSubstitution([]string{"ab"}, map[rune][]string{'a': {"4"}})
	for {
		if _, ok := c.Next(); !ok {
			break
		}
	}
	c.Reset()
	v, ok := c.Next()
	if !ok || v != "ab" {
		t.Fatalf("expected Reset to rewind to the first value, got %q ok=%v", v, ok)
	}
}

func TestCharSubstitution_ComposesWithChain(t *testing.T) {
	c := NewCharSubstitution([]string{"a"}, map[rune][]string{'a': {"4"}})
	chain := NewChain(c, UpperCaseRule{})
	v, _ := chain.Next()
	if v != "A" {
		t.Fatalf("got %q, want %q", v, "A")
	}
}
