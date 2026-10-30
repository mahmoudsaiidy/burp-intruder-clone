# Security Policy

## Responsible use of this tool

This project sends concurrent, automated HTTP requests to whatever target
you configure. **Only point it at systems you own or have explicit,
documented authorization to test.** Running it against third-party systems
without authorization may be illegal in your jurisdiction. The maintainer
takes no responsibility for misuse — see the [LICENSE](./LICENSE) and the
Legal section of the [README](./README.md#legal).

## Reporting a vulnerability in the tool itself

If you find a security issue *in this codebase* — something that would let
the tool be tricked into doing something other than what its user
intended (e.g. a crafted response from a target causing memory corruption,
an injection vector in how a config or request template is parsed, that
kind of thing) — please report it privately rather than opening a public
issue:

- Email **m.elsaidy35@gmail.com** with a description and, if possible, a
  minimal reproduction.
- Please allow a reasonable amount of time to respond and address the
  report before any public disclosure.

This is a young, actively developed project (see [`STATUS.md`](./STATUS.md)
for current scope) without a dedicated security team, so response times
are best-effort, but every report will be read and taken seriously.

## Supported versions

There are no tagged releases yet — `main` is the only supported line.
Once releases start, this section will be updated with which versions
receive security fixes.
