package payload

import "testing"

func TestChain_AppliesRulesInOrder(t *testing.T) {
	base := NewWordlist([]string{"admin"})

	// Prefix then UpperCase: the prefix itself should also get uppercased,
	// proving rules see the previous rule's output, not the raw base value.
	c := NewChain(base, PrefixRule{Prefix: "pre-"}, UpperCaseRule{})

	v, ok := c.Next()
	if !ok {
		t.Fatalf("expected a value")
	}
	want := "PRE-ADMIN"
	if v != want {
		t.Fatalf("got %q, want %q", v, want)
	}

	_, ok = c.Next()
	if ok {
		t.Fatalf("expected generator to be exhausted after one value")
	}
}

func TestChain_OrderMatters(t *testing.T) {
	base := NewWordlist([]string{"x"})

	// Base64 then Prefix: the prefix is appended AFTER encoding, so it's
	// never itself base64'd and stays readable in the output.
	c := NewChain(base, Base64EncodeRule{}, PrefixRule{Prefix: "b64:"})
	v, _ := c.Next()
	want := "b64:" + Base64EncodeRule{}.Apply("x")
	if v != want {
		t.Fatalf("got %q, want %q", v, want)
	}
}

func TestChain_NoRulesPassesValuesThrough(t *testing.T) {
	base := NewWordlist([]string{"a", "b"})
	c := NewChain(base)

	var got []string
	for {
		v, ok := c.Next()
		if !ok {
			break
		}
		got = append(got, v)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected sequence: %v", got)
	}
}

func TestChain_LenAndResetDelegateToBase(t *testing.T) {
	base := NewWordlist([]string{"a", "b", "c"})
	c := NewChain(base, UpperCaseRule{})

	if c.Len() != 3 {
		t.Fatalf("expected Len 3, got %d", c.Len())
	}

	// drain it
	for {
		if _, ok := c.Next(); !ok {
			break
		}
	}
	if _, ok := c.Next(); ok {
		t.Fatalf("expected exhausted generator")
	}

	c.Reset()
	v, ok := c.Next()
	if !ok || v != "A" {
		t.Fatalf("expected Reset to rewind to first (processed) value, got %q ok=%v", v, ok)
	}
}
