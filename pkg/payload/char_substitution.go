package payload

// CharSubstitution is a Generator over every character-substitution variant
// of a fixed word list — Burp's "Character substitution" payload type: for
// each word, and each character position that has one or more configured
// replacements (e.g. 'a' -> "4", "@"), produce every combination of using
// the original character or one of its replacements at that position,
// leaving positions with no configured replacement untouched. The original
// word itself is always the first variant produced for it (every position
// defaulting to "use the original character").
//
// Substitutions map a rune to the ordered replacement strings Burp's own
// UI configures per character (each entry can be more than one character,
// e.g. 'a' -> "4", "@", "/\\"). Matching is exact (case-sensitive) — pass
// both cases as separate keys if a substitution should apply regardless of
// case.
type CharSubstitution struct {
	Words         []string
	Substitutions map[rune][]string

	wordIdx int
	choices [][]string // this word's per-position option lists, built lazily
	idx     []int      // current combination: one choice-index per position
	started bool
	done    bool
}

// NewCharSubstitution constructs a CharSubstitution generator over words,
// applying substitutions.
func NewCharSubstitution(words []string, substitutions map[rune][]string) *CharSubstitution {
	return &CharSubstitution{Words: words, Substitutions: substitutions}
}

// choicesFor returns, for each rune of word, the ordered list of strings
// that may appear at that position: the original character first, then any
// configured replacements for it.
func (c *CharSubstitution) choicesFor(word string) [][]string {
	runes := []rune(word)
	choices := make([][]string, len(runes))
	for i, r := range runes {
		opts := make([]string, 0, 1+len(c.Substitutions[r]))
		opts = append(opts, string(r))
		opts = append(opts, c.Substitutions[r]...)
		choices[i] = opts
	}
	return choices
}

func (c *CharSubstitution) Next() (string, bool) {
	if len(c.Words) == 0 || c.done {
		return "", false
	}

	if !c.started {
		c.started = true
		if !c.loadWord(0) {
			c.done = true
			return "", false
		}
	} else if !c.advance() {
		c.done = true
		return "", false
	}

	return c.current(), true
}

// loadWord sets up choices/idx for Words[wordIdx], skipping any empty word
// (which has zero positions and therefore, degenerately, exactly one
// variant: the empty string itself — still emitted once, same as a plain
// Wordlist would emit an empty entry).
func (c *CharSubstitution) loadWord(wordIdx int) bool {
	if wordIdx >= len(c.Words) {
		return false
	}
	c.wordIdx = wordIdx
	c.choices = c.choicesFor(c.Words[wordIdx])
	c.idx = make([]int, len(c.choices))
	return true
}

// advance moves to the next combination for the current word, odometer-
// style (last position varies fastest, same convention BruteForce.advance
// uses). When the current word's combinations are exhausted it moves to
// the next word. It returns false once every word's every combination has
// been produced.
func (c *CharSubstitution) advance() bool {
	for i := len(c.idx) - 1; i >= 0; i-- {
		c.idx[i]++
		if c.idx[i] < len(c.choices[i]) {
			return true
		}
		c.idx[i] = 0
		// carry into the next-more-significant position; if this was the
		// first position, every combination for this word has been produced
	}
	return c.loadWord(c.wordIdx + 1)
}

func (c *CharSubstitution) current() string {
	out := make([]byte, 0, len(c.idx))
	for i, choiceIdx := range c.idx {
		out = append(out, c.choices[i][choiceIdx]...)
	}
	return string(out)
}

func (c *CharSubstitution) Reset() {
	c.started = false
	c.done = false
	c.wordIdx = 0
	c.choices = nil
	c.idx = nil
}

// Len returns the total number of variants this generator will produce:
// for each word, the product of each position's option count (1 for a word
// with no substitutable characters, i.e. just the word itself), summed
// across every word.
func (c *CharSubstitution) Len() int {
	total := 0
	for _, word := range c.Words {
		variants := 1
		for _, r := range word {
			variants *= 1 + len(c.Substitutions[r])
		}
		total += variants
	}
	return total
}
