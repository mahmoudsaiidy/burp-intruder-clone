package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"intruder/internal/attack"
	"intruder/internal/engine"
	"intruder/internal/match"
	"intruder/internal/payload"
	"intruder/internal/template"
	"intruder/internal/testtarget"
)

// demoWordlist mirrors cmd/cli's hardcoded Sniper demo — same wordlist,
// same single-marker request shape — now streamed over a WebSocket instead
// of printed to a console. This is the first wiring of the engine into the
// web UI; a real "configure your own attack" flow needs internal/config
// (save/load attack configs) and a request-builder UI, neither built yet —
// see STATUS.md.
var demoWordlist = []string{"123456", "password", "admin", "hunter2", "letmein", "qwerty"}

// wsMessage is the JSON shape sent over /ws, one object per line-delimited
// text frame. Type is always set; which other fields are populated depends
// on it:
//   - "result": Hit, Status, Length, DurationMS, Payload, and Error (Error
//     set instead of Status/Length when the request itself failed, e.g. a
//     timeout — same Err-vs-Status distinction engine.Result makes).
//   - "done": Total, Hits — sent once, after the last "result".
//   - "error": Message — sent instead of any "result"/"done" if the attack
//     couldn't even start (e.g. the demo target failed to bind a port).
type wsMessage struct {
	Type       string `json:"type"`
	Hit        bool   `json:"hit,omitempty"`
	Status     int    `json:"status,omitempty"`
	Length     int64  `json:"length,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Payload    string `json:"payload,omitempty"`
	Error      string `json:"error,omitempty"`
	Total      int    `json:"total,omitempty"`
	Hits       int    `json:"hits,omitempty"`
	Message    string `json:"message,omitempty"`
}

// runDemoAttack starts its own disposable instance of testtarget's mock
// login endpoint on an ephemeral localhost port, runs the same Sniper
// attack cmd/cli demos against it, and streams one "result" wsMessage per
// engine.Result over conn as results arrive, finishing with a "done"
// summary and a WebSocket close frame.
//
// It never returns an error to the caller: by the time this runs, the HTTP
// response has already been hijacked into a raw WebSocket connection, so
// there's no ResponseWriter left to report an HTTP-level error on — any
// setup failure becomes an "error" wsMessage instead.
func runDemoAttack(ctx context.Context, conn *Conn) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		sendError(conn, fmt.Errorf("starting demo target: %w", err))
		return
	}
	target := &http.Server{Handler: testtarget.LoginHandler()}
	go target.Serve(ln)
	defer target.Close()

	addr := ln.Addr().(*net.TCPAddr)

	raw := fmt.Sprintf(
		"GET /login?password=§pass§ HTTP/1.1\r\n"+
			"Host: 127.0.0.1:%d\r\n"+
			"User-Agent: intruder-web\r\n"+
			"Accept: */*\r\n"+
			"\r\n",
		addr.Port,
	)
	tmpl, err := template.Parse(raw)
	if err != nil {
		sendError(conn, fmt.Errorf("template parse: %w", err))
		return
	}

	gen := payload.NewWordlist(demoWordlist)
	tgt := template.TargetConfig{Host: "127.0.0.1", Port: addr.Port, UseTLS: false}

	const workers = 5
	client := engine.NewClient(engine.DefaultClientConfig(workers))
	pool := engine.NewPool(client, engine.PoolConfig{Workers: workers, RatePerSecond: 0})

	attackCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cfg := attack.SniperConfig{Template: tmpl, Target: tgt, Generator: gen}
	jobs := attack.Jobs(attackCtx, cfg, workers*2)
	results := pool.Run(attackCtx, jobs)
	flagged := match.Flag(attackCtx, results, match.StatusMatcher{Op: match.Equal, Value: 200})

	total, hits := 0, 0
	for f := range flagged {
		r := f.Result
		total++
		if f.Matched {
			hits++
		}
		msg := wsMessage{
			Type:       "result",
			Hit:        f.Matched,
			DurationMS: r.Duration.Milliseconds(),
			Payload:    r.Job.Meta["payload"],
		}
		if r.Err != nil {
			msg.Error = r.Err.Error()
		} else {
			msg.Status = r.Status
			msg.Length = r.Length
		}
		if !writeJSON(conn, msg) {
			return // client is gone; stop driving the attack forward
		}
	}

	writeJSON(conn, wsMessage{Type: "done", Total: total, Hits: hits})
	conn.WriteClose(1000, "attack complete")
}

func sendError(conn *Conn, err error) {
	writeJSON(conn, wsMessage{Type: "error", Message: err.Error()})
	conn.WriteClose(1011, "internal error")
}

// writeJSON marshals and sends msg, reporting success so callers in a
// streaming loop can stop early once the client is gone instead of
// treating every subsequent write as a fatal error.
func writeJSON(conn *Conn, msg wsMessage) bool {
	b, err := json.Marshal(msg)
	if err != nil {
		return false
	}
	return conn.WriteText(b) == nil
}
