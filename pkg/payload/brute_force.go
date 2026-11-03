package payload

// BruteForce is a Generator over every string of length MinLen..MaxLen
// (inclusive) drawable from a fixed character set — Burp's "Brute forcer"
// payload type. It produces all MinLen-length combinations first, in
// lexicographic order over Charset, then all MinLen+1-length combinations,
// and so on up to MaxLen.
type BruteForce struct {
	// Charset is the set of characters each position may take. Duplicate
	// runes are not deduplicated (a repeated rune just makes that value
	// more likely to recur at a given position, which is harmless but
	// also pointless) — callers should pass distinct runes.
	Charset []rune
	MinLen  int // must be >= 1
	MaxLen  int // must be >= MinLen

	curLen  int
	idx     []int // current combination, one charset index per position
	started bool
	done    bool
}

// NewBruteForce constructs a BruteForce generator over charset, producing
// every combination of length minLen..maxLen.
func NewBruteForce(charset string, minLen, maxLen int) *BruteForce {
	return &BruteForce{Charset: []rune(charset), MinLen: minLen, MaxLen: maxLen}
}

func (b *BruteForce) valid() bool {
	return len(b.Charset) > 0 && b.MinLen >= 1 && b.MaxLen >= b.MinLen
}

func (b *BruteForce) Next() (string, bool) {
	if !b.valid() || b.done {
		return "", false
	}

	if !b.started {
		b.started = true
		b.curLen = b.MinLen
		b.idx = make([]int, b.curLen)
	} else if !b.advance() {
		b.done = true
		return "", false
	}

	return b.current(), true
}

// advance moves to the next combination, odometer-style (last position
// varies fastest). When the current length's combinations are exhausted it
// moves to the next length, starting that length's indices back at all
// zeros. It returns false once MaxLen's combinations are exhausted too.
func (b *BruteForce) advance() bool {
	base := len(b.Charset)
	for i := len(b.idx) - 1; i >= 0; i-- {
		b.idx[i]++
		if b.idx[i] < base {
			return true
		}
		b.idx[i] = 0
		// carry into the next-more-significant position; if this was the
		// first position, every combination of curLen has been produced
	}

	// every combination of b.curLen is exhausted: move to the next length
	b.curLen++
	if b.curLen > b.MaxLen {
		return false
	}
	b.idx = make([]int, b.curLen)
	return true
}

func (b *BruteForce) current() string {
	runes := make([]rune, len(b.idx))
	for i, charIdx := range b.idx {
		runes[i] = b.Charset[charIdx]
	}
	return string(runes)
}

func (b *BruteForce) Reset() {
	b.started = false
	b.done = false
	b.curLen = 0
	b.idx = nil
}

// Len returns the total number of combinations this generator will
// produce: sum of len(Charset)^L for L from MinLen to MaxLen.
func (b *BruteForce) Len() int {
	if !b.valid() {
		return 0
	}
	base := len(b.Charset)
	total := 0
	pow := 1
	for l := 1; l <= b.MaxLen; l++ {
		pow *= base
		if l >= b.MinLen {
			total += pow
		}
	}
	return total
}
