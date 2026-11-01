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

// CorrectPassword is the one value LoginFunc's /login endpoint accepts,
// and one of the two values MultiFieldLoginFunc and SecureLoginFunc
// require.
const CorrectPassword = "hunter2"

// CorrectUsername is the username MultiFieldLoginFunc and SecureLoginFunc
// require alongside CorrectPassword.
const CorrectUsername = "admin"

// CSRFToken is the token CSRFFunc issues and SecureLoginFunc requires. A
// real target would mint a fresh, unpredictable token per session; this one
// is fixed so attack configs and tests can assert against it directly.
const CSRFToken = "deadbeef"

// loginLatency is added to every handler in this package so
// concurrency/timing differences are visible in attack results instead of
// everything returning in <1ms.
const loginLatency = 15 * time.Millisecond

// LoginFunc is the original, single-field login check: 200 for
// CorrectPassword, 401 for anything else. It is also what LoginHandler
// mounts at /login.
//
// Kept byte-for-byte unchanged so internal/server's hardcoded demo attack
// and cmd/cli's no-argument demo — both of which fire requests at this
// exact endpoint and check for an exact 6-requests/1-hit result — keep
// behaving exactly as they always have.
func LoginFunc(w http.ResponseWriter, r *http.Request) {
	time.Sleep(loginLatency)

	if r.URL.Query().Get("password") == CorrectPassword {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "welcome back")
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintln(w, "invalid credentials")
}

// LoginHandler returns an http.Handler simulating a login endpoint at
// /login?password=... : artificial latency so concurrency/timing
// differences are visible in attack results, 200 for CorrectPassword, 401
// for anything else.
func LoginHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", LoginFunc)
	return mux
}

// MultiFieldLoginFunc checks username and password independently (200 only
// if both are correct, 401 otherwise) — a target with two genuinely
// independent markers, for exercising Pitchfork (lockstep) and Cluster
// bomb (full combinatorial product) against a real server instead of only
// httptest. See STATUS.md's "Next step" for the motivation.
func MultiFieldLoginFunc(w http.ResponseWriter, r *http.Request) {
	time.Sleep(loginLatency)

	q := r.URL.Query()
	if q.Get("username") == CorrectUsername && q.Get("password") == CorrectPassword {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "welcome back")
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintln(w, "invalid credentials")
}

// CSRFFunc simulates a CSRF-token-issuing endpoint, returning "token=<CSRFToken>"
// in the body — the exact shape internal/chain's and internal/config's own
// tests already pull out with a `token=(\w+)` body-regex extractor, so an
// example chain config can extract it the same way.
func CSRFFunc(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "token=%s", CSRFToken)
}

// SecureLoginFunc is like MultiFieldLoginFunc but additionally requires a
// valid CSRF token (from CSRFFunc) — a login endpoint that genuinely can't
// be attacked in one request, for exercising internal/chain +
// internal/config's chain support (fetch the token first, then feed it
// into the attack) against a real, standalone target rather than only
// httptest-backed unit tests.
func SecureLoginFunc(w http.ResponseWriter, r *http.Request) {
	time.Sleep(loginLatency)

	q := r.URL.Query()
	if q.Get("csrf") == CSRFToken && q.Get("username") == CorrectUsername && q.Get("password") == CorrectPassword {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "welcome back")
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintln(w, "invalid credentials")
}

// Mux returns an http.Handler exposing every testtarget endpoint on one
// mux, for cmd/testserver's standalone process:
//
//	/login         LoginFunc          — single field (password only); the
//	                                     original demo endpoint, unchanged.
//	/login/multi   MultiFieldLoginFunc — username+password, independent
//	                                     fields, for Pitchfork/Cluster bomb.
//	/csrf          CSRFFunc            — issues a CSRF token.
//	/login/secure  SecureLoginFunc     — csrf+username+password; requires a
//	                                     token from /csrf first, for chain
//	                                     demos.
//
// internal/server's demo attack intentionally keeps using LoginHandler
// (just /login) rather than Mux, so its hardcoded demo's behavior doesn't
// change.
func Mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", LoginFunc)
	mux.HandleFunc("/login/multi", MultiFieldLoginFunc)
	mux.HandleFunc("/csrf", CSRFFunc)
	mux.HandleFunc("/login/secure", SecureLoginFunc)
	return mux
}
