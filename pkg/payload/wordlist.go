package payload

// Wordlist is a Generator over a fixed, in-memory list of strings — a
// loaded dictionary file, or a pasted/typed list, the simplest payload type.
type Wordlist struct {
	Words []string
	pos   int
}

func NewWordlist(words []string) *Wordlist {
	return &Wordlist{Words: words}
}

func (w *Wordlist) Next() (string, bool) {
	if w.pos >= len(w.Words) {
		return "", false
	}
	v := w.Words[w.pos]
	w.pos++
	return v, true
}

func (w *Wordlist) Reset() { w.pos = 0 }

func (w *Wordlist) Len() int { return len(w.Words) }
