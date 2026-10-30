// Command testserver is a tiny local HTTP target for exercising the attack
// engine end to end, without needing an external (and authorized) target
// on hand for every dev iteration. It simulates a login endpoint with one
// correct password, so a Sniper run against it has a real "hit" to find —
// this is NOT part of the Intruder tool itself.
package main

import (
	"flag"
	"log"
	"net/http"

	"intruder/internal/testtarget"
)

func main() {
	addr := flag.String("addr", ":8089", "listen address")
	flag.Parse()

	log.Printf("test target listening on %s (try /login?password=%s)", *addr, testtarget.CorrectPassword)
	log.Fatal(http.ListenAndServe(*addr, testtarget.LoginHandler()))
}
