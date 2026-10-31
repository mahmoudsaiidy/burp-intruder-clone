// Package server is the web UI's backend: an HTTP+WS server consuming the
// engine's result stream instead of the CLI's console printer.
//
// "/ws" still runs the original hardcoded demo attack (demo.go). "/ws/config"
// is the real "submit your own request template and payload list" flow:
// the client sends one JSON internal/config.AttackConfig as its first text
// frame, and the server builds and runs it the same way cmd/cli's
// config-file mode does — both share internal/config.Run rather than each
// hand-wiring the engine. A request-builder page in web/ that composes
// that JSON from form inputs is still future work (see STATUS.md); today a
// client has to send the JSON itself.
package server

import (
	"context"
	"net/http"
)

// NewHandler returns the web UI's top-level http.Handler:
//   - "/ws" upgrades to a WebSocket and runs the hardcoded demo attack
//     (demo.go), streaming results as they arrive.
//   - "/ws/config" upgrades to a WebSocket, reads one JSON AttackConfig as
//     the client's first message, and runs that attack (config_attack.go).
//   - "/" (and everything else) serves static files from webDir, when
//     webDir is non-empty. Tests that only exercise "/ws"/"/ws/config" pass
//     "".
func NewHandler(webDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWS)
	mux.HandleFunc("/ws/config", handleWSConfig)
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

func handleWSConfig(w http.ResponseWriter, r *http.Request) {
	conn, err := Upgrade(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer conn.Close()

	runConfiguredAttack(r.Context(), conn)
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
