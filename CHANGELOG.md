# Changelog

All notable changes to verikt are documented here. Format follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Changed — breaking
- **`verikt check` requires the embedded Rust engine for Go projects.** The Go duplicates of the engine's analysis are deleted (~960 lines). ADR-006 specified the Rust engine as a "clean replacement of `go/ast` analysis (no fallback — clean cut)"; the migration never removed the Go side, so both shipped and disagreed silently — most seriously by downgrading a SQL-injection finding to a warning. Where the engine is unavailable, `check` now fails with a clear error instead of producing different findings. `verikt analyze` is unaffected: its architecture and convention detection are Go-side and have no Rust equivalent (ADR-011).

## [0.2.0] — 2026-08-14

Fixes an external audit of 0.1.0 (8 findings, all reproduced) and the parity gaps that
surfaced while fixing them — cases where the Go and Rust implementations disagreed and
nothing detected it. Six breaking changes; read *Upgrading* first.

### Upgrading from 0.1.0

Two changes move the exit code in **opposite** directions, so a pipeline can newly pass
or newly fail without any change to your code:

- **Warnings no longer fail `verikt check`.** Only error severity does. If you relied on
  exit 1 for warning-level findings, gate on the JSON instead: `jq -e '[.violations[], .anti_patterns[]] | map(select(.severity=="error")) | length == 0'`.
- **Error-severity anti-patterns now fail again on the default path.** `sql_concatenation`,
  `swallowed_error` and `domain_imports_adapter` were reported as warnings whenever the
  embedded engine resolved. A repository containing them will start failing. These are
  real findings — SQL injection, discarded errors, inverted dependencies — so fix them or
  scope them with `severity_overrides` in `verikt.yaml`.
- **Stale proxy rules now fail.** A rule whose `scope` matched no files was reported as
  passing. Together with the scope fix, rules that silently matched nothing will start
  matching — and failing if they find something.

Three need a config or tooling change:

- **JSON consumers:** `anti_patterns[]` keys are lowercase (`severity`, not `Severity`).
  The document carries `schema_version: 2`; check it before parsing.
- **Test fixtures and generated code:** the engine no longer analyses `testdata/`,
  matching the Go toolchain. If you keep analysable code there, pass that directory as the
  project root. For generated code, use `check.exclude` — it now filters every finding
  category, not just orphan packages.
- **Monorepos:** `verikt.yaml` is no longer inherited across a directory with its own
  `go.mod` or `package.json`. Nested modules need their own config.

Contributors: the website builds with Bun (`bun install` in `website/`), and toolchains
are pinned in `.mise.toml` — run `mise install`. See `CONTRIBUTING.md`.

### Changed — breaking
- **`anti_patterns[]` in `verikt check --output json` uses lowercase keys** (`name`, `category`, `severity`, `file`, `line`, `message`) instead of Go field names. `violations[]` already used lowercase, so consumers had to handle both spellings. The document now carries `schema_version: 2` — check it before parsing.
- **Anti-pattern severities are honoured on the Rust engine path.** The engine stamped every finding with the requesting rule's severity, so `sql_concatenation`, `swallowed_error` and `domain_imports_adapter` were warnings on the default path and errors on the Go fallback. They are errors again: a SQL-injection finding now fails CI where it previously did not.
- **A stale proxy rule fails `verikt check`.** A rule whose `scope` matched no files never ran, so reporting it as a pass hid broken rules. With the scope fix below, rules that silently matched nothing will start failing.
- **`verikt check` and `verikt guide` reject unknown capabilities.** A typo in the `capabilities:` list was accepted silently and rendered into the generated guide as though it were real.
- **Configuration lookup stops at project boundaries.** `verikt.yaml` is no longer inherited across a directory holding its own `go.mod` or `package.json`. A nested module needs its own config.
- **The docs site builds with [Bun](https://bun.sh).** `package-lock.json` is replaced by `bun.lock`; run `bun install` in `website/`.

### Fixed
- **`verikt guide` no longer destroys existing Claude Code hooks.** Each hook event was assigned outright, so a project with its own `PostToolUse` or `SessionStart` hook lost it on every run. Entries are merged and deduplicated, `settings.json` is only rewritten when it actually changes, and `--target all` now writes only the agents a project uses instead of leaving `.cursorrules`, `.windsurfrules` and a copilot file behind.
- **The engine skips `testdata` directories**, matching the Go toolchain. Fixtures contain anti-patterns deliberately, so the engine reported them as real findings while the Go path — which never sees testdata — did not.
- **`check.exclude` applies to every finding**, not only orphan-package detection. Anti-patterns, function metrics, structure and naming findings were reported for excluded paths regardless — including test fixtures, which contain the anti-patterns the detectors look for on purpose and which the Go toolchain excludes from builds.
- **Warnings are no longer printed with the failure marker.** A run that exits 0 could display dozens of apparent failures, because every finding used `✗` regardless of severity.
- **Proxy rules ran against nothing under the default `--path .`** — scope expansion matched the walk root against its own hidden-directory guard and skipped the whole tree, so every rule reported "scope matches 0 files". They only worked with an absolute `--path`.
- **Declared components are enforced again.** Dependency checks keyed `may_depend_on` on a guessed layer name rather than the declared component, so a component named `adapter` where the heuristic guesses `adapters` was exempt from all dependency enforcement (ADR-010).
- **The exit code matches the documentation.** `--help` promised "error-severity violations" while the gate counted warnings, including anti-patterns exempt from `severity_overrides`: one warning-level `god_package` made exit 0 unreachable with no waiver.
- **Symlinked directories are skipped in the Rust engine** (INV-002). None of its three walkers checked, and `path.is_dir()` follows symlinks, so it read files outside the project and could recurse forever on a cycle.
- **Test files no longer produce dependency violations.** The engine's import graph included `_test.go` files while the Go implementation excludes them, so a test crossing a layer boundary to exercise it was reported as a violation.
- **`verikt add --dry-run` exists.** It was documented in `--help` but never registered, so the documented example failed with `unknown flag`.
- **`verikt analyze` reports packages no component claims.** Only `check` knew about the condition, so `analyze` looked cleaner than the code was.
- **Testing conventions come from real test files.** `packages.Load` runs with `Tests` disabled, so no `_test.go` reached the detector and it reported "test files 0/N" for every project. Counting `.Run(` calls in production code also labelled projects with zero tests as `table-driven`.
- **`guessLayer` recognises `/core`**, matching `isDomainPackage`; projects naming their domain package `core` were reported as `unrecognized`.
- **The embedded engine cache is content-addressed.** It was keyed on a hand-maintained version constant, so changing the engine without bumping it left users running the previously extracted binary — silently, and indistinguishably from a successful upgrade. Stale directories are pruned.
- **`middleware.Recoverer` is the outermost middleware** in the scaffolded chi router. Registered seventh, it left tracing, logging and security headers outside its recover boundary, where a panic kills the process.
- **The `naked_goroutine` remedy names the panic consequence.** It recommended `errgroup`, which propagates panics rather than recovering them, so following the advice still crashed the process.
- **`verikt setup` honours `CLAUDE_CONFIG_DIR`**, so Claude Code profile aliases no longer detect and write to the wrong profile.

### Added
- **`nil_map_write` and `type_assertion_without_ok` detectors** — the two ways idiomatic-looking Go panics in production. Implemented in both Go and the Rust engine; `nil_map_write` is an error, `type_assertion_without_ok` a warning.
- **verikt governs itself** — a root `verikt.yaml` describing the real component structure, and a CI job that runs `verikt check` on this repository. It gates on error severity, and ratchets the warning-level count against `.github/verikt-debt-baseline` so debt cannot grow unnoticed.
- **`domain_imports_adapter` and `mvc_in_hexagonal` in the Rust engine.** Both were Go-only, and engine results replace the Go ones wholesale, so neither architecture detector ran for anyone whose engine resolved. Tests now fail when detector sets or severities diverge between the two implementations.
- **CI for the Rust engine** — `cargo fmt --check`, `cargo clippy -D warnings`, `cargo test`. It is the default analysis path and had none.
- **`.mise.toml`** pinning Go, Rust and Bun to the versions CI uses, with `build-engine` and `check-engine` tasks. The embedded binary is gitignored, so a fresh clone has nothing to embed until built.
- **INV-004** — detectors never see test files.
- A guard that fails when help text documents a flag that is not registered.

## [0.1.0] — 2026-03-23

First public release of verikt. Previously developed as "archway" — renamed to verikt as part of the dikta platform (diktahq).

### Added
- **Go language provider** — 63 capabilities across 10 categories (transport, data, resilience, patterns, security, observability, infrastructure, quality, platform, frontend). 4 architecture patterns: hexagonal, layered, clean, flat.
- **TypeScript/Node.js provider** — 39 capabilities across data, resilience, security, patterns, and infrastructure. Two architectures: hexagonal and flat. HTTP framework choice: Express, Fastify, or Hono.
- **Rust analysis engine** — tree-sitter-based import graph analysis for Go and TypeScript. Embedded via `//go:embed`, extracted to user cache on first run. Protobuf communication over stdin/stdout. Cross-compiled for darwin-arm64, darwin-amd64, linux-arm64, linux-amd64.
- `verikt new` with interactive wizard and `--no-wizard` mode.
- `verikt add` for adding capabilities to existing projects.
- `verikt check` with 11 AST-based detectors (dependency violations, anti-patterns, function metrics, structure checks). Powered by Rust engine import graph.
- `verikt guide` generating architecture context for Claude Code, Cursor, Copilot, and Windsurf. Includes governance checkpoint validated by EXP-10.
- `verikt init` — single onboarding entry point. Detects greenfield (empty directory → scaffold wizard) vs brownfield (existing code → analyze + map or bubble context).
- `verikt analyze` for detecting architecture patterns in existing codebases.
- `verikt setup` for registering with AI agents. Installs skills globally, locally, or both.
- `/verikt:init` skill for Claude Code with `[n/6]` progress indicators.
- Smart suggestions, capability warnings, feature-flag template engine, proxy rules, decision gates.
- **Drizzle ORM** as alternative to Prisma for TypeScript postgres and mysql capabilities.
- **Squirrel** query builder and **sqlc** capabilities for Go.
- Node.js version-gated features via `features.yaml`. Default Node version: 22 (Active LTS).
- Website with Starlight docs, 10 experiments, glossary, capability pages, architecture comparisons.
- `brew install diktahq/tap/verikt` distribution.
