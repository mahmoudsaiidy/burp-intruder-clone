package payload

import "time"

// DateRange is a Generator over an evenly-spaced sequence of dates/times —
// Burp's "Dates" payload type: a From/To range (inclusive of From, and of
// To only if the step lands exactly on it, same "arithmetic sequence"
// semantics NumericRange already has for integers), a Step duration, and a
// Layout (a Go reference-time layout, e.g. "2006-01-02") controlling how
// each value is formatted.
type DateRange struct {
	From, To time.Time
	Step     time.Duration // must be > 0; From must not be after To
	Layout   string        // a time.Format layout; "" defaults to time.RFC3339

	cur     time.Time
	started bool
	done    bool
}

// NewDateRange constructs a DateRange generator. It's a plain struct
// otherwise (zero value is usable with the fields set directly) — this
// constructor exists for symmetry with NewNumericRange/NewBruteForce.
func NewDateRange(from, to time.Time, step time.Duration, layout string) *DateRange {
	return &DateRange{From: from, To: to, Step: step, Layout: layout}
}

func (d *DateRange) layout() string {
	if d.Layout == "" {
		return time.RFC3339
	}
	return d.Layout
}

// valid reports whether this range can produce at least one value — same
// "Step <= 0 or From after To is an empty generator, not an error or a
// panic" convention NumericRange/BruteForce already use.
func (d *DateRange) valid() bool {
	return d.Step > 0 && !d.From.After(d.To)
}

func (d *DateRange) Next() (string, bool) {
	if !d.valid() || d.done {
		return "", false
	}
	if !d.started {
		d.started = true
		d.cur = d.From
	} else {
		next := d.cur.Add(d.Step)
		if !next.After(d.cur) { // guard against a Step too small to advance at all
			d.done = true
			return "", false
		}
		d.cur = next
	}
	if d.cur.After(d.To) {
		d.done = true
		return "", false
	}
	return d.cur.Format(d.layout()), true
}

func (d *DateRange) Reset() {
	d.started = false
	d.done = false
}

// Len returns how many values this range will produce. It's computed
// directly from From/To/Step rather than by walking the sequence, so it's
// correct even before the first Next() call — same approach
// NumericRange.Len uses.
func (d *DateRange) Len() int {
	if !d.valid() {
		return 0
	}
	span := d.To.Sub(d.From)
	return int(span/d.Step) + 1
}
