// Command cli is the headless entry point for the attack engine — today it
// runs one hardcoded Sniper attack and prints results to the console, as
// the first end-to-end slice. This is also the proof that core/UI really
// are decoupled: everything it calls lives under internal/ with no
// knowledge of a server or web UI.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"intruder/internal/attack"
	"intruder/internal/engine"
	"intruder/internal/payload"
	"intruder/internal/template"
)

func main() {
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

	fmt.Printf("%-6s  %-15s  %-7s  %-8s  %s\n", "STATUS", "PAYLOAD", "LENGTH", "TIME", "ERROR")
	count := 0
	start := time.Now()
	for r := range results {
		count++
		p := r.Job.Meta["payload"]
		if r.Err != nil {
			fmt.Printf("%-6s  %-15s  %-7s  %-8s  %v\n", "-", p, "-", r.Duration.Round(time.Millisecond), r.Err)
			continue
		}
		fmt.Printf("%-6d  %-15s  %-7d  %-8s\n", r.Status, p, r.Length, r.Duration.Round(time.Millisecond))
	}

	fmt.Printf("\ndone: %d requests in %s\n", count, time.Since(start).Round(time.Millisecond))
}
