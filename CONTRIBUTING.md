# Contributing to verikt

verikt is a Go CLI with an embedded Rust analysis engine. The engine binary is not
committed, so a fresh clone does **not** build until you produce it — that is the one
setup step that is not obvious, and it is first below.

## Setup

Tool versions are pinned in `.mise.toml`, matching what CI uses.

```bash
mise install            # Go, Rust, Bun
mise run build-engine   # builds the Rust engine and places it for //go:embed
go build ./...
```

### Why the second step is required

`internal/engineclient/embed.go` embeds the engine with `//go:embed`, and
`internal/engineclient/bin/` is gitignored — binaries do not belong in git. The embed
directive needs the file to exist at compile time, so without it you get:

```
pattern bin/verikt-engine-darwin-arm64: no matching files found
```

That is a missing build artefact, not a broken checkout. `mise run build-engine`
compiles the engine and copies it to the path for your platform.

The step is mandatory, not a nicety: `//go:embed` failing is a **compile** error, so
until you run it `go build`, `go vet` and `go test` all fail across the module. The
runtime fallback to Go-native analysis only covers platforms with no engine build at
all (`embed_other.go`), not a local checkout that has not produced one.

## Verifying a change

Run one command:

```bash
mise run verify
```

It runs every gate CI runs, in CI's order, **against a freshly built engine** — that last
part matters. Verifying against a stale embedded engine produces a passing local run that
says nothing about CI, which has already happened twice on this repository: once with an
older golangci-lint reporting no issues on code CI rejected, once with a debt baseline
measured four findings short.

The individual gates, if you need them separately:

```bash
mise run check-engine          # cargo fmt --check, clippy -D warnings, cargo test
mise run build-engine          # rebuild and re-embed before measuring anything
go test ./... && go vet ./...
golangci-lint run              # mise pins the version CI installs
verikt check                   # verikt governs itself — see verikt.yaml
```

`verikt check` exits non-zero only for **error**-severity findings. Warning-level
findings are real debt and are ratcheted: `.github/verikt-debt-baseline` holds the
current count, and CI fails if it rises. Lower it when you pay debt down. Raising it
needs a reason in the commit message.

The docs site is separate:

```bash
cd website && bun install && bun run build
```

## Changing a detector

Anti-pattern detectors live **only** in the Rust engine
(`engine/crates/engine-bin/src/antipatterns.rs`). There is no Go implementation, and
adding one is forbidden — see ADR-011. Two implementations disagreed silently for a
release, and which one ran depended on whether the embedded binary resolved.

`detectorSeverities` in `internal/engineclient/severity.go` is the single source of truth
for severity: the engine stamps every finding with the requesting rule's severity, so a
detector without an entry there is reported as a warning whatever its real severity.
`TestDetectorSeverityCoversEveryEngineDetector` fails if you add an engine detector and
forget it.

After changing the engine, rebuild and re-embed it with `mise run build-engine` —
otherwise you are still testing the previously embedded binary.

`verikt check` requires the engine for Go projects. Without it you get a clear error, not
a partial answer. `verikt analyze` is different: architecture and convention detection are
Go-side (`internal/analyzer/detector/`) because no Rust equivalent exists.

## Architecture decisions and invariants

`docs/decisions/` holds ADRs, `docs/invariants/` holds constraints that must not be
violated. Both are compiled into `.claude/rules/governance.md`, which is generated —
do not edit it by hand.

Read the invariants before changing analysis behaviour. Two are easy to break by
accident:

- **INV-002** — directory walks must skip symlinks, in Go *and* in Rust. `path.is_dir()`
  follows symlinks, so the entry itself has to be tested.
- **INV-004** — detectors never see test files. Test code legitimately does things that
  are defects in production code.

## Commits

Conventional commits, with `!` and a `BREAKING CHANGE:` trailer for anything that
changes output shape, exit codes, or configuration semantics. Release notes are
generated from commit messages, so explain *why* the change was needed — a reader six
months from now needs the reasoning, not a restatement of the diff.

Add an entry to `CHANGELOG.md` under `[Unreleased]` for anything user-visible.
