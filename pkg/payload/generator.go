// Package payload implements the payload generators attack modes consume:
// wordlists now, with numeric-range, brute-forcer and processing-rule-chain
// generators layered in alongside the other attack modes.
package payload

// Generator produces a sequence of payload strings for one marker position
// (Sniper/Battering ram use one Generator total; Pitchfork/Cluster bomb use
// one per marker).
type Generator interface {
	// Next returns the next payload value. ok is false once the generator
	// is exhausted; the returned value is meaningless when ok is false.
	Next() (value string, ok bool)
	// Reset rewinds back to the first value. Sniper needs this to restart
	// the same generator once per marker position; Cluster bomb needs it to
	// restart inner generators on each outer-generator tick.
	Reset()
	// Len reports the total number of values this generator will produce,
	// when known in advance (every generator we have is finite and
	// deterministic today). Used to size progress bars / total-request
	// counts before a run starts.
	Len() int
}
