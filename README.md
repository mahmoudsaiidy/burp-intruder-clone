# burp-intruder-clone

A from-scratch Go implementation of the attack engine behind Burp Suite's
**Intruder** module — for use against targets you are authorized to test.
It isn't a reverse-engineered copy of Burp's binary; it independently
implements the same attack modes, payload handling, concurrency model, and
results workflow, as a standalone Go core with a UI-agnostic design.

## Status

Core engine, all four attack modes, payload/match tooling, and multi-stage
request chaining are built and tested. A first slice of the web UI is up
too — an HTTP+WS server streaming a live demo attack to a browser page —
though it still only runs one hardcoded attack, the same as the CLI; a real
"configure your own attack" flow is the next non-optional step. See
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
- **Multi-stage chaining** — `internal/chain`: fires one or more
  prerequisite requests before the main attack (a login, a token fetch),
  pulls a value out of each response with a `match.Extractor`, and
  substitutes it into later stages' requests and into the final attack
  template — e.g. grab a CSRF token or session cookie and carry it into
  every attack request.
- **Web UI** — `internal/server` + `web/`: an HTTP+WS backend (hand-rolled
  stdlib WebSocket layer, no third-party dependency) streaming
  `engine.Result`/`match.Flagged` to a browser page in real time, served
  by `cmd/server`. Currently runs one hardcoded Sniper demo per connection
  against its own disposable local target — not yet a general front end
  for arbitrary attacks (see Status above).

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
the engine works, not yet a general-purpose front end.

You can also run the same kind of demo in a browser:

```sh
go run ./cmd/server
# open http://localhost:8080 and click "Run demo attack"
```

`cmd/server` is self-contained — it starts its own disposable local target
per run, so there's no second process to start first. It serves the page
from `web/` and streams results to it live over a WebSocket at `/ws`.

## Project layout

```
cmd/server/     entry point: HTTP+WS server, serves web/ and the /ws demo attack
cmd/cli/        headless entry point — runs one hardcoded Sniper attack
cmd/testserver/ standalone process wrapping internal/testtarget, for manual dev use
internal/
  template/     raw HTTP request parsing, §marker§ detection/injection
  payload/      payload generators + processing-rule chains
  attack/       attack-mode orchestration (sniper, battering ram, pitchfork, cluster bomb)
  engine/       worker pool, shared http.Client, rate limiter, result streaming
  match/        match/filter rules + value extraction
  chain/        multi-stage request chaining (extract a value, feed it into a later request)
  testtarget/   the mock login handler cmd/testserver and the web demo both attack
  config/       save/load attack configs (not built yet)
  server/       web UI backend: HTTP+WS, hand-rolled stdlib WebSocket layer
web/            static frontend (index.html, no build step)
```

## Roadmap

The next non-optional step is a real attack-configuration flow — submitting
your own request template, markers, attack mode, payload generator(s), and
match rules, instead of the CLI's and web UI's current hardcoded demo
attack. See [`STATUS.md`](./STATUS.md) for the detailed plan.

## Legal

This tool sends concurrent, automated requests to whatever target you
point it at. Only use it against systems you own or have explicit
authorization to test.
