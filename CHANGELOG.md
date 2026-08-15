# Changelog

All notable changes to verikt are documented here. Format follows [Keep a Changelog](https://keepachangelog.com/).

## [0.2.0] — 2026-08-15

Fixes an external audit of 0.1.0 (8 findings, all reproduced) and the parity gaps that
surfaced while fixing them — cases where the Go and Rust implementations disagreed and
nothing detected it. Six breaking changes; read *Upgrading* first.

### Upgrading from 0.1.0

**`verikt check` now requires the engine.** For Go projects it fails with
`analysis engine unavailable` where it previously fell back to a second, divergent
implementation. Released binaries embed the engine, so this affects builds from source
that skipped the engine build — see `CONTRIBUTING.md`.

Two further changes move the exit code in **opposite** directions, so a pipeline can newly
pass or newly fail without any change to your code:

- **Warnings no longer fail `verikt check`.** Only error severity does. If you relied on
  exit 1 for warning-level findings, count them from the JSON:
  `jq '[(.violations // [])[], (.anti_patterns // [])[], (.proxy_rules.violations // [])[]] | map(select(.severity!="error")) | length'`.
  To *gate*, use `jq -e '.result == "pass"'` — selecting on severity across the two
  top-level arrays misses proxy-rule violations, stale rules and decision gates, all
  of which fail the check.
- **Error-severity anti-patterns now fail again on the default path.** `sql_concatenation`,
  `swallowed_error` and `domain_imports_adapter` were reported as warnings whenever the
  embedded engine resolved. A repository containing them will start failing. These are
  real findings — SQL injection, discarded errors, inverted dependencies — so fix them, or
  waive them with `severity_overrides` in `verikt.yaml`, which now covers anti-patterns
  and requires a reason.
- **Stale proxy rules now fail.** A rule whose `scope` matched no files never ran, so
  reporting it as a pass hid broken rules. A rule that ran across its scope and found
  nothing is passing — that case was briefly reported as stale during development and is
  covered by a Go/Rust parity test.
- **A path verikt cannot read now fails the check.** Unreadable directories and source
  files are reported as `unreadable_path` at error severity: the tool cannot vouch for what
  it did not read, and reporting nothing was indistinguishable from reading it and finding
  nothing. If a path in your tree is legitimately unreadable, waive it with a reason in
  `severity_overrides`, or exclude it with `check.exclude`.
- **A failing engine no longer falls back to the Go grep implementation** for proxy rules.
  Where the engine is present but errors, `verikt check` now fails instead of quietly
  producing findings from a second implementation. Where no engine exists for your platform
  at all, the Go implementation still runs and the output says so.
- **`max_lines` counts differently, so which functions are flagged changes.** A function
  with exactly `max_lines` lines in its body was previously reported as one line over and
  failed; it now passes. Expect slightly *fewer* function-length findings, and if you
  ratchet a debt count, re-measure before comparing.

Three need a config or tooling change:

- **JSON consumers:** `anti_patterns[]` keys are lowercase (`severity`, not `Severity`).
  The document carries `schema_version: 2`; check it before parsing. `violations[]`,
  `anti_patterns[]` and the new `waived[]` are always present and always arrays.
- **Test fixtures and generated code:** the engine no longer analyses `testdata/`,
  matching the Go toolchain. If you keep analysable code there, pass that directory as the
  project root. For generated code, use `check.exclude` — it now filters every finding
  category, not just orphan packages.
- **Monorepos:** `verikt.yaml` is no longer inherited across a directory with its own
  `go.mod` or `package.json`. Nested modules need their own config.

Contributors: the website builds with Bun (`bun install` in `website/`), and toolchains
are pinned in `.mise.toml` — run `mise install`. See `CONTRIBUTING.md`.

### Changed — breaking
- **`verikt check` requires the embedded Rust engine for Go projects.** The Go duplicates of the engine's analysis are deleted (~960 lines). ADR-006 specified the Rust engine as a "clean replacement of `go/ast` analysis (no fallback — clean cut)"; the migration never removed the Go side, so both shipped and disagreed silently — most seriously by downgrading a SQL-injection finding to a warning. Where the engine is unavailable, `check` now fails with a clear error instead of producing different findings. `verikt analyze` is unaffected: its architecture and convention detection are Go-side and have no Rust equivalent (ADR-011).
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
- **The exit code matches the documentation.** `--help` promised "error-severity violations" while the gate counted warnings: one warning-level `god_package` made exit 0 unreachable.
- **`--output json` returns the exit code it reports.** The JSON branch returned as soon as it had printed, so a document reading `"result": "fail"` still exited 0 — and every CI example writes JSON to a file, so a pipeline gating on the exit code passed on exactly the runs that found something.
- **`violations[]` and `anti_patterns[]` are always present in the JSON output.** They carried `omitempty`, so a passing project emitted neither key and the gate published in these notes died with `jq: Cannot iterate over null` — on precisely the projects that were clean. verikt's own debt ratchet had the same defect.
- **`severity_overrides` applies to anti-patterns.** They were the one exempt category, while these notes told users to scope newly-failing findings with exactly that key. A waiver requires a reason, and waived findings are still reported — in a `WAIVED` section and the `waived[]` array — so an accepted decision is visible without blocking, rather than silently absent.
- **`orphan_package` findings carry a project-relative path.** `file` held the full import path, so no path-scoped feature could ever match one: `check.exclude`, `severity_overrides` paths, `--staged` and `--diff` all silently skipped them.
- **`check.exclude` matches whole path segments.** `gen/**` compiled to `strings.Contains(path, "gen")`, so excluding generated code also dropped every finding under `internal/agent` — SQL injection included — and still printed "All checks pass". Use `**/testdata/**` for "at any depth"; a bare `testdata/**` is anchored at the project root.
- **`domain_imports_adapter` matches below the module root.** It searched raw import paths for `/infra`, `/adapter` and similar, so every first-party import in a module named `github.com/acme/infra-tools` was an error-severity finding, and a third-party `handler-kit` counted as the project's handler layer.
- **`nil_map_write` tracks allocation per function.** One `m := make(map…)` anywhere in a file suppressed every nil-map bug elsewhere in it, and `m`, `result` and `cache` are exactly the names that repeat. Grouped declarations (`var a, b map[string]int`) now register every name.
- **`type_assertion_without_ok` counts both sides of an assignment.** `s, n = v.(string), g()` has two targets but is not the comma-ok form, and was treated as safe.
- **TypeScript excludes test files** (INV-004), which only the Go path had received: `src/domain/user.test.ts` importing `../infrastructure/db` failed the check. The `--staged` file list applied no exclusions at all.
- **A TypeScript engine failure is no longer reported as a pass.** The error was discarded, so on the one language where the engine is optional a crash was indistinguishable from a clean project.
- **`--rule` with an unknown ID fails instead of passing.** A typo, or a rule since renamed or deleted, filtered every rule away and then printed "All proxy rules pass" and exited 0 — a pipeline pinned to it would keep passing while enforcing nothing.
- **Proxy grep rules skip `testdata/` and `_`-prefixed directories**, as the Go and TypeScript collectors already did. The same fixture directory was invisible to every detector and visible to every proxy rule, so a fixture holding a deliberate `fmt.Sprintf("SELECT ...")` failed the build at error severity.
- **The embedded engine is extracted atomically.** It was written straight to its final path while the existence check treated any file as ready, so a concurrent process could exec a truncated binary. The content-addressed cache makes this the common case rather than a rare one: every engine change forces a fresh extraction, and `go test ./...` runs the packages that need it in parallel.
- **`max_lines` counts the lines in a function body.** The body node spans the opening brace row to the closing brace row and one of those rows was counted as code, so a function with exactly 50 lines in it was reported as 51 and failed a limit it met. `max_lines: 50` now means a body may be up to 50 lines, and the number in the message is the number a reader counts. This removed one false finding from verikt's own debt baseline (69 → 68).
- **`max_return_values` counts return values, not commas.** A single return value whose type contained commas was counted once per comma: `func() func(a, b, c int, d, e string) error` was reported as 5 return values, and `Result[string, error]` as 2. The check enumerated the type kinds it recognised and counted commas in the text for anything else, which could never be complete; a multi-value return is the one case wrapped in a parameter list, and everything else is a single type.
- **A failing engine is no longer answered by the Go grep implementation.** `RunRules` caught any engine error and quietly ran the second implementation instead — the two had already been shown to disagree, so the findings a user saw depended on whether the engine happened to work, with nothing to say which had run. That is what `ErrEngineRequired` exists to prevent, and its own doc comment described this situation while proxy rules kept doing it. Where no engine exists at all the Go implementation still runs, and the run now says so.
- **An unreadable path is reported instead of silently emptying the result.** A permission error aborted the orphan-package walk and the caller then discarded every finding, so one unreadable directory anywhere under the project root turned that detector off completely and printed a clean tree. Unreadable paths are now error-severity findings (`unreadable_path`) from a dedicated walk that runs for every language and needs no `go.mod` — the engine's own walkers discard read errors silently, so nothing else was looking. Waivable with a reason like any other finding, and `check.exclude` is honoured. Source files are opened as well as directories walked: a file that exists and is listed but cannot be read is the one case a directory walk cannot see, because only the *read* fails — inside the engine, which discards it.
- **The engine's per-module summaries are combined.** Each module emits its own `CheckComplete` and the client kept only the last, despite the engine's own comment saying the client merged them. A request carrying grep rules alongside another rule type lost one module's `rule_statuses` entirely — and the proxy-rule path reads exactly that field.
- **`verikt check --diff <ref>` reports findings again.** The ref was passed after `--`, which makes git read it as a pathspec: it matched nothing, and every finding was then filtered away leaving exit 0 — so `--diff main`, documented as the way to adopt verikt on an existing codebase, passed unconditionally. The comparison is now against the merge base, and an unresolvable ref is an error rather than an empty file list.
- **`--staged` filters detector findings too**, not only proxy rules, so the pre-commit hook the command suggests no longer fails on unstaged code.
- **Flag combinations that run no checks are rejected.** `--detectors --rule x` skipped the detectors and the proxy rules and exited 0.
- **Waived proxy-rule findings are reported**, like every other waived finding, instead of vanishing behind "All proxy rules pass".
- **Glob patterns with `**` anywhere in them work.** `internal/**/testdata/**`, `src/**/*_test.go` and `internal/gen/**/*.go` silently matched nothing, as did a trailing slash and a bare directory name, so a `severity_overrides` waiver written that way quietly did nothing. Rule scopes and `verikt.yaml` now share one matcher, so the same pattern string means the same thing in both — and a scope with two `**` segments is no longer reported stale.
- **The Go grep fallback and the engine walk the same files.** The Go path skipped only `vendor` and `node_modules` while the engine also skipped `testdata`, `target` and `_`-prefixed directories, so the two produced different findings for identical input.
- **A malformed environment value is an error.** `HTTP_PORT=eighty-eighty` was silently ignored, leaving the service on whatever port the file named. Every bad value in one run is reported together, and every field the generated `config.yaml.example` documents is now actually read.
- **`mailpit` generates code that compiles.** Its bootstrap partial declared an email provider nothing consumed, so any project including it failed with "declared and not used". The provider is constructed where the application sends mail; the compilation matrix now covers `mailpit` and `i18n`.
- **Released binaries embed one engine, not four.** `//go:embed bin` is what lets a fresh clone compile before any engine exists, but the release workflow stages all four platform engines for a single parallel GoReleaser run, so every published artefact carried every engine — 17.4 MB per artefact of payload the target platform cannot execute.
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
- **The `platform` config layer reads the environment.** The generated loader read a committed YAML file and nothing else, so a project whose only real setting was a credential had two honest options: commit the secret, or stop using the config layer. Configuration now comes from defaults in `config.Default()`, then the file, then the environment, in that order — defaults are established before the file is read, so a value the file sets is never overwritten by a default it did not ask for. Credentials (`MYSQL_DSN`, `POSTGRES_URL`, `REDIS_PASSWORD`) are tagged `yaml:"-"` and the decoder runs with `KnownFields(true)`, so a secret written into the committed file is refused with the offending line rather than accepted silently. `config.yaml.example` no longer ships literal passwords. No new dependency: variables are read with `os.LookupEnv`, so an unset one leaves the file's value alone.
- **A `verify` target in the `makefile` capability** — build, test, lint and `verikt check` behind one command a developer and a CI pipeline can both invoke.
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
