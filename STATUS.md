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
cmd/server/     entry point: HTTP+WS server, serves web/ and the /ws demo attack — DONE (first slice)
cmd/cli/        headless entry point — runs one hardcoded Sniper attack
cmd/testserver/ standalone process wrapping internal/testtarget, for manual dev use
internal/
  template/     raw HTTP request parsing, §marker§ detection/injection, Build() -> *http.Request
  payload/      Generator interface; Wordlist, NumericRange, BruteForce, processing-rule Chain — all DONE
  attack/       attack-mode orchestration; Sniper, Battering ram, Pitchfork, Cluster bomb all DONE
  engine/       worker pool, shared http.Client, rate limiter+jitter, Result streaming — DONE
  match/        match/filter rules on engine.Result + value extraction — DONE
  chain/        multi-stage request chaining (extract a value, feed it into a later request) — DONE
  testtarget/   the mock login handler cmd/testserver and the web demo both attack — DONE
  config/       JSON attack-config spec + Build/Run orchestrator — DONE
  server/       web UI backend (HTTP+WS + hand-rolled WebSocket layer) — DONE (first slice: demo attack only)
web/            static frontend (index.html, no build step) — DONE (first slice)
```

Concurrency model: payload generator (writer goroutine) → bounded job
channel → N worker goroutines (shared `*http.Client` for connection reuse)
→ bounded result channel → consumer (CLI printer, or now the web UI's
WebSocket stream via `internal/server`). One `context.Context` per attack
run for clean cancellation. Full design rationale was discussed and agreed
before any code was written — see git log / chat history if the "why"
behind any of this needs revisiting.

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

- [x] `internal/payload` — two more generators, both satisfying the plain
      `Generator` interface so they work everywhere `Wordlist` does
      (every attack mode, and as a `Chain` base):
      - `numeric_range.go` — `NumericRange{From, To, Step, MinDigits}` /
        `NewNumericRange(from, to, step, minDigits)`. Burp's "Numbers"
        payload type: an inclusive arithmetic sequence, with optional
        zero-padding (padding the digits, not the sign — `-2` pads to
        `-002`, not `00-2`). `Len()` is computed directly from the bounds
        (no walking the sequence), so it's correct before the first
        `Next()`. `Step <= 0` or `From > To` is treated as an empty range
        (`Len() == 0`, `Next()` always `false`) rather than an error or a
        panic — consistent with how `internal/attack` already treats an
        empty generator (zero jobs, not a crash).
      - `brute_force.go` — `BruteForce{Charset, MinLen, MaxLen}` /
        `NewBruteForce(charset, minLen, maxLen)`. Burp's "Brute forcer"
        payload type: every string drawable from `Charset`, shortest
        length first, each length's combinations in lexicographic order
        (odometer-style, same nested-increment idea `cluster_bomb.go`
        uses for combining markers, applied here to combining character
        positions instead). `Len()` = Σ `len(Charset)^L` for `L` from
        `MinLen` to `MaxLen`. An empty charset or `MaxLen < MinLen` or
        `MinLen < 1` is an empty generator, same convention as
        `NumericRange`.
      Tests cover basic sequences, step, zero-padding (including
      negatives), invalid-config-is-empty-not-a-crash, `Reset`, and that
      each composes with `Chain` (e.g. `NewChain(NewNumericRange(...),
      PrefixRule{...})`) exactly like `Wordlist` does. Full suite passes
      under `-race`.

- [x] `internal/match` — match/filter rules on `engine.Result`, plus value
      extraction, kept as two separate concerns (a Matcher answers
      yes/no, an Extractor answers "here's the string"):
      - `match.go` — `Matcher` interface (`Match(r engine.Result) bool`).
        `StatusMatcher`/`LengthMatcher` compare against a `CompareOp`
        (`Equal`/`NotEqual`/`LessThan`/`GreaterThan`/`LessOrEqual`/
        `GreaterOrEqual`, Burp's own six); `BodyRegexMatcher` /
        `HeaderRegexMatcher` match on a compiled `*regexp.Regexp` against
        the body or a named header; `ErrorMatcher` flags a Result whose
        *request* failed (timeout, connection refused) rather than
        completing with a status. `And`/`Or`/`Not` combine any of the
        above (`Or{}` matches nothing, same as Burp's "no conditions
        enabled"). A Result with `Err != nil` never matches
        Status/Length/body/header matchers — there's no status/body to
        compare against a failed request.
      - `extract.go` — `Extractor` interface
        (`Extract(r engine.Result) (string, bool)`). `BodyRegexExtractor`
        / `HeaderRegexExtractor` return the first capture group's text if
        the regex has one, else the whole match — mirrors Burp's
        "extract grammar". This is the primitive multi-stage flows
        (grab a CSRF token or session cookie from one response, feed it
        into the next request) will build on; the actual chaining —
        feeding an extracted value back into a later `Template.Build` —
        is NOT built yet, this is just the extraction primitive.
      - `flag.go` — `Flag(ctx, results <-chan engine.Result, m Matcher)
        <-chan Flagged` wraps a `Pool.Run` result stream, tagging every
        Result with whether it matched `m` without dropping or
        reordering anything (a "flag column", not a filter — same idea
        as a Burp Intruder results table). Closes on context
        cancellation or the input channel closing, same contract as
        `Pool.Run` itself. **Not wired into `cmd/cli` yet** — the CLI
        still prints the raw, untagged result stream.
      Tests cover every matcher (incl. the errored-Result-never-matches
      rule), every combinator, both extractors (capture group vs. whole
      match vs. no match vs. errored Result), and `Flag`'s
      ordering/tagging and cancellation behavior. Full suite passes
      under `-race`.
- [x] `cmd/cli` wired to `match.Flag` — the Sniper demo now flags
      `StatusMatcher{Equal, 200}` hits with a `HIT` column (`***` on a
      match) instead of printing the raw, untagged result stream.
      Verified live against `cmd/testserver`: `hunter2` → `200` is the
      only flagged row, in-order, all 6 rows still printed (flagging
      doesn't filter).

- [x] **Multi-stage chaining** — `internal/chain`, new package, orchestrated
      one level above both `internal/template` and `internal/attack` (the
      question STATUS.md left open last session — now settled): a chain
      `Stage` is just another request+extract step built from the same
      primitives those two packages already expose (`template.Template` to
      build it, `match.Extractor` to read it), and what `Resolve` produces
      is plain raw-text that the attack's own §marker§ parsing hasn't seen
      yet. So no changes were needed in `internal/attack` or
      `internal/template` to support this — an attack mode's `Jobs()` runs
      against a chain-resolved template exactly like any other template.
      - `Stage{Name, Raw, MarkerValues, Target, Extractor, Placeholder}` —
        one prerequisite request. `Placeholder` is a literal substring
        (e.g. `"{{csrf}}"`, deliberately *not* §-delimited, so it can never
        be confused with — or collide with — an attack's own §payload§
        markers) that gets swapped for this stage's extracted value in
        every later stage's `Raw` and in the final attack template.
      - `Resolve(ctx, client, stages, attackRaw) (resolvedAttackRaw string,
        extracted map[string]string, err error)` — runs stages
        *sequentially* (never concurrently: a later stage's own request
        text, e.g. a CSRF-token fetch that must present an earlier stage's
        session cookie, may depend on an earlier stage's value), firing
        each one with a small unexported `execute()` helper that returns
        the same `engine.Result` shape attack results use — so a Stage's
        `Extractor` is exactly the `match.Extractor` the main attack
        already has, no separate "setup response" type. A stage that fails
        to build, fails to fire, or whose `Extractor` finds nothing stops
        the whole chain immediately with an error naming that stage —
        same "setup problem, not a per-request condition" convention
        `internal/attack` already uses for template-build failures.
        `extracted` comes back keyed by `Stage.Name` (every stage's value,
        whether or not it had a `Placeholder`) for logging/debugging.
      - Duplicate stage names and two stages sharing one `Placeholder` are
        both rejected explicitly, rather than silently letting one
        overwrite the other.
      Tests: unit coverage for every error path above, plus two
      `httptest`-backed integration tests — one single-stage (CSRF token →
      main attack header), one genuinely multi-stage (a session stage's
      extracted value is asserted, server-side, to actually arrive on the
      *next* stage's own outgoing request, proving values really flow
      forward through the chain and not just into the final attack text) —
      and a check that the attack's own §marker§ survives chain resolution
      completely untouched. Full suite (`go test -race ./...`) passes,
      chain package included.
      **Not wired into `cmd/cli` yet** — `internal/chain` is a standalone,
      fully tested package; hooking a chain into the CLI's demo (or a web
      UI run) is follow-up wiring, not a design question anymore.

- [x] **Web UI, first slice** — `internal/server` (HTTP+WS backend) +
      `web/index.html` (static frontend), `cmd/server` as the entry point.
      Not the full original plan yet (a real "configure your own attack"
      flow needs `internal/config`, still not built — see item 2 below),
      but the architecture's other end-to-end proof: `engine.Result` /
      `match.Flagged` driving a *live web page* instead of just the CLI's
      console printer, the thing STATUS.md has been deferring since the
      very first entry.
      - **WebSocket layer is hand-rolled stdlib, not a third-party
        module** (`internal/server/websocket.go`) — `go.mod` has had zero
        dependencies through every package so far, and what this needed
        was narrow: handshake, unmasked server→client text frames,
        answering pings, noticing a client close. That's RFC 6455 §1.3 /
        §5, not a reason to take on `gorilla/websocket` or similar.
        Explicitly *not* implemented, because nothing here needs it:
        fragmented messages (everything sent/expected is a single FIN
        frame), the permessage-deflate extension, UTF-8 validation. A
        fragmented frame from a client is reported as an error rather than
        mishandled.
      - `cmd/server/main.go` — serves `web/` and wires `/ws` to
        `internal/server.NewHandler`. Self-contained: no second process to
        start first (see next bullet).
      - `internal/testtarget` — the mock login handler **refactored out of
        `cmd/testserver`** into its own package (`LoginHandler()`,
        `CorrectPassword`) so both `cmd/testserver` (a real standalone
        process, for manual/interactive dev use) and the web demo attack
        can use it. The web demo (`internal/server/demo.go`) starts its
        own instance on an ephemeral `127.0.0.1:0` port per WebSocket
        connection, so `go run ./cmd/server` + opening the page in a
        browser needs nothing else running first.
      - `/ws` runs the same hardcoded Sniper demo `cmd/cli` already ran
        (same wordlist, same single-marker request), now streamed as one
        JSON `wsMessage` per `engine.Result` (`type: "result"`, with
        `hit`/`status`/`length`/`duration_ms`/`payload`, or `error` when
        the request itself failed) followed by one `type: "done"` summary
        (`total`, `hits`) and a clean WebSocket close. A background reader
        on the connection cancels the attack's context the instant the
        client disconnects or sends its own close frame, so a closed
        browser tab doesn't leave the demo attack running to completion
        for nothing.
      - `web/index.html` — single self-contained file (inline CSS/JS, no
        build step, matches the "no native-toolchain dependency" stack
        decision above), a button that opens the WebSocket and renders a
        live results table, starring flagged rows.
      Tests: `internal/server/websocket_test.go` — `acceptKey` against the
      literal RFC 6455 worked example (not just self-consistency), a
      rejected non-WebSocket request, a full real-TCP handshake +
      text-frame + close exercised through an actual `httptest.Server`
      (the stdlib has no WebSocket client, so the test rolls the minimum
      one itself: handshake + masked frame writes; frame *decoding* reuses
      the package's own `decodeFrame`, which already handles masked and
      unmasked frames identically by reading the bit off the wire), and a
      `net.Pipe`-based check that a ping is answered with a pong and never
      surfaces to `ReadMessage`'s caller. `internal/server/demo_test.go` —
      a full `/ws` run against a real `httptest.Server` asserting the
      exact message sequence (one `result` per wordlist entry, exactly one
      flagged, matching the `hunter2` payload, a `done` with the right
      `total`/`hits`), plus the static-file-serving branch of
      `NewHandler`. `go test -race ./...` passes, `internal/server`
      included; also manually verified live — `go run ./cmd/server` plus
      an independent raw-socket WebSocket client (not the Go test
      harness) got the same 6-requests/1-hit result over real TCP.

- [x] **Real attack configuration, backend slice** — `internal/config`,
      new package, the "next meaty step" from the previous version of this
      file: a JSON-serializable `AttackConfig` (raw template, target,
      `Mode`, one `GeneratorSpec` per marker Mode needs, an optional
      recursive `MatcherSpec` tree — `and`/`or`/`not` combinators included
      — and an optional `[]StageSpec` chain) plus the code that builds it
      into the live `template.Template` / `payload.Generator` /
      `match.Matcher` / `chain.Stage` objects the rest of the codebase
      already defines, and runs them end to end.
      - `config.go` — `AttackConfig`, `TargetSpec`, `Load`/`LoadFile`
        (`json.Decoder` with `DisallowUnknownFields`, so a typo in a
        hand-written config fails loudly instead of silently doing
        nothing).
      - `generator.go` — `GeneratorSpec` (wordlist / numeric_range /
        brute_force) + `RuleSpec`, building a bare generator or, if
        `Rules` is set, a `*payload.Chain` wrapping it — same
        decorator relationship `internal/payload` already has.
      - `matcher.go` — `MatcherSpec`, recursive for `and`/`or`/`not`,
        covering every `internal/match` matcher plus Burp's six compare
        ops (`eq`/`ne`/`lt`/`gt`/`le`/`ge`, defaulting to `eq`).
      - `stage.go` — `StageSpec`/`ExtractorSpec`, the JSON form of
        `chain.Stage`/`match.Extractor`.
      - `build.go` — `AttackConfig.Build()` does every validation in one
        place: template parses, `Mode` is one of the four known modes,
        generator count matches what `Mode` needs (1 for Sniper/Battering
        ram, one per marker for Pitchfork/Cluster bomb — checked against
        the *parsed* template's own marker count, not just trusted from
        the config), each generator/matcher/stage spec itself builds
        cleanly. No `Matcher` configured defaults to `match.Or{}` (flags
        nothing — same "no conditions enabled" default Burp's filter has,
        not an error). `Workers`/`TimeoutSeconds` get defaults (5, 30s)
        when unset.
      - `run.go` — `AttackConfig.Run(ctx)` (and `Built.Run`) is the one
        place that now wires a built config into a real run: resolves any
        chain stages first (re-parsing the attack template afterward,
        since substitution shifts §marker§ byte offsets even though it
        never touches the markers themselves), switches on `Mode` to pick
        `attack.Jobs` / `BatteringRamJobs` / `PitchforkJobs` /
        `ClusterBombJobs`, builds an `engine.Pool`, and returns
        `match.Flag`'s output channel plus the expected request total —
        the exact wiring `cmd/cli`'s demo and the web UI's demo attack
        each used to hand-roll separately (see both those entries above).
      Tests: `config_test.go` — every spec's `Build()` against valid and
      invalid input (bad types, bad regexes, missing required fields,
      `and`/`or` with no children, `not` with no child, mode/generator-
      count mismatches for all four modes), `Load`'s unknown-field
      rejection, and the default-value behavior. `run_test.go` —
      `httptest`-backed integration tests: a Sniper run and a Pitchfork
      run end to end from JSON (mirroring `internal/attack`'s own test
      style), and a chain-stage run proving a config-described CSRF-token
      stage's value really reaches the main attack's requests. Full suite
      (`go test -race ./...`) passes, config package included.
      **Wired into both front ends**: `cmd/cli` now runs
      `./intruder-cli path/to/config.json` through `config.Run` (no
      arguments still runs the original hardcoded demo, byte-for-byte
      unchanged — `runDemo()` in `cmd/cli/main.go`); the web UI gained a
      new `/ws/config` endpoint (`internal/server/config_attack.go`,
      `handleWSConfig` in `server.go`) that reads one JSON `AttackConfig`
      as the client's first text frame and streams the same `wsMessage`
      shape `/ws` already does, plus a `"started"` message carrying the
      expected total before the first result (`/ws` itself is untouched —
      still the hardcoded demo, zero behavior change for the existing
      `web/index.html` button). `internal/server/config_attack_test.go`
      covers `/ws/config` end to end (a Sniper run against a mock login
      handler the test itself serves, checking `started`/`result`/`done`)
      and a config that fails `Build()` coming back as a single `"error"`
      message rather than a panic. Also manually verified live: built
      `cmd/testserver` and `cmd/cli` as real binaries, ran the test target
      on a real port, and fed `cmd/cli` a JSON config file — got the same
      `hunter2` → `200` hit, byte-for-byte the same results-table format
      the hardcoded demo prints.
      **Not built yet**: a request-builder *page* in `web/` that composes
      the JSON from form inputs — today a web client has to construct and
      send the `AttackConfig` JSON itself (exactly what
      `config_attack_test.go` does). `internal/config` and `/ws/config`
      are the real, tested backend half of "real attack configuration";
      the UI half is its own, more purely front-end-design task — see
      Next step below.

- [x] **Request-builder UI** — `web/builder.html`, a new page alongside the
      existing demo (`web/index.html`, which now links to it). Pure
      client-side vanilla JS, no build step, matching the rest of `web/`'s
      stack decision: a target section (host/port/TLS), a raw-template
      textarea with a live §marker§ counter, a mode picker, a
      per-marker generator editor (wordlist / numeric range / brute force,
      each with an add-a-rule UI for the processing-rule chain) that grows
      or shrinks to the exact count `Mode` needs (1 for Sniper/Battering
      ram, one per detected marker for Pitchfork/Cluster bomb), a
      match-rule builder (any number of conditions combined with AND/OR,
      each with an optional NOT), and a collapsible chain-stage editor
      (name/raw/target/extractor/placeholder, repeatable) — all assembled
      client-side into one `AttackConfig` JSON document and run by opening
      `/ws/config` and sending it as the first frame, rendering results
      into the same live table style `index.html`'s demo already used.
      "Preview JSON" shows the exact document before sending (useful for
      saving it as a `cmd/cli path/to/config.json` file, too); "Load
      testserver example" fills in a working Sniper config against
      `cmd/testserver`'s default port, for a first run with nothing else
      to configure.
      - **Found and fixed a real trap in the process**: a hand-typed
        template missing its terminating blank line (`http.ReadRequest`
        requires headers end with CRLF CRLF) makes `template.Build` fail
        for every job, which `attack.Jobs` treats as a "setup problem,
        not a per-request condition" and silently stops the whole job
        sequence — `/ws/config`'s `"started"` message still reports the
        correct expected total (computed independently from marker ×
        generator counts), but zero `"result"` messages ever follow and
        the run just reports `done` with 0/0. Caught this by actually
        running the built page end-to-end against a live `cmd/testserver`
        (headless-browser-driven, not just unit-level), not from reading
        the code. The builder now normalizes every outgoing template
        (main attack and each chain stage) to always end in a blank line
        before sending, rather than silently producing an empty-looking
        run the first time someone forgets it.
      Verified live: headless-browser-driven runs against a real
      `cmd/server` + `cmd/testserver` pair — a Sniper run through the
      builder flags `hunter2` as the one hit among 5 requests (matching
      the hardcoded demo's own known-good result); mode switching
      regenerates the right generator-block count (1 for
      Sniper/Battering ram, N for Pitchfork/Cluster bomb against an
      N-marker template); rule-chip add/remove, multi-condition
      AND/OR matcher building (and the single-condition and
      zero-condition/omitted-matcher cases), and chain-stage add both
      render and serialize correctly into the JSON `AttackConfig` shape
      `internal/config` expects. `gofmt -l .` and `go vet ./...` both
      clean; `go test -race ./...` unaffected (no Go files changed — this
      was a `web/`-only addition).

- [x] **Both "optional, quick" items above, done together** — `internal/testtarget`
      gained three new endpoints alongside the original, byte-for-byte
      unchanged `/login` (`LoginFunc`, still the original demo target):
      - `/login/multi` (`MultiFieldLoginFunc`) — independent `username` +
        `password` query params (`CorrectUsername = "admin"`,
        `CorrectPassword` reused), `200` only if both are right. Gives
        Pitchfork/Cluster bomb a real two-marker target to run against
        outside of `httptest`.
      - `/csrf` (`CSRFFunc`) — returns `token=<CSRFToken>` in the body,
        the exact `token=(\w+)` shape `internal/chain`'s and
        `internal/config`'s own tests already extract with a body-regex
        extractor, so an example chain config can reuse the same pattern.
      - `/login/secure` (`SecureLoginFunc`) — like `/login/multi` but also
        requires `csrf=<CSRFToken>`, i.e. a login that genuinely can't be
        attacked in one request. Exercises `internal/chain` +
        `internal/config`'s chain support (fetch the token, then feed it
        forward) against a real standalone target.
      - `Mux()` — combines all four endpoints (including the original
        `/login`) onto one `http.Handler`; `cmd/testserver` now serves
        this instead of just `LoginHandler()`, and logs all four routes on
        startup. `internal/server`'s hardcoded demo deliberately keeps
        using `LoginHandler()` (just `/login`) unchanged, so its exact
        6-requests/1-hit demo result is untouched.
      - `internal/testtarget` gained its first test file
        (`testtarget_test.go`) — table-driven coverage of every endpoint's
        status-code branches, plus `TestMuxKeepsSingleFieldLoginUnchanged`
        guarding the compatibility promise `LoginFunc`'s doc comment
        makes. Previously this package had zero tests.
      - `examples/` — four ready-to-run `AttackConfig` JSON files
        (`sniper.json`, `pitchfork.json`, `cluster_bomb.json`,
        `chain_csrf.json`), one per attack mode plus the chain case, each
        targeting `cmd/testserver`'s default port. Verified live: built
        `cmd/testserver` and `cmd/cli` as real binaries, ran the test
        target, and fed `cmd/cli` each example — every one produces
        exactly one flagged `200` hit among the rest `401`s, matching each
        config's intended one correct credential/pair/chain value.
        README gained an "Examples" section documenting all four
        alongside the new testtarget endpoints.
      `go build ./...`, `go vet ./...`, `gofmt -l .` (clean), and
      `go test -race ./...` all pass, `internal/testtarget` included.

## Known bug (found while documenting marker-in-body usage)

**`internal/template.Build` does not recalculate `Content-Length` after
injecting payload values**, so a marker placed in a request *body* (as
opposed to the URL/query/headers, where this has no effect) silently
corrupts the request whenever the payload's byte length differs from
whatever `Content-Length` was written in the template.

Confirmed live (not just by reading the code): built a template with
`Content-Length: 17` and a body `{"token":"§t§"}`, then called `Build` with
a 30-character payload. `req.ContentLength` stayed `17`, and reading
`req.Body` back returned exactly `{"token":"a_much_` (17 bytes) — the
payload is **silently truncated client-side**, before the request is even
sent, not rejected or flagged anywhere. A payload shorter than the
original placeholder would instead leave `Content-Length` promising bytes
the body never supplies.

This is a real gap, not just a doc caveat (README's new "Beyond logins:
paths and bodies" section under Usage documents the workaround — same-
length payloads, or a target that accepts chunked encoding — but the
underlying behavior should be fixed, not just documented around).

**Suggested fix, next session:** in `Template.Build` (`internal/template/parser.go`),
after `Inject` resolves `values` into `resolved` but before
`http.ReadRequest` parses it, detect whether any marker fell inside the
body (i.e. after the blank-line header/body boundary) and, if so,
recompute `Content-Length` from the actual resolved body length before
parsing — or, simpler, let `http.ReadRequest` parse normally and then
override `req.ContentLength` (and the `Content-Length` header text) from
`len(bodyBytes)` whenever a `Content-Length` header is present at all,
regardless of where the marker was (harmless no-op when nothing in the
body changed). Needs a test mirroring the repro above, plus one proving a
header/query-only marker still leaves `Content-Length` untouched when the
body itself didn't change size.

## Next step

1. **Fix the `Content-Length` bug above** — the most concrete, correctness-
   affecting item currently open.
2. Promote the core engine packages out of `internal/` to a public `pkg/`
   API, per the README's "As a library" section — the README already
   invites this on request but nothing's moved yet.
3. A saved-config library/gallery in the web UI (list, re-load, re-run a
   previously saved `AttackConfig` from `builder.html`, rather than only
   "Preview JSON" + "Load testserver example") — `internal/config`'s
   `LoadFile` already does the hard part for the CLI side; this would be
   the web equivalent.
4. `internal/payload` has four generator types; Burp Intruder also has a
   "Character substitution" / "Illegal Unicode" / date-range payload type
   historically — worth a look if API-compat with Burp's own payload list
   becomes a goal, but not clearly needed yet.

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
