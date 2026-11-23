package payload

import "fmt"

// IllegalUnicode is a Generator over illegal, overlong UTF-8 encodings of a
// fixed set of ASCII characters — Burp's "Illegal Unicode" payload type,
// historically used to probe decoders (and the filters sitting in front of
// them, e.g. path-traversal/"../" blockers) that accept one of these
// non-canonical encodings and decode it back to the character it
// overlong-encodes, even though a conformant UTF-8 decoder must reject it.
//
// For each character in Chars, three overlong forms are produced, in this
// order: the 2-byte, 3-byte, and 4-byte overlong encodings of that
// character's code point (the canonical, non-overlong UTF-8 encoding of an
// ASCII character is always 1 byte — these are all, by construction,
// "too many bytes for this code point", which is exactly what "illegal"
// means here). Each is rendered as a %-escaped byte sequence (lower-case
// hex, e.g. "%c0%ae") — the form these values are actually injected in,
// directly into a URL path or query string.
//
// Only ASCII characters (code point 0-127) have a well-defined overlong
// encoding of this kind; a non-ASCII rune in Chars is silently skipped
// (contributes nothing to Len or to the output sequence), the same
// "invalid input produces an empty/smaller generator, not an error or a
// panic" convention NumericRange/BruteForce already use for their own
// invalid configuration.
type IllegalUnicode struct {
	Chars []rune

	charIdx int // index into Chars of the character currently being expanded
	formIdx int // 0, 1, or 2: which of that character's three overlong forms is next
	started bool
	done    bool
}

// NewIllegalUnicode constructs an IllegalUnicode generator over chars.
func NewIllegalUnicode(chars []rune) *IllegalUnicode {
	return &IllegalUnicode{Chars: chars}
}

const illegalUnicodeFormsPerChar = 3

func (u *IllegalUnicode) Next() (string, bool) {
	if u.done {
		return "", false
	}
	if !u.started {
		u.started = true
		u.charIdx = -1
		u.formIdx = illegalUnicodeFormsPerChar - 1 // force advance() to roll into charIdx 0
	}
	for {
		u.formIdx++
		if u.formIdx >= illegalUnicodeFormsPerChar {
			u.formIdx = 0
			u.charIdx++
		}
		if u.charIdx >= len(u.Chars) {
			u.done = true
			return "", false
		}
		c := u.Chars[u.charIdx]
		if c < 0 || c > 0x7F {
			continue // not ASCII: no overlong form defined, skip entirely
		}
		return overlongUTF8(c, u.formIdx), true
	}
}

// overlongUTF8 renders the (form+2)-byte overlong UTF-8 encoding of ASCII
// code point c as a lower-case %-escaped byte sequence: form 0 is 2 bytes,
// form 1 is 3 bytes, form 2 is 4 bytes. Each continuation byte carries 6
// bits of c (10xxxxxx), and the lead byte carries whatever bits remain
// above those, with its own fixed high-bit prefix (110, 1110, or 11110)
// marking the sequence length.
func overlongUTF8(c rune, form int) string {
	switch form {
	case 0: // 2-byte: 110xxxxx 10xxxxxx (11 bits of payload for a 7-bit value)
		b0 := 0xC0 | byte(c>>6)
		b1 := 0x80 | byte(c&0x3F)
		return fmt.Sprintf("%%%02x%%%02x", b0, b1)
	case 1: // 3-byte: 1110xxxx 10xxxxxx 10xxxxxx
		b0 := 0xE0 | byte(c>>12)
		b1 := 0x80 | byte((c>>6)&0x3F)
		b2 := 0x80 | byte(c&0x3F)
		return fmt.Sprintf("%%%02x%%%02x%%%02x", b0, b1, b2)
	default: // 4-byte: 11110xxx 10xxxxxx 10xxxxxx 10xxxxxx
		b0 := 0xF0 | byte(c>>18)
		b1 := 0x80 | byte((c>>12)&0x3F)
		b2 := 0x80 | byte((c>>6)&0x3F)
		b3 := 0x80 | byte(c&0x3F)
		return fmt.Sprintf("%%%02x%%%02x%%%02x%%%02x", b0, b1, b2, b3)
	}
}

func (u *IllegalUnicode) Reset() {
	u.started = false
	u.done = false
	u.charIdx = 0
	u.formIdx = 0
}

// Len returns the total number of overlong encodings this generator will
// produce: illegalUnicodeFormsPerChar for every ASCII character in Chars,
// 0 for every non-ASCII one.
func (u *IllegalUnicode) Len() int {
	n := 0
	for _, c := range u.Chars {
		if c >= 0 && c <= 0x7F {
			n += illegalUnicodeFormsPerChar
		}
	}
	return n
}
