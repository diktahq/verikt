# Changelog

All notable changes to verikt are documented here. Format follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased] — 0.3.0

Ten issues, #8 to #18. Most are cases where verikt reported success for something
it had not done: an installer that skipped verification when it could not verify, a
hook that never reported a violation, scaffolds that failed their own
`verikt check`. Read *Upgrade notes* before upgrading; several change what an
existing pipeline sees.

### Upgrade notes

- **An unavailable engine stops `verikt check`.** If the embedded analysis engine
  cannot be extracted — an unwritable cache directory, for example — the check now
  fails with `analysis engine unavailable`, followed by the underlying cause, instead
  of a generic message. Pass `--allow-degraded` to skip the built-in detectors (with a
  warning on stderr) and check only proxy rules and decision gates. It is refused with
  `--detectors`, which would leave nothing to check.
- **`--diff` and `--staged` now include renamed files.** They used git's `ACM` filter,
  so every finding in a file the branch had renamed — including renamed and then
  edited — was filtered away. Findings that were hidden this way will now appear, and a
  pipeline gating on `--diff main` can newly fail without any change to your code.
- **Two new structure detectors, both at warning severity**, so neither changes the
  exit code unless you raise it. `live_dot_claude_fixture` is always on: it reports a
  `.claude/` directory holding `skills`, `rules`, `hooks` or `settings.json` under a
  `test`, `tests`, `testdata`, `fixtures`, `evals` or `examples` path, where an agent
  can load it as live instructions. `cmd_allowlist` (Go) is off until you set
  `check.allowed_entrypoints`; it then reports any `package main` under `cmd/` or at
  the module root that the list does not name. Waive a specific finding with a
  `severity_overrides` entry under the detector's name (`severity: ignore`, a
  `reason`, and `paths`), or make one blocking with `severity: must`.
- **Cursor's rules moved to `.cursor/rules/verikt.mdc`.** Cursor deprecated
  `.cursorrules`. The next `verikt guide` moves verikt's section out of `.cursorrules`,
  keeps your own content there, and deletes the file only if verikt's section was all
  it held. Commit the new file.
- **Commit `AGENTS.md` and `.verikt/references/`.** `verikt guide` can now write a
  section of `AGENTS.md` (`--target agents`), and every target's always-loaded file
  now points to on-demand references under `.verikt/references/`. Without that
  directory in the repository, the agent is sent to files that don't exist.
- **Projects scaffolded with an earlier version keep their old components.**
  `verikt new` used to write hexagonal components into `verikt.yaml` for every
  non-flat architecture, so clean and layered projects failed their own check, and
  `platform/` packages were orphans in every architecture. Regenerating is not needed;
  copy `components`, `rules.structure.required_dirs` and `check` from a fresh
  `verikt new --arch <your arch>` into your `verikt.yaml`. The new `app` component
  classifies `cmd/` and `internal/bootstrap/`, so `check.exclude: [cmd/**]` is no
  longer needed and can go.
- **No Windows archives.** The Windows build shipped without the engine, so
  `verikt check` could only fail there. Releases are macOS and Linux, amd64 and arm64.
- **Installing no longer runs `verikt setup`.** Neither the Homebrew formula nor the
  install script registers verikt with your agents any more. Run `verikt setup`
  yourself, or pass `--setup` to the install script.

### Added

- **`AGENTS.md` target** (`verikt guide --target agents`), for Codex, Antigravity and
  other agents that read it. verikt writes only the section between its markers. (#13)
- **Two-tier guide output.** Always-loaded files hold compact writer rules; layer
  detail, the capability catalog, the review checklist and (with `--catalog-only`) the
  scaffold interview move to `.verikt/references/`. `TestGuideOutput_InstructionCount`
  (at most 30) and `TestGuideOutput_TokenBudget` (at most 1,500) enforce INV-001 on every
  target, replacing the 3,100-token exception the combined file needed. (#13)
- **`verikt setup --codex` and `--antigravity`.** Opt-in: verikt writes a marked
  section of `~/.codex/AGENTS.md` or `~/.gemini/GEMINI.md` and leaves the rest of the
  file alone. These locations have not yet been checked against the vendors'
  documentation (#19). (#13)
- **`verikt check --allow-degraded`.** (#9)
- **`optional: true` on a component** skips the missing-component check for it. The
  guide labels optional components. (#18)
- **`platform` and `app` components** in every scaffolded Go architecture except flat:
  `platform` for capability-written cross-cutting code (optional; depends only on the
  innermost layer), `app` for the composition root. (#18)
- **`cmd_allowlist` and `live_dot_claude_fixture` structure detectors**, at warning
  severity. (#15)
- **Build-provenance attestation step** in the GitHub Actions release workflow, for
  every archive in `checksums.txt`. It only runs when that workflow builds the release;
  GitHub Actions does not currently run for this repository, so a release built
  locally carries no attestation. Verify one with
  `gh attestation verify <archive> --repo diktahq/verikt`. (#11)
- **Local enforcement for contributors.** GitHub Actions does not run for this
  repository, so nothing checks a change after it is pushed. `lefthook` runs gofmt,
  golangci-lint and race tests on staged packages at pre-commit, and `mise run verify`
  at pre-push; `mise run verify` adds `go mod tidy -diff`, `go test -race`, shellcheck
  and the installer tests. golangci-lint is 2.15.0, with gosec, errorlint, nilerr and
  exhaustive enabled. See `CONTRIBUTING.md`. (#12)

### Fixed

- **The install script downloads an archive that exists.** It requested
  `verikt_<os>_<arch>.tar.gz`; releases publish `verikt_<version>_<os>_<arch>.tar.gz`.
  `latest` is now resolved to a concrete tag first. (#8)
- **The install script fails closed.** A missing checksum entry, a mismatched hash, or
  no `sha256sum`/`shasum` on the machine each stop the install. The old script
  installed with a warning when the entry was missing, and when no hash tool existed
  it set the computed hash to the expected one, so the comparison always passed. Every download uses `curl -f`, and `VERIKT_VERSION`
  must be a semver tag. (#8)
- **The engine error names its cause.** The reason extraction failed was discarded, so
  the user saw only that the engine was required. (#9)
- **`--diff` rejects an empty ref or one starting with `-`**, which git would have read
  as an option. (#9)
- **The Claude Code `PostToolUse` hook reports violations.** `verikt-check.sh` ran the
  check with `|| true`, which forced the exit code to 0, so it never reported a
  violation. It now counts violations, anti-patterns, proxy-rule violations and
  decision violations, and reports an unreadable failure as unverified rather than as
  a pass. (#10)
- **The guide-refresh hook works on macOS**, using `sha256sum` or `shasum -a 256`,
  whichever exists. (#10)
- **`.claude/settings.json` is merged safely.** verikt refuses to write a file that
  is not valid JSON or whose `hooks` has the wrong type, updates its own entries in
  place, keeps yours, and writes atomically, preserving the file mode. (#10)
- **A cached engine is verified before it runs.** It is used only if it is a regular
  file (not a symlink) of the right size, executable, and matching the embedded
  SHA-256; anything else is extracted again. Cache directories are `0700`, and a
  symlinked cache directory is refused. (#11)
- **The capability matrix covers all 65 Go capabilities and 4 architectures.**
  `matrix.go`, which feeds the generated `docs/PROJECT.md`, covered 21, so the rest
  were silently left out. Parity tests tie the matrix, the reference doc and the
  website to the templates. Published counts went from 63 to 65. (#14)
- **`dynamodb`, `feature-flags`, `multi-tenancy` and `s3` scaffold again.** Each
  required a `config` capability that does not exist, so `verikt new` failed for all
  four. `mongodb` and `sqlite` suggested it too; they now suggest `platform`. The
  TypeScript `http-api` suggested a nonexistent `validation`. A test now rejects any
  dangling `requires`, `suggests` or `conflicts` entry. (#16)
- **Scaffolded projects pass their own `verikt check`.** `verikt.yaml` takes its
  components from the chosen architecture's manifest. Empty clean and layered layers
  get a `doc.go` instead of `.gitkeep`, so the empty-layer warning stays quiet on a
  fresh scaffold. Adapters with fixed package names get their own subdirectory in
  clean, layered and flat, instead of putting two packages in one folder. Flat
  projects with `testing` or `saga` no longer fail with `mkdirat: empty path`. Covered
  by `TestScaffoldPassesCheck` across four architectures. (#18)
- **Shared guide files with malformed markers fail closed.** Copilot and Windsurf
  files whose verikt markers were broken got a second section appended; verikt now
  refuses to write them and says why. (#13)
- **A feature-matrix or version-detection error no longer scaffolds with every
  feature flag silently off.** (#12)
- **Engine request frames are bounded at 64 MiB**, and `rules.functions` limits
  outside the `int32` range are rejected. (#12)

### Changed

- **Scaffolded Go code and verikt itself use `go.yaml.in/yaml/v3`** instead of
  `gopkg.in/yaml.v3`. (#12)

### Removed

- **Windows release archives.** (#11)
- **`verikt setup` from the Homebrew formula's install and from the install script's
  default run.** The formula prints a reminder instead; the script runs it only with
  `--setup`. (#8, #11)
- **`.cursorrules` as the Cursor output**, replaced by `.cursor/rules/verikt.mdc`. (#13)

## [0.2.2] — 2026-08-15

Found by dogfooding on a real project. Three false positives, all at error
severity, so each failed a build — and none could be fixed in the reporting
project's code.

### Changed

- **`sql_concatenation` is warning severity, not error.** It reads string
  literals, so it cannot know a match is SQL. Heuristic detectors now ship at
  warning and earn error severity by demonstrating precision on real code
  (INV-005); raise it with `severity_overrides` if you want it blocking.

### Fixed

- **`sql_concatenation` no longer fires on English prose.** It uppercased every
  concatenated string and matched `"SELECT "`, `"DELETE "`, `"FROM "` as
  substrings, so a variable named `where` and a sentence containing "delete the
  block" produced two findings in a module with no database, no driver and no
  query. A finding now requires evidence the file deals with SQL: a database
  import, a query-executing call, or a literal written as SQL conventionally is
  — upper-case keywords in statement order. `"SELECT * FROM orders WHERE id="`
  is a query; `"select the rows from "` is a sentence.
- **`nil_map_write` no longer fires when the map's address is taken.**
  `json.Unmarshal(data, &cfg)` allocates the map, so the write that follows is
  safe — it was reported at error severity, and the suggested remedy would have
  been a redundant `make()`.
- **`sql_concatenation` judges the expression, not the file.** The evidence check
  was file-scoped, so one genuine query literal held as data made every unrelated
  concatenation in that file suspect: an audit tool reported
  `id + ": absent from canonical inventory"` as SQL injection. Findings across the
  corpus fell from 17 to 2, and both survivors are in a real database file.
- **Nested modules are no longer analysed as part of the parent.** A directory
  with its own `go.mod` is a different project, and its packages belong to that
  module's import path — but the walk went through the boundary and reported
  them as orphan packages under a path derived from the parent module, which
  does not exist. Config lookup already stopped there; the analysis did not.
- **`PROXY RULES (0 valid, 0 invalid, 0 stale) ✓ All proxy rules pass`** printed
  a green tick for checks that do not exist. A test asserted that tick.

### Added

- **Generated files are not analysed.** A file carrying Go's
  `// Code generated ... DO NOT EDIT.` marker before its package clause is
  skipped, as every Go analysis tool does. Across seven real repositories, 61% of
  all findings — 76 of 126 — were in generated code, almost all mockery mocks,
  and regenerating restores every one of them.
- **`scripts/corpus-audit.sh`** runs the detectors across real repositories and
  reports every finding with the line that produced it, flagging any detector
  whose findings cluster in one directory. Repositories are copied and the copy
  analysed; the originals are never written to (ADR-012).
- **`verikt false-positive`** produces a bug report for a finding that should not
  have fired, including the code it fired on. A false positive is a defect, not
  something to waive: waiving hides it for you and leaves it for everyone with
  the same shape of code. One project reworded English error messages to stop a
  detector firing.
- **INV-005 — a detector establishes evidence, not shape.** All three false
  positives shared a structure: the detector matched a shape and never asked
  whether the risk it names was reachable.
- **The restraint fixture is seeded from real code.** The invented version caught
  none of these three. A fixture written from imagination only covers the false
  positives its author already pictured.

## [0.2.1] — 2026-08-15

A false-positive fix. No configuration change is required, and nothing that
passed 0.2.0 fails here — `verikt check` reports strictly less.

### Fixed

- **`global_mutable_state` no longer reports lookup tables.** An unexported
  package-level variable that nothing writes to is not global mutable state. Go
  has no const map or const slice, so `var x = map[K]V{...}` is the only way to
  express a table — reporting it named a construct the language requires and
  prescribed dependency injection, which makes a static table worse. On this
  repository the check fired 22 times and 21 were tables nothing ever wrote to.

  An exported variable is still reported whatever the file shows: another package
  can write to it and the analysis sees one file at a time. Mutation — assignment,
  index assignment, `delete`, or `append` assigned back — is still reported.

  The trade is deliberate. A table mutated from a sibling file in the same package
  is now missed. A detector that is wrong most of the time is worth less than one
  that occasionally misses, because the first teaches you to skim the section it
  appears in.

- **A dead exported `KnownDetectors` slice is removed** from `internal/rules`. It
  duplicated the detector registry, was read by nothing, and being exported left
  it writable by any importing package. It was the one true finding among that
  detector's 22, and deleting it was the fix.

### Added

- **A restraint fixture** (`internal/engineclient/testdata/detector-restraint`):
  code written the way a careful engineer writes it, where every detector must
  stay silent. Any finding fails the build.

  Detection was already guarded — a fixture asserts all 14 detectors fire — and
  restraint was not, so a detector could grow louder indefinitely and no test
  objected. That asymmetry is how a detector came to be wrong 21 times in 22.
  Verified by reverting the fix above: the gate fails with the false positives
  named.

## [0.2.0] — 2026-08-15

Fixes an external audit of 0.1.0 (8 findings, all reproduced) and the parity gaps that
surfaced while fixing them — cases where the Go and Rust implementations disagreed and
nothing detected it. Seven breaking changes; read *Upgrading* first.

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
