<div align="center">

# Intruder

**A from-scratch Go implementation of the attack engine behind Burp Suite's Intruder — for authorized security testing.**

[![CI](https://github.com/mahmoudsaiidy/burp-intruder-clone/actions/workflows/ci.yml/badge.svg)](https://github.com/mahmoudsaiidy/burp-intruder-clone/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.24%2B-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](./CONTRIBUTING.md)

[Quick start](#quick-start) · [Features](#features) · [Usage](#usage) · [Examples](#examples) · [Architecture](#architecture) · [Contributing](#contributing) · [Status](#project-status)

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

- **A marker can go anywhere in the request** — `§...§` is found by
  scanning the raw request text itself, not a fixed set of fields, so the
  same engine that brute-forces a login's `password` field equally brute-
  forces a URL path segment (directory/file discovery, à la gobuster/ffuf)
  or a JSON/form body field, with no special mode needed. See
  [Beyond logins: paths and bodies](#beyond-logins-paths-and-bodies).
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
# open http://localhost:8080 and click "Run demo attack",
# or open http://localhost:8080/builder.html to configure and run your own
```

`cmd/cli` still only runs its built-in demo attack (a Sniper run against a
disposable local login endpoint) unless you pass it a JSON config file (see
[Usage](#usage) below). The web UI's `/builder.html` page lets you compose
and run your own request template, payloads, attack mode, and match rule
entirely in the browser — see [Project status](#project-status) for what's
still ahead.

## Usage

### CLI

```sh
go run ./cmd/cli
```

With no arguments, wires a request template, a wordlist, the Sniper attack
mode, the engine, and a `StatusMatcher` together by hand, then prints a
results table — fires six requests against a mock `/login` endpoint and
flags the one that returns `200`:

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

Given a path to a JSON `AttackConfig` instead, it runs that — same
`internal/config.Run` wiring the web UI's `/ws/config` uses, so a config
that works from one works from the other:

```sh
go run ./cmd/testserver &       # the target these examples attack
go run ./cmd/cli examples/sniper.json
```

See [Examples](#examples) below for ready-to-run configs covering all four
attack modes and a chain.

### Web UI

```sh
go run ./cmd/server
```

Self-contained — it starts its own disposable local target per run, so
there's nothing else to start first. Open `http://localhost:8080`, click
**Run demo attack**, and watch results stream into a live table over a
WebSocket at `/ws`.

To run your *own* request template instead of the demo, open
`http://localhost:8080/builder.html`: compose a raw HTTP request with
§marker§ injection points, pick an attack mode, configure a payload
generator per marker (with optional processing rules), build a match rule,
and optionally add chain stages — then run it, streamed the same way over
`/ws/config`. "Load testserver example" fills in a working config against
`go run ./cmd/testserver` to try it with nothing else to set up first.

### Beyond logins: paths and bodies

A `§marker§` is just a span of the raw request text — `internal/template`
scans for it the same way wherever it appears, so there's no special mode
for "attack the URL" vs. "attack a form field." The CLI/web UI demos and
`examples/` all happen to attack login query params, but the same engine
covers two other common uses just by moving where the marker sits:

- **Directory/file discovery** — put the marker in the path and feed it a
  wordlist, same idea as `gobuster`/`dirb`/`ffuf`:
  ```
  GET /§dir§ HTTP/1.1
  Host: target
  ```
  Use a `length` or `body_regex` matcher to filter out a uniform 404/soft-
  404 page so only genuine hits get flagged.
- **Body brute-forcing** — put the marker in a POST/PUT body field
  (JSON, form-encoded, whatever the target expects):
  ```
  POST /api/reset HTTP/1.1
  Host: target
  Content-Type: application/json
  Content-Length: 17

  {"token":"§t§"}
  ```
  **Caveat, confirmed by testing against the live code, not just reading
  it:** `Content-Length` is read once, as written in the template, and
  never recalculated after a payload is injected. This isn't just "wrong
  header, server complains" — `http.Request.Body` is wrapped to the
  *original* declared length, so a longer payload is **silently truncated
  client-side before the request is even sent** (verified: a 17-byte
  `Content-Length` with a 30-character payload injected sends exactly 17
  bytes of body, cutting the payload off mid-string, no error raised
  anywhere). A shorter payload instead leaves the connection waiting to
  write bytes that never come. Until `internal/template.Build`
  recalculates `Content-Length` after injection (not yet done — tracked in
  `STATUS.md`), keep every payload the same byte length for a body marker,
  or use a target that accepts `Transfer-Encoding: chunked` instead of a
  fixed `Content-Length`.

There isn't a wordlist-free / recursive / auto-calibrating directory-brute
mode built in (unlike a dedicated tool such as `ffuf`) — you supply the
wordlist and the matcher yourself, same as any other attack here.

## Examples

[`examples/`](./examples) has ready-to-run `AttackConfig` JSON files for
`cmd/cli` (or paste their contents into `builder.html`'s "Preview JSON" /
send flow), each targeting `go run ./cmd/testserver`'s default port
(`:8089`):

| File | Mode | Target endpoint | What it shows |
| --- | --- | --- | --- |
| [`sniper.json`](./examples/sniper.json) | Sniper | `/login` | one marker, a wordlist, flags the `200` |
| [`pitchfork.json`](./examples/pitchfork.json) | Pitchfork | `/login/multi` | two independent markers (username + password) walked in lockstep |
| [`cluster_bomb.json`](./examples/cluster_bomb.json) | Cluster bomb | `/login/multi` | the same two markers, full combinatorial product |
| [`chain_csrf.json`](./examples/chain_csrf.json) | Sniper + chain | `/login/secure` (via `/csrf` first) | a setup stage fetches a CSRF token and feeds it into every attack request |

`cmd/testserver` exposes all four endpoints these configs attack —
`/login` (single field, the original demo target), `/login/multi`
(independent username + password), `/csrf` (issues a token), and
`/login/secure` (requires that token plus both credentials) — so Pitchfork,
Cluster bomb, and chain support all have something real to run against
outside of `go test`, not just the single-field Sniper demo.

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
cmd/server/     entry point: HTTP+WS server, serves web/, the /ws demo attack, and /ws/config
cmd/cli/        headless entry point — runs the hardcoded Sniper demo, or a JSON config file if given one
cmd/testserver/ standalone process wrapping internal/testtarget, for manual dev use
internal/
  template/     raw HTTP request parsing, §marker§ detection/injection
  payload/      payload generators + processing-rule chains
  attack/       attack-mode orchestration (sniper, battering ram, pitchfork, cluster bomb)
  engine/       worker pool, shared http.Client, rate limiter, result streaming
  match/        match/filter rules + value extraction
  chain/        multi-stage request chaining (extract a value, feed it into a later request)
  testtarget/   mock login endpoints cmd/testserver and the web demo attack: a
                single-field /login (the original demo target), a two-field
                /login/multi (Pitchfork/Cluster bomb), and a /csrf + gated
                /login/secure pair (chain support)
  config/       JSON attack-config spec (template/target/mode/generators/matcher/chain) + build/run orchestrator
  server/       web UI backend: HTTP+WS, hand-rolled stdlib WebSocket layer
web/            static frontend — index.html (demo attack) + builder.html (configure & run your own), no build step
```

## Project status

Core engine, all four attack modes, payload/match tooling, multi-stage
chaining, the JSON `internal/config` layer, and the web UI — including
`builder.html`, a page to compose and run your own request template,
payloads, and match rule entirely in the browser — are built and tested
(`go test -race ./...` is green). See `STATUS.md` for what's still open.

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

[MIT](./LICENSE) © Mahmoud El-Saidy
