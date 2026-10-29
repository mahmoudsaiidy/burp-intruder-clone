# Project status

> This file is the resume point for picking this project back up in a new
> session. Read this first, then continue from "Next step".

## What we're building

A desktop/local tool that replicates the behavior of Burp Suite's Intruder
module, for authorized security testing. Not a reverse-engineered clone of
Burp's binary — a from-scratch Go implementation that matches its attack
modes, payload handling, concurrency model, and results workflow.

**Stack decisions made so far:**
- Core engine: Go, goroutine worker pool, tuned `net/http` Transport for
  connection pooling/keep-alive
- Interface: local web UI served over WebSockets (not a Wails desktop GUI)
  — chosen for zero native-toolchain dependency and clean cross-platform
  behavior, and because it keeps the core engine UI-agnostic for an
  eventual headless CLI
- Target OS: cross-platform (Linux/macOS/Windows) — no CGO, no OS-specific
  paths in the core packages

## Architecture (agreed, see original design discussion for full rationale)

```
cmd/server/     entry point: HTTP+WS server, wires everything (NOT BUILT YET)
cmd/cli/        headless entry point — exists now, runs one hardcoded Sniper attack
cmd/testserver/ throwaway local HTTP target for dev testing only, not part of the tool
internal/
  template/     raw HTTP request parsing, §marker§ detection/injection, Build() -> *http.Request
  payload/      Generator interface; Wordlist implemented; numeric/brute-force/processing NOT YET
  attack/       attack-mode orchestration; Sniper implemented; Battering ram/Pitchfork/Cluster bomb NOT YET
  engine/       worker pool, shared http.Client, rate limiter+jitter, Result streaming — DONE
  config/       save/load attack configs — NOT BUILT YET
  server/       web UI backend (HTTP+WS) — NOT BUILT YET
web/            static frontend — NOT BUILT YET
```

Concurrency model: payload generator (writer goroutine) → bounded job
channel → N worker goroutines (shared `*http.Client` for connection reuse)
→ bounded result channel → consumer (currently: CLI printer; later: WS hub).
One `context.Context` per attack run for clean cancellation. Full design
rationale was discussed and agreed before any code was written — see git
log / chat history if the "why" behind any of this needs revisiting.

## Done

- [x] `internal/template` — parser, multi-marker §...§ detection/injection,
      `Build()` resolves a payload set into a client-side `*http.Request`.
      Tests pass.
- [x] `internal/payload` — `Generator` interface + `Wordlist`. Tests pass.
- [x] `internal/engine` — `NewClient` (tuned, pooled Transport),
      `Pool.Run` (worker pool, context cancellation, rate limiter + jitter,
      body-size cap), `Result`/`Job` types. Tests pass under `-race`.
- [x] `internal/attack` — Sniper mode (`Jobs()`): one marker live at a
      time, others held at original value. Verified against a live
      `httptest` server (exact query combinations checked). Tests pass
      under `-race`.
- [x] `cmd/testserver` — mock login endpoint (`/login?password=...`,
      correct password is `hunter2`) for exercising the engine end to end
      without needing an external authorized target during dev.
- [x] `cmd/cli` — wires template+payload+attack+engine together, runs the
      Sniper attack against `cmd/testserver`, prints a results table.
      **Verified working end to end**: 6 requests (1 marker × 6 payloads)
      completed in ~33ms on 5 workers with 15ms artificial per-request
      latency (proves real concurrency, not serial), and correctly flagged
      `hunter2` → `200` vs. everything else → `401`.

Full test suite passes: `go test -race ./...`

**Known environment quirk (dev sandbox only, shouldn't apply on your
machine):** `localhost` didn't resolve in this sandbox; had to use
`127.0.0.1` in the CLI's hardcoded template/target. Not a code issue.

## Next step

Layer in the remaining attack modes, following the exact same
`Jobs(ctx, cfg, buffer) <-chan engine.Job` shape `internal/attack/sniper.go`
already establishes:

1. **Battering ram** — same payload value inserted into *all* markers
   simultaneously (one generator, one value per iteration, broadcast to
   every marker index).
2. **Pitchfork** — multiple payload sets (one `Generator` per marker)
   iterated in parallel/lockstep; stops at the shortest generator (Burp's
   behavior) — confirm this against Burp docs/behavior when we get there.
3. **Cluster bomb** — multiple payload sets, full combinatorial product
   across all markers (nested iteration with `Reset()` on inner
   generators, same pattern Sniper already uses for its outer loop).

After that, in the order originally planned: payload processing rule
chains (URL-encode/base64/prefix-suffix/case), numeric-range and
brute-force generators, match/extract rules on the engine's `Result`, then
the web UI (`internal/server` + `web/`) consuming the result stream over
WebSockets instead of the CLI's console printer.

## How to resume this project in a new session

1. Clone this repo.
2. Tell Claude: "Continue the Burp Intruder clone project — read STATUS.md
   and pick up from Next step."
3. Point Claude at this file; it has everything needed to continue without
   re-deriving the architecture from scratch.
