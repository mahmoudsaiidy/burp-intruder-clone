package payload

// Chain wraps a base Generator and applies a sequence of Rules to each
// value it produces, in order. It implements Generator itself, so a
// processed generator composes with every attack mode (Sniper, Battering
// ram, Pitchfork, Cluster bomb) exactly like an unprocessed one — no
// attack-mode-specific handling needed.
//
// Reset and Len simply delegate to Base: processing rules transform each
// value but never add, drop, or reorder them, so the count and rewind
// behavior are entirely the base generator's.
type Chain struct {
	Base  Generator
	Rules []Rule
}

// NewChain returns a Chain applying rules, in order, to every value Base
// produces.
func NewChain(base Generator, rules ...Rule) *Chain {
	return &Chain{Base: base, Rules: rules}
}

func (c *Chain) Next() (string, bool) {
	v, ok := c.Base.Next()
	if !ok {
		return "", false
	}
	for _, r := range c.Rules {
		v = r.Apply(v)
	}
	return v, true
}

func (c *Chain) Reset() { c.Base.Reset() }

func (c *Chain) Len() int { return c.Base.Len() }
