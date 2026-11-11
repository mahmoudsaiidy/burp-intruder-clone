package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"intruder/internal/configstore"
)

// maxSavedConfigBytes caps how large a single saved AttackConfig document
// the gallery's PUT endpoint will read, generously (configs are request
// templates, wordlists, and a handful of rules/stages — nowhere near this
// big in practice) but boundedly, so a client can't make the server buffer
// an unbounded request body.
const maxSavedConfigBytes = 1 << 20 // 1 MiB

// registerConfigStoreRoutes wires the saved-config gallery's REST API onto
// mux: list every saved name, fetch one by name, save (create or
// overwrite) one, and delete one. builder.html is the one client today,
// but this is a plain JSON API, not WebSocket-framed, since it's simple
// request/response CRUD rather than a streamed result feed like
// "/ws"/"/ws/config".
//
// store may be nil — e.g. in tests that don't exercise the gallery, or if
// cmd/server is ever run with saved configs deliberately disabled — in
// which case these routes aren't registered at all, and requests to them
// fall through to NewHandler's normal "/" handling (a 404 from the static
// file server, or no handler at all when webDir is also "").
func registerConfigStoreRoutes(mux *http.ServeMux, store *configstore.Store) {
	if store == nil {
		return
	}

	mux.HandleFunc("GET /api/configs", func(w http.ResponseWriter, r *http.Request) {
		names, err := store.List()
		if err != nil {
			storeError(w, err)
			return
		}
		writeJSONBody(w, http.StatusOK, names)
	})

	mux.HandleFunc("GET /api/configs/{name}", func(w http.ResponseWriter, r *http.Request) {
		data, err := store.Load(r.PathValue("name"))
		if err != nil {
			storeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	mux.HandleFunc("PUT /api/configs/{name}", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(io.LimitReader(r.Body, maxSavedConfigBytes+1))
		if err != nil {
			http.Error(w, "reading request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(data) > maxSavedConfigBytes {
			http.Error(w, "config document too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err := store.Save(r.PathValue("name"), data); err != nil {
			storeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/configs/{name}", func(w http.ResponseWriter, r *http.Request) {
		if err := store.Delete(r.PathValue("name")); err != nil {
			storeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// storeError maps a configstore error to the right HTTP status: a missing
// name is a 404; an invalid name or an unparsable config document (Save's
// own config.Load validation, ErrInvalidConfig) is a 400 — the client sent
// something wrong, not the server failing; anything else (disk I/O, say)
// is a 500, logged server-side since its text isn't something the client
// can act on.
func storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, configstore.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, configstore.ErrInvalidName), errors.Is(err, configstore.ErrInvalidConfig):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		log.Printf("configstore: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func writeJSONBody(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("configstore: encoding response: %v", err)
	}
}
