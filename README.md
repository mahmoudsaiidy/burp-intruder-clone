<div align="center">

# Intruder

**A from-scratch Go implementation of the attack engine behind Burp Suite's Intruder — for authorized security testing.**

[![CI](https://github.com/mahmoudsaiidy/burp-intruder-clone/actions/workflows/ci.yml/badge.svg)](https://github.com/mahmoudsaiidy/burp-intruder-clone/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.24%2B-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](./CONTRIBUTING.md)

[Quick start](#quick-start) · [Features](#features) · [Usage](#usage) · [Architecture](#architecture) · [Contributing](#contributing) · [Status](#project-status)

</div>

---

Intruder automates sending many HTTP requests with varying payloads against
a target you're authorized to test, and lets you match/flag the responses
that matter — the same workflow as Burp Suite's Intruder module. It is
**not** a reverse-engineered copy of Burp's binary; it's an independent,
from-scratch implementation in Go that matches its attack modes, payload
handling, concurrency model, and results workflow, built as a standalone
core with a CLI and a web UI on top.

> **⚠️ Authorized use only.** This tool fires concurrent, automated
> requests at whatever target you point it at. Only use it against systems
> you own or have explicit, documented authorization to test. See
> [Legal & responsible use](#legal--responsible-use).

## Features

- **Four attack modes**, matching Burp's own semantics exactly:
  - **Sniper** — one marker varies at a time, others held at their original value.
  - **Battering ram** — a single payload broadcast into every marker at once.
  - **Pitchfork** — one generator per marker, walked in lockstep.
  - **Cluster bomb** — one generator per marker, full combinatorial product.
- **Four payload generator types** — wordlist, numeric range (with
  zero-padding), brute force (charset + length range), and processing-rule
  chains (URL-encode, Base64, prefix/suffix, case) that wrap *any* other
  generator.
- **Multi-stage request chaining** — fire a setup request (a login, a
  token fetch), extract a value from its response, and carry it forward
  into later requests — e.g. a CSRF token or session cookie.
- **Match & filter** on status code, response length, headers, or a body
  regex, with `AND`/`OR`/`NOT` combinators, plus an extractor for pulling
  values out of responses (grep-style, with capture-group support).
- **A tuned, concurrent HTTP engine** — a bounded worker pool over a
  shared, connection-pooled client, optional rate limiting + jitter, and
  context-based cancellation.
- **Two interfaces**: a headless CLI and a web UI (HTTP + WebSocket,
  live-streaming results to the browser) over the same engine.
- **Zero third-party dependencies.** Everything, including the WebSocket
  layer, is pure Go standard library — see [Why no dependencies?](#why-no-third-party-dependencies).

## Quick start

Requires **Go 1.24+**. Nothing else — no CGO, no native toolchain, no
frontend build step.

```sh
git clone https://github.com/mahmoudsaiidy/burp-intruder-clone
cd burp-intruder-clone
go build ./...
```

Then either run the CLI demo:

```sh
go run ./cmd/cli
```

...or open the web UI:

```sh
go run ./cmd/server
# open http://localhost:8080 and click "Run demo attack"
```

Both currently run a built-in demo attack (a Sniper run against a
disposable local login endpoint) rather than taking your own request
template yet — see [Project status](#project-status) for what's next.

## Usage

### CLI

```sh
go run ./cmd/cli
```

Wires a request template, a wordlist, the Sniper attack mode, the engine,
and a `StatusMatcher` together, then prints a results table — fires six
requests against a mock `/login` endpoint and flags the one that returns
`200`:

```
HIT    STATUS  PAYLOAD          LENGTH   TIME      ERROR
                123456           -        -
                password         -        -
                admin            -        -
***    200     hunter2          ...      ...
                letmein          -        -
                qwerty           -        -

done: 6 requests (1 flagged) in ...
```

### Web UI

```sh
go run ./cmd/server
```

Self-contained — it starts its own disposable local target per run, so
there's nothing else to start first. Open `http://localhost:8080`, click
**Run demo attack**, and watch results stream into a live table over a
WebSocket at `/ws`.

### As a library

Every attack-engine package lives under `internal/` for now (see
[Project layout](#project-layout)), so it isn't importable from outside
this module yet. If you want to build on the engine directly rather than
through the CLI/web UI, open an issue — promoting a package to a public
`pkg/` API is a reasonable ask and easy to prioritize if there's demand.

## Architecture

```
request template + markers (§payload§)
            │
   payload generator(s)  ──▶  attack mode (sniper / battering ram / pitchfork / cluster bomb)
            │                            │
  [optional: chain.Resolve              bounded job channel
   injects values from                   │
   setup requests first]          N worker goroutines, shared *http.Client
                                          │
                                 bounded result channel
                                          │
                              match/filter + extract  ──▶  CLI printer / WebSocket stream
```

One `context.Context` per attack run drives clean cancellation end to end
— closing a browser tab mid-attack, for instance, cancels the run instead
of leaving it to finish for nothing.

### Why no third-party dependencies?

`go.mod` has stayed dependency-free through every package, including the
web UI's WebSocket layer (`internal/server/websocket.go`), which is a
~250-line, intentionally-scoped implementation of RFC 6455 rather than a
pull of `gorilla/websocket` or similar. This is a deliberate project
constraint, not an oversight — it keeps the build trivial (`go build ./...`
and nothing else, ever) and the dependency surface at zero. New
dependencies are possible but are expected to be discussed in an issue
first; see [`CONTRIBUTING.md`](./CONTRIBUTING.md).

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

## Project status

Core engine, all four attack modes, payload/match tooling, multi-stage
chaining, and a first slice of the web UI are built and tested
(`go test -race ./...` is green). Both interfaces currently run one
hardcoded demo attack rather than taking a user-supplied request — real
attack configuration is the next planned step.

[`STATUS.md`](./STATUS.md) is the authoritative, continuously updated
breakdown: what's done, what's deliberately deferred, and the exact next
step, kept detailed enough that picking the project back up doesn't
require re-deriving any design decisions.

## Contributing

Contributions are welcome — this is an early-stage project with real open
work (see `STATUS.md`'s "Next step" section). Please read
[`CONTRIBUTING.md`](./CONTRIBUTING.md) before opening a PR: it covers
setup, the testing style this project uses, and the "open an issue before
a big change" convention. Everyone participating is expected to follow the
[Code of Conduct](./CODE_OF_CONDUCT.md).

Found a security issue in the tool itself (not a target you're testing
*with* it)? See [`SECURITY.md`](./SECURITY.md) for how to report it
privately.

## Legal & responsible use

This tool sends concurrent, automated requests to whatever target it's
pointed at. **Only use it against systems you own or have explicit,
written authorization to test.** Unauthorized use against third-party
systems may be illegal in your jurisdiction. The author and contributors
accept no liability for misuse — see the [LICENSE](./LICENSE).

## License

[MIT](./LICENSE) © Mahmoud Saiidy
