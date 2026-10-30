# burp-intruder-clone

A from-scratch Go implementation of the attack engine behind Burp Suite's
**Intruder** module — for use against targets you are authorized to test.
It isn't a reverse-engineered copy of Burp's binary; it independently
implements the same attack modes, payload handling, concurrency model, and
results workflow, as a standalone Go core with a UI-agnostic design.

## Status

Core engine, all four attack modes, and payload/match tooling are built,
tested, and verified end to end against a local test server. Multi-stage
request chaining and the web UI are not built yet. See
[`STATUS.md`](./STATUS.md) for the detailed, up-to-date breakdown and the
resume point for ongoing work.

## What it does

Given a raw HTTP request with one or more `§marker§`-delimited injection
points, it substitutes payload values into those markers, fires the
resulting requests concurrently against the target, and streams back
results you can match/flag on status code, length, headers, or body
content — the same core workflow as Burp Intruder, driven entirely from
Go code (no GUI yet).

- **Attack modes** — `internal/attack`
  - **Sniper** — one marker varies at a time, others held at their
    original value; repeats for each marker.
  - **Battering ram** — a single payload set broadcast into every marker
    at once.
  - **Pitchfork** — one payload generator per marker, walked in lockstep;
    stops when the shortest generator is exhausted.
  - **Cluster bomb** — one payload generator per marker, full combinatorial
    product across all of them.
- **Payload generators** — `internal/payload`
  - `Wordlist` — a fixed list of values.
  - `NumericRange` — an arithmetic sequence with optional zero-padding.
  - `BruteForce` — every string drawable from a charset, shortest first.
  - `Chain` — wraps any generator with an ordered list of processing
    rules (URL-encode, Base64-encode, prefix, suffix, upper/lower case),
    composing with every attack mode with no extra code.
- **Engine** — `internal/engine`: a bounded worker pool over a shared,
  connection-pooled `http.Client`, with a rate limiter + jitter, a
  response body-size cap, and context-based cancellation.
- **Matching & extraction** — `internal/match`: `Matcher`s
  (status/length/body-regex/header-regex/error, composable with
  and/or/not) tag each result without dropping or reordering it — a
  "flag column," not a filter — plus `Extractor`s for pulling a value
  (e.g. a token) out of a response body or header.
- **Request templates** — `internal/template`: parses a raw HTTP request,
  finds `§marker§` positions, and rebuilds a concrete `*http.Request` for
  a given payload set.

## Requirements

- Go 1.24+

## Build & test

```sh
go build ./...
go test -race ./...
```

## Try it

`cmd/cli` is a headless demo that wires everything together and runs a
Sniper attack against a disposable local target — useful for seeing the
whole pipeline run end to end.

```sh
# terminal 1: start the mock login target (correct password is "hunter2")
go run ./cmd/testserver

# terminal 2: run the Sniper demo against it
go run ./cmd/cli
```

The CLI tries a small built-in wordlist against the target's `password`
field and prints a results table, flagging the `200` response:

```
HIT    STATUS  PAYLOAD          LENGTH   TIME      ERROR
                123456           -        -         ...
...
***    200     hunter2          ...      ...
```

`cmd/cli` currently runs one hardcoded Sniper attack; it's a proof that
the engine works, not yet a general-purpose front end — that's what the
planned web UI (`internal/server` + `web/`) is for.

## Project layout

```
cmd/server/     entry point: HTTP+WS server, wires everything (not built yet)
cmd/cli/        headless entry point — runs one hardcoded Sniper attack
cmd/testserver/ throwaway local HTTP target for dev testing, not part of the tool
internal/
  template/     raw HTTP request parsing, §marker§ detection/injection
  payload/      payload generators + processing-rule chains
  attack/       attack-mode orchestration (sniper, battering ram, pitchfork, cluster bomb)
  engine/       worker pool, shared http.Client, rate limiter, result streaming
  match/        match/filter rules + value extraction
  config/       save/load attack configs (not built yet)
  server/       web UI backend, HTTP+WS (not built yet)
web/            static frontend (not built yet)
```

## Roadmap

1. Multi-stage request chaining — extract a value (CSRF token, session
   cookie) from one response and feed it into a later request in the same
   run.
2. Web UI — an `internal/server` HTTP+WS backend and a `web/` frontend
   consuming the engine's result/flag stream, replacing the CLI's console
   printer as the primary interface.

## Legal

This tool sends concurrent, automated requests to whatever target you
point it at. Only use it against systems you own or have explicit
authorization to test.
