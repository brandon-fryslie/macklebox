# macklebox

An MIT-licensed tool for keeping application settings in sync across machines:
it keeps the one real copy of each config file in a folder that already syncs
between your machines (a cloud-drive folder or any replicated directory) and
wires each application to read it from there.

This repository is a clean-room reimplementation built **only** from the
behavioral specification in [`appspec/`](appspec/) — a black-box description of
an existing config-sync tool's observable behavior, written so an independent
team can rebuild a behaviorally-equivalent, command-line-compatible program
without reference to any other implementation.

## Status

Implementation in progress (Go). The command-line boundary — invocation
grammar, global options, dispatch order, exit codes per
[`appspec/02-invocation.md`](appspec/02-invocation.md) — is built; every
subcommand currently stops at the config-load gate until the resolver layer
lands. Start at [`appspec/00-overview.md`](appspec/00-overview.md) — the spec
reads top-down through altitudes (product contract → architecture → boundary
detail).

## Building

```sh
go build ./cmd/mackup   # produces ./mackup
go test ./...
```

## Layout

| Path | What it is |
|------|------------|
| `appspec/` | The functional specification that drives the build (source of truth) |
| `cmd/mackup/` | Console entry point (the only code touching real argv/streams/exit) |
| `internal/cli/` | Invocation grammar, dispatch pipeline, usage/version output |
| `LICENSE`  | MIT |
