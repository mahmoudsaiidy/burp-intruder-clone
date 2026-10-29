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
  payload/      Generator interface; Wordlist + processing-rule Chain done; numeric/brute-force NOT YET
  attack/       attack-mode orchestration; Sniper, Battering ram, Pitchfork, Cluster bomb all DONE
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

- [x] `internal/attack` — all four attack modes implemented, each its own
      file following the `Jobs(ctx, cfg, buffer) <-chan engine.Job` shape
      `sniper.go` established:
      - `battering_ram.go` — one generator, same value broadcast into
        every marker on each iteration. `BatteringRamJobs` /
        `BatteringRamConfig`. `TotalJobs()` = `Generator.Len()`.
      - `pitchfork.go` — one generator per marker (`Generators []payload.Generator`,
        same order as `Template.Markers`), walked in lockstep; stops the
        instant the shortest generator is exhausted, matching Burp's own
        Pitchfork behavior (longer generators' remaining values are never
        used). `PitchforkJobs` / `PitchforkConfig`. `TotalJobs()` = length
        of the shortest generator. A `len(Generators) != len(Markers)`
        mismatch is treated as a setup error — `Jobs` sends nothing rather
        than guessing.
      - `cluster_bomb.go` — one generator per marker, full combinatorial
        product (odometer-style nested iteration: the *last* marker's
        generator varies fastest, earlier ones roll over and `Reset()`
        once everything to their right has cycled — same nested-`Reset()`
        pattern Sniper's outer loop already uses, generalized to N
        generators instead of markers-vs-one). `ClusterBombJobs` /
        `ClusterBombConfig`. `TotalJobs()` = product of every generator's
        `Len()`; an empty generator anywhere correctly produces zero jobs.
      All three have `httptest`-backed tests verifying the *exact* set of
      query-string combinations produced (same style as
      `sniper_test.go`), plus a mismatch/empty-generator edge case each
      for Pitchfork and Cluster bomb. Full suite (`go test -race ./...`)
      passes, attack package included.
      **Not wired into `cmd/cli` yet** — the CLI still only demos Sniper
      against `cmd/testserver`'s single-field `/login` endpoint, since
      exercising Pitchfork/Cluster bomb meaningfully needs a target with
      ≥2 independent fields (e.g. username+password), which testserver
      doesn't have yet. The attack-mode tests above are the verification
      for this step; wiring a multi-field CLI demo is optional follow-up,
      not a blocker for the next step below.

**Known environment quirk (dev sandbox only, shouldn't apply on your
machine):** `localhost` didn't resolve in this sandbox; had to use
`127.0.0.1` in the CLI's hardcoded template/target. Not a code issue.

- [x] `internal/payload` — payload processing rule chains, done as a
      `Generator` *decorator* rather than attack-mode-specific code:
      - `rules.go` — `Rule` interface (`Apply(value string) string`) plus
        six concrete rules: `URLEncodeRule`, `Base64EncodeRule`,
        `PrefixRule`, `SuffixRule`, `UpperCaseRule`, `LowerCaseRule`.
      - `chain.go` — `Chain` wraps a base `Generator` and a `[]Rule`,
        applying them in order to each value `Next()` produces (each rule
        sees the *previous* rule's output, so rule order is meaningful —
        e.g. `Prefix` then `Base64Encode` encodes the prefix too, the
        reverse doesn't). `Chain` itself implements `Generator`
        (`Reset`/`Len` delegate straight to `Base`, since rules transform
        values but never add/drop/reorder them), so it composes with
        Sniper, Battering ram, Pitchfork and Cluster bomb with zero
        changes to `internal/attack` — any `Generator` a `*Chain` wraps,
        and a `*Chain` itself, works anywhere a `Generator` is accepted.
      Tests cover each rule in isolation, rule-order sensitivity, a
      no-rules passthrough, and `Reset`/`Len` delegation. Full suite
      passes under `-race`.

## Next step

1. **(Optional, quick)** Give `cmd/testserver` a second query field (e.g.
   `username`) alongside `password`, and add a CLI demo (or a short-lived
   `main.go` variant) exercising Pitchfork and/or Cluster bomb — and now
   also a `Chain`-wrapped generator — end to end the same way Sniper
   already is. Proves things against a live server, not just unit tests.
2. **More generators**: numeric-range, brute-force (character-set ×
   length). Same `Generator` interface as `Wordlist`, so no changes needed
   to `internal/attack` or `internal/payload/chain.go` — a `Chain` can
   wrap these too, for free, same as it wraps `Wordlist` today.
3. **Match/extract rules** on `engine.Result` — flagging interesting
   responses (status/length/regex match) and pulling values out of a
   response body for use in later requests (needed for multi-stage auth
   flows, CSRF tokens, etc.).
4. **Web UI**: `internal/server` (HTTP+WS backend) + `web/` (static
   frontend) consuming the `engine.Result` stream over WebSockets instead
   of the CLI's console printer. This is the last item in the original
   plan and the biggest single chunk of remaining work — expect it to need
   its own multi-session arc, not a single sitting.

## How to resume this project (e.g. if a session runs out of budget)

If you're reading this because a previous session stopped partway (ran out
of its token/usage budget, got interrupted, etc.), there's no special
recovery needed — nothing was left half-written. Every change so far has
landed as a complete, tested commit; `go test -race ./...` passing is the
checkpoint. Just:

1. Open/clone this repo (`git log --oneline` to see what's actually landed
   — this file is a summary, the commits are ground truth).
2. Tell Claude: "Continue the Burp Intruder clone project — read
   STATUS.md and pick up from Next step."
3. Claude should re-run `go build ./... && go test -race ./...` first to
   confirm the working tree matches what this file claims, *then* start
   on the next unchecked item above — not re-derive the architecture or
   redo finished work from scratch.

This file is kept up to date after every meaningful chunk of work
specifically so that handoff works — update it (Done list, Next step,
this section if the resume story changes) before stopping, not just when
a session happens to end cleanly.
