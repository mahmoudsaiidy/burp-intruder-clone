// Command server is the web UI's entry point: an HTTP+WS server wiring
// internal/server's handler (WebSocket endpoints, the saved-config
// gallery's REST API, and the static frontend) into a listening process.
// Self-contained — the demo attack it serves starts its own disposable
// target, so there's no separate cmd/testserver process to run first.
package main

import (
	"flag"
	"log"
	"net/http"

	"intruder/internal/configstore"
	"intruder/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	webDir := flag.String("web", "web", "directory of static frontend files")
	configsDir := flag.String("configs", "configs", "directory for builder.html's saved-config gallery (created if missing); pass an empty string to disable the gallery")
	flag.Parse()

	var store *configstore.Store
	if *configsDir != "" {
		var err error
		store, err = configstore.New(*configsDir)
		if err != nil {
			log.Fatalf("saved-config gallery: %v", err)
		}
	}

	log.Printf("web UI listening on http://localhost%s (serving %s, demo attack at /ws)", *addr, *webDir)
	if store != nil {
		log.Printf("saved-config gallery: %s", *configsDir)
	} else {
		log.Printf("saved-config gallery disabled (-configs=\"\")")
	}
	log.Fatal(http.ListenAndServe(*addr, server.NewHandler(*webDir, store)))
}
