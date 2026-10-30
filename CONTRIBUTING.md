# Contributing

Thanks for considering a contribution — this project is young (see
[`STATUS.md`](./STATUS.md) for exactly what's built and what isn't), so
there's real room to shape it.

## Before you start

- For anything beyond a small fix (typos, obvious bugs), please open an
  issue first to discuss the approach. This project has a fairly deliberate
  architecture (see `STATUS.md`'s "Architecture" section) and it saves
  everyone time to agree on direction before writing code.
- Check [`STATUS.md`](./STATUS.md) for the current state and the planned
  next steps — it's kept up to date and is the fastest way to see what's
  genuinely unclaimed work versus already in progress.

## Development setup

Requires Go 1.24+. No other toolchain, no CGO, no build step for the
frontend (`web/` is plain HTML/CSS/JS).

```sh
git clone https://github.com/mahmoudsaiidy/burp-intruder-clone
cd burp-intruder-clone
go build ./...
go test -race ./...
```

Try the CLI demo or the web UI (see the main [README](./README.md#try-it)
for both).

## Making changes

- **Keep `go test -race ./...` passing.** The project has no CI-exempt
  packages — every `internal/` package has tests, and PRs are expected to
  keep that true, including for new code.
- **Format with `gofmt`** (`gofmt -l .` should print nothing) and run
  `go vet ./...` before opening a PR. CI checks both.
- **Match the existing style.** Doc comments here tend to explain *why* a
  design choice was made, not just *what* the code does — skim a file or
  two in `internal/` (e.g. `internal/chain/chain.go`) to get a feel for it
  before adding a new package.
- **New packages should have tests that exercise real behavior**, not just
  mocks — several packages use `httptest` to verify behavior against a
  live local server (e.g. `internal/attack`'s query-combination tests,
  `internal/server`'s WebSocket tests). Prefer that style where it fits.
- **No new dependencies without discussion.** `go.mod` has stayed
  dependency-free through every package so far (including a hand-rolled
  WebSocket layer in `internal/server` for exactly this reason) — that's a
  deliberate choice, not an oversight, so pulling in a module is worth
  raising in an issue first.

## Submitting a pull request

1. Fork the repo and create a branch from `main`.
2. Make your change, with tests.
3. Run `go build ./... && go vet ./... && gofmt -l . && go test -race ./...`
   locally — this is exactly what CI runs.
4. Open a PR describing what changed and why. Reference any related issue.
5. Update [`STATUS.md`](./STATUS.md) if your change affects what's done or
   what the next steps are — it's the project's single source of truth for
   status, and stale entries there are worse than none.

## Reporting bugs / requesting features

Use the issue templates. For anything that looks like a security
vulnerability in the tool itself (not a target you're testing *with* it),
see [`SECURITY.md`](./SECURITY.md) instead of a public issue.

## Code of conduct

This project follows the [Contributor Covenant](./CODE_OF_CONDUCT.md).
