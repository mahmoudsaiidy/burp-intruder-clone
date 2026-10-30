// Package server is the web UI's backend: an HTTP+WS server consuming the
// engine's result stream instead of the CLI's console printer.
//
// Only the WebSocket endpoint and a hardcoded demo attack exist so far
// (see demo.go) — there's no "submit your own request template and payload
// list" flow yet, since that needs internal/config (save/load attack
// configs, not built) and a request-builder UI. This is the first slice:
// prove the engine's result/flag stream can drive a live web page the same
// way it already drives cmd/cli's console output.
package server

import (
	"context"
	"net/http"
)

// NewHandler returns the web UI's top-level http.Handler:
//   - "/ws" upgrades to a WebSocket and runs the demo attack (demo.go),
//     streaming results as they arrive.
//   - "/" (and everything else) serves static files from webDir, when
//     webDir is non-empty. Tests that only exercise "/ws" pass "".
func NewHandler(webDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWS)
	if webDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(webDir)))
	}
	return mux
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := Upgrade(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer conn.Close()

	runDemoAttack(withDisconnectDetection(r.Context(), conn), conn)
}

// withDisconnectDetection returns a context cancelled as soon as the
// client sends a close frame or the connection otherwise errors.
// runDemoAttack's own goroutine is busy writing results to conn, so
// reading in the background is the only way to notice the client going
// away mid-attack.
func withDisconnectDetection(parent context.Context, conn *Conn) context.Context {
	ctx, cancel := context.WithCancel(parent)
	go func() {
		for {
			op, _, err := conn.ReadMessage()
			if err != nil || op == opClose {
				cancel()
				return
			}
		}
	}()
	return ctx
}
