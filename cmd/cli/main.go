// Command cli is the headless entry point for the attack engine. With no
// arguments it runs the original hardcoded Sniper demo against
// cmd/testserver, same as always. Given a path to a JSON attack config
// (see internal/config), it loads and runs that instead — the CLI side of
// "real attack configuration" from STATUS.md's next step, sharing
// internal/config.Run with the web UI's /ws/config endpoint rather than
// each hand-wiring the engine separately.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"intruder/internal/attack"
	"intruder/internal/config"
	"intruder/internal/engine"
	"intruder/internal/match"
	"intruder/internal/payload"
	"intruder/internal/template"
)

func main() {
	if len(os.Args) > 1 {
		runFromConfigFile(os.Args[1])
		return
	}
	runDemo()
}

// runFromConfigFile loads an AttackConfig from path and runs it via
// internal/config.Run — the same code path the web UI's /ws/config
// endpoint uses, so a config that works from one works from the other.
func runFromConfigFile(path string) {
	cfg, err := config.LoadFile(path)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	ctx := context.Background()
	flagged, total, err := cfg.Run(ctx)
	if err != nil {
		log.Fatalf("starting attack: %v", err)
	}

	fmt.Printf("%s: %d request(s)\n\n", cfg.Mode, total)
	count, hits, start := printResults(flagged)
	fmt.Printf("\ndone: %d requests (%d flagged) in %s\n", count, hits, time.Since(start).Round(time.Millisecond))
}

// runDemo is the original hardcoded Sniper smoke test: wires
// template+payload+attack+engine together by hand and runs it against
// cmd/testserver. Kept as-is (rather than rewritten as a config.AttackConfig
// internally) so running the CLI with no arguments keeps behaving exactly
// as it always has.
func runDemo() {
	raw := "GET /login?password=§pass§ HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8089\r\n" +
		"User-Agent: intruder-cli\r\n" +
		"Accept: */*\r\n" +
		"\r\n"

	tmpl, err := template.Parse(raw)
	if err != nil {
		log.Fatalf("template parse: %v", err)
	}
	if len(tmpl.Markers) != 1 {
		log.Fatalf("expected 1 marker, found %d", len(tmpl.Markers))
	}

	words := []string{"123456", "password", "admin", "hunter2", "letmein", "qwerty"}
	gen := payload.NewWordlist(words)

	target := template.TargetConfig{Host: "127.0.0.1", Port: 8089, UseTLS: false}

	const workers = 5
	client := engine.NewClient(engine.DefaultClientConfig(workers))
	pool := engine.NewPool(client, engine.PoolConfig{
		Workers:       workers,
		RatePerSecond: 0, // uncapped for this first smoke test
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := attack.SniperConfig{Template: tmpl, Target: target, Generator: gen}
	fmt.Printf("sniper: %d marker(s) x %d payload(s) = %d requests\n\n", len(tmpl.Markers), gen.Len(), cfg.TotalJobs())

	jobs := attack.Jobs(ctx, cfg, workers*2)
	results := pool.Run(ctx, jobs)

	// a 200 is the "interesting" response for this login endpoint — a real
	// run would set this from flags/config, but the point here is just to
	// exercise match.Flag end to end: it tags every row without dropping
	// or reordering any of them, same as a Burp Intruder results table
	// adding a flag column rather than filtering rows out.
	flagged := match.Flag(ctx, results, match.StatusMatcher{Op: match.Equal, Value: 200})

	count, hits, start := printResults(flagged)
	fmt.Printf("\ndone: %d requests (%d flagged) in %s\n", count, hits, time.Since(start).Round(time.Millisecond))
}

// printResults prints the shared results table both runDemo and
// runFromConfigFile use, returning the total/flagged counts and the time
// printing started so the caller can report a final duration.
func printResults(flagged <-chan match.Flagged) (count, hits int, start time.Time) {
	fmt.Printf("%-5s  %-6s  %-15s  %-7s  %-8s  %s\n", "HIT", "STATUS", "PAYLOAD", "LENGTH", "TIME", "ERROR")
	start = time.Now()
	for f := range flagged {
		r := f.Result
		count++
		p := r.Job.Meta["payload"]
		if r.Err != nil {
			fmt.Printf("%-5s  %-6s  %-15s  %-7s  %-8s  %v\n", "", "-", p, "-", r.Duration.Round(time.Millisecond), r.Err)
			continue
		}
		hit := ""
		if f.Matched {
			hit = "***"
			hits++
		}
		fmt.Printf("%-5s  %-6d  %-15s  %-7d  %-8s\n", hit, r.Status, p, r.Length, r.Duration.Round(time.Millisecond))
	}
	return count, hits, start
}
