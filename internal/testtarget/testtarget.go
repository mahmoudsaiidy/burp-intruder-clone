// Package testtarget is the mock login endpoint used to exercise the
// attack engine end to end without needing an external, authorized target
// on hand for every dev iteration. It's shared by cmd/testserver (a
// standalone process for manual, interactive use) and internal/server's
// demo attack (which starts its own instance on an ephemeral port so
// `go run ./cmd/server` is self-contained, no second process required).
//
// This is NOT part of the Intruder tool itself — it exists purely so the
// rest of the codebase has something safe and local to attack.
package testtarget

import (
	"fmt"
	"net/http"
	"time"
)

// CorrectPassword is the one value LoginHandler's /login endpoint accepts.
const CorrectPassword = "hunter2"

// LoginHandler returns an http.Handler simulating a login endpoint at
// /login?password=... : artificial latency so concurrency/timing
// differences are visible in attack results, 200 for CorrectPassword, 401
// for anything else.
func LoginHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		// a bit of artificial latency so throughput/timing differences are
		// visible in results instead of everything returning in <1ms
		time.Sleep(15 * time.Millisecond)

		if r.URL.Query().Get("password") == CorrectPassword {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "welcome back")
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "invalid credentials")
	})
	return mux
}
