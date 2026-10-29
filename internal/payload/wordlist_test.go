package payload

import "testing"

func TestWordlist(t *testing.T) {
	w := NewWordlist([]string{"a", "b", "c"})
	if w.Len() != 3 {
		t.Fatalf("expected len 3, got %d", w.Len())
	}

	var got []string
	for {
		v, ok := w.Next()
		if !ok {
			break
		}
		got = append(got, v)
	}
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("unexpected sequence: %v", got)
	}

	w.Reset()
	v, ok := w.Next()
	if !ok || v != "a" {
		t.Fatalf("expected reset to rewind to first value, got %q ok=%v", v, ok)
	}
}
