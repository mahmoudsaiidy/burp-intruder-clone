package payload

import "strconv"

// NumericRange is a Generator over an arithmetic sequence of integers —
// Burp's "Numbers" payload type: From, To (inclusive), Step, and an
// optional MinDigits that zero-pads shorter numbers so every value in the
// sequence is the same width (e.g. 001, 002, ... 099, 100).
type NumericRange struct {
	From, To int64
	Step     int64 // must be > 0; From must be <= To
	// MinDigits zero-pads each number's digits out to at least this many
	// characters (sign, if any, is not counted and stays outside the
	// padding). 0 or 1 means no padding beyond the number's natural width.
	MinDigits int

	cur     int64
	started bool
	done    bool
}

// NewNumericRange constructs a NumericRange generator. It's a plain struct
// otherwise (zero value is usable with From/To/Step/MinDigits set
// directly) — this constructor exists for symmetry with NewWordlist and to
// make an invalid range (Step <= 0, or From > To) explicit immediately
// rather than silently producing zero values at Next() time.
func NewNumericRange(from, to, step int64, minDigits int) *NumericRange {
	return &NumericRange{From: from, To: to, Step: step, MinDigits: minDigits}
}

// valid reports whether this range can produce at least one value.
func (n *NumericRange) valid() bool {
	return n.Step > 0 && n.From <= n.To
}

func (n *NumericRange) Next() (string, bool) {
	if !n.valid() || n.done {
		return "", false
	}
	if !n.started {
		n.started = true
		n.cur = n.From
	} else {
		next := n.cur + n.Step
		if next < n.cur { // overflow guard: Step pushed cur past int64 range
			n.done = true
			return "", false
		}
		n.cur = next
	}
	if n.cur > n.To {
		n.done = true
		return "", false
	}
	return formatPadded(n.cur, n.MinDigits), true
}

func (n *NumericRange) Reset() {
	n.started = false
	n.done = false
}

// Len returns how many values this range will produce. It's computed
// directly from From/To/Step rather than by walking the sequence, so it's
// correct even before the first Next() call.
func (n *NumericRange) Len() int {
	if !n.valid() {
		return 0
	}
	span := n.To - n.From
	return int(span/n.Step) + 1
}

// formatPadded renders n as decimal, zero-padding its digits (not its
// sign) out to minDigits characters.
func formatPadded(n int64, minDigits int) string {
	s := strconv.FormatInt(n, 10)
	neg := false
	digits := s
	if len(s) > 0 && s[0] == '-' {
		neg = true
		digits = s[1:]
	}
	for len(digits) < minDigits {
		digits = "0" + digits
	}
	if neg {
		return "-" + digits
	}
	return digits
}
