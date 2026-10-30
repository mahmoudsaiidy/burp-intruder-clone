// Command server is the web UI's entry point: an HTTP+WS server wiring
// internal/server's handler (WebSocket endpoint + static frontend) into a
// listening process. Self-contained — the demo attack it serves starts its
// own disposable target, so there's no separate cmd/testserver process to
// run first.
package main

import (
	"flag"
	"log"
	"net/http"

	"intruder/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	webDir := flag.String("web", "web", "directory of static frontend files")
	flag.Parse()

	log.Printf("web UI listening on http://localhost%s (serving %s, demo attack at /ws)", *addr, *webDir)
	log.Fatal(http.ListenAndServe(*addr, server.NewHandler(*webDir)))
}
