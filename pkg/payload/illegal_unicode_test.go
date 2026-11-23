package payload

import "testing"

func TestIllegalUnicode_KnownOverlongForms(t *testing.T) {
	// '.' is 0x2E. Its 2-byte overlong form (0xC0 0xAE) is the classic
	// IIS Unicode-bug byte pair used to smuggle "../" past naive filters;
	// verifying it against that well-known literal pins the bit-math down,
	// not just self-consistency.
	u := NewIllegalUnicode([]rune{'.'})
	if u.Len() != 3 {
		t.Fatalf("expected Len 3 (one ASCII char x 3 forms), got %d", u.Len())
	}
	got := collectAll(u)
	want := []string{"%c0%ae", "%e0%80%ae", "%f0%80%80%ae"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestIllegalUnicode_MultipleChars(t *testing.T) {
	u := NewIllegalUnicode([]rune{'.', '/'})
	if u.Len() != 6 {
		t.Fatalf("expected Len 6, got %d", u.Len())
	}
	got := collectAll(u)
	if len(got) != 6 {
		t.Fatalf("got %d values, want 6: %v", len(got), got)
	}
	// First 3 are '.'s forms, next 3 are '/'s forms.
	wantPrefixes := []string{"%c0%ae", "%e0%80%ae", "%f0%80%80%ae", "%c0%af", "%e0%80%af", "%f0%80%80%af"}
	for i := range wantPrefixes {
		if got[i] != wantPrefixes[i] {
			t.Fatalf("got %v, want %v", got, wantPrefixes)
		}
	}
}

func TestIllegalUnicode_NonASCIISkipped(t *testing.T) {
	// 'é' (U+00E9) is outside the ASCII range this generator covers, so it
	// contributes nothing — not an error, consistent with the
	// invalid-input-is-empty convention other generators use.
	u := NewIllegalUnicode([]rune{'.', 'é'})
	if u.Len() != 3 {
		t.Fatalf("expected Len 3 (only '.' counted), got %d", u.Len())
	}
	got := collectAll(u)
	if len(got) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(got), got)
	}
}

func TestIllegalUnicode_AllNonASCIIProducesNothing(t *testing.T) {
	u := NewIllegalUnicode([]rune{'é', 'ñ'})
	if u.Len() != 0 {
		t.Fatalf("expected Len 0, got %d", u.Len())
	}
	if _, ok := u.Next(); ok {
		t.Fatalf("expected no values")
	}
}

func TestIllegalUnicode_Reset(t *testing.T) {
	u := NewIllegalUnicode([]rune{'.', '/'})
	for {
		if _, ok := u.Next(); !ok {
			break
		}
	}
	u.Reset()
	v, ok := u.Next()
	if !ok || v != "%c0%ae" {
		t.Fatalf("expected Reset to rewind to the first value, got %q ok=%v", v, ok)
	}
}

func TestIllegalUnicode_ComposesWithChain(t *testing.T) {
	u := NewIllegalUnicode([]rune{'.'})
	c := NewChain(u, PrefixRule{Prefix: "path="})
	v, _ := c.Next()
	if v != "path=%c0%ae" {
		t.Fatalf("got %q, want %q", v, "path=%c0%ae")
	}
}
