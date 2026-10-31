package server

import (
	"bytes"
	"context"
	"fmt"

	"intruder/internal/config"
)

// runConfiguredAttack is /ws/config's handler body: read one JSON
// AttackConfig as the client's first text frame, build and run it via
// internal/config.Run (the same entry point cmd/cli's config-file mode
// uses), and stream results the same way runDemoAttack does — reusing its
// wsMessage shape and writeJSON/sendError helpers so a client doesn't need
// to know which endpoint it talked to in order to render the results.
func runConfiguredAttack(ctx context.Context, conn *Conn) {
	op, payload, err := conn.ReadMessage()
	if err != nil {
		sendError(conn, fmt.Errorf("reading config message: %w", err))
		return
	}
	if op == opClose {
		return // client closed before sending a config; nothing to run
	}
	if op != opText {
		sendError(conn, fmt.Errorf("expected a text frame carrying the attack config, got opcode %d", op))
		return
	}

	cfg, err := config.Load(bytes.NewReader(payload))
	if err != nil {
		sendError(conn, fmt.Errorf("parsing config: %w", err))
		return
	}

	// Only now, with the config's first message already consumed, is it
	// safe to hand a background reader the connection — ReadMessage isn't
	// meant to be called concurrently from two goroutines.
	runCtx := withDisconnectDetection(ctx, conn)

	flagged, total, err := cfg.Run(runCtx)
	if err != nil {
		sendError(conn, fmt.Errorf("starting attack: %w", err))
		return
	}
	if !writeJSON(conn, wsMessage{Type: "started", Total: total}) {
		return
	}

	count, hits := 0, 0
	for f := range flagged {
		r := f.Result
		count++
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

	writeJSON(conn, wsMessage{Type: "done", Total: count, Hits: hits})
	conn.WriteClose(1000, "attack complete")
}
