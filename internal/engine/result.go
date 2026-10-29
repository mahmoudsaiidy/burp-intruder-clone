package engine

import (
	"net/http"
	"time"
)

// Job is one unit of work for the pool: a fully-built request plus whatever
// metadata the attack mode wants echoed back on the matching Result (which
// payload(s) produced it, marker index, iteration number — the engine
// itself never looks at Meta, it just carries it through).
type Job struct {
	Request *http.Request
	Meta    map[string]string
}

// Result is what a worker produces after firing one Job. Err is set when
// the request itself failed (timeout, connection refused, etc.) — that's
// distinct from a non-2xx Status, which is a perfectly normal result the
// attack still wants to see and match/filter on.
type Result struct {
	Job      Job
	Status   int
	Length   int64
	Duration time.Duration
	Headers  http.Header
	Body     []byte
	Err      error
}
