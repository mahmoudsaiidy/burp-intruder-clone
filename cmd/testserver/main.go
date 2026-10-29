// Command testserver is a tiny local HTTP target for exercising the attack
// engine end to end, without needing an external (and authorized) target
// on hand for every dev iteration. It simulates a login endpoint with one
// correct password, so a Sniper run against it has a real "hit" to find —
// this is NOT part of the Intruder tool itself.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	addr := flag.String("addr", ":8089", "listen address")
	flag.Parse()

	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		// a bit of artificial latency so throughput/timing differences are
		// visible in results instead of everything returning in <1ms
		time.Sleep(15 * time.Millisecond)

		pass := r.URL.Query().Get("password")
		if pass == "hunter2" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "welcome back")
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "invalid credentials")
	})

	log.Printf("test target listening on %s (try /login?password=hunter2)", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
