// Command testserver is a tiny local HTTP target for exercising the attack
// engine end to end, without needing an external (and authorized) target
// on hand for every dev iteration. It simulates a handful of login-style
// endpoints so every attack mode — and the chain package — has something
// safe and local to run against. This is NOT part of the Intruder tool
// itself.
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

	log.Printf("test target listening on %s", *addr)
	log.Printf("  /login?password=...                         single-field (try %s)", testtarget.CorrectPassword)
	log.Printf("  /login/multi?username=...&password=...       two independent fields (try %s / %s)", testtarget.CorrectUsername, testtarget.CorrectPassword)
	log.Printf("  /csrf                                        issues a CSRF token")
	log.Printf("  /login/secure?csrf=...&username=...&password=...  requires /csrf's token too")
	log.Fatal(http.ListenAndServe(*addr, testtarget.Mux()))
}
