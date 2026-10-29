package match

import (
	"context"

	"intruder/internal/engine"
)

// Flagged pairs one Result with whether it matched the Matcher a Flag call
// was given.
type Flagged struct {
	Result  engine.Result
	Matched bool
}

// Flag consumes a Pool's result stream and tags every Result with whether
// it matched m, without dropping or reordering anything — this is meant to
// sit directly between engine.Pool.Run and whatever consumes results today
// (the CLI's printer now, the web UI's WebSocket hub later), the same way
// a Burp Intruder results table adds a "flag" column rather than hiding
// non-matching rows. The returned channel closes once results is drained
// or ctx is cancelled, matching Pool.Run's own cancellation contract.
func Flag(ctx context.Context, results <-chan engine.Result, m Matcher) <-chan Flagged {
	out := make(chan Flagged)

	go func() {
		defer close(out)
		for {
			select {
			case r, ok := <-results:
				if !ok {
					return
				}
				flagged := Flagged{Result: r, Matched: m.Match(r)}
				select {
				case out <- flagged:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}
