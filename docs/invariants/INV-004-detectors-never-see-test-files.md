# Invariant 004: Detectors Never See Test Files

**Status:** active
**Date:** 2026-08-14

## Constraint

`packages.Load` for detector analysis MUST keep `Tests` disabled
(`internal/analyzer/analyzer.go`). Test files MUST NOT enter `pkg.Syntax`.

Statistics that genuinely require test files — the test-file count, subtest
detection, BDD library detection — MUST be gathered separately by reading
`_test.go` files directly (`scanTestFiles` in
`internal/analyzer/detector/convention.go`).

A testing pattern MUST NOT be inferred when a project has zero test files.

## Rationale

Every anti-pattern detector runs over `pkg.Syntax`. Test code legitimately does
things that are defects in production code:

- `naked_goroutine` — test helpers start goroutines without lifecycle management
- `global_mutable_state` — shared fixtures and golden-file tables are package-level vars
- `context_background_in_handler` — tests construct `context.Background()` freely
- `god_package` — large table-driven test files export many helpers

Enabling `Tests: true` also changes the package set: go/packages synthesises test
variants (`pkg [pkg.test]`, `pkg_test`, and a generated main). Those duplicate
packages appear as extra nodes and edges in the dependency graph, double-count
exported symbols, and produce import paths that match no declared component.

The alternative — enabling `Tests` and then filtering test files back out inside
every detector — puts the same conditional in a dozen places and fails silently
the moment someone adds a detector without it.

## Consequences of Violation

- False positives across every anti-pattern detector, sourced from test code
- Inflated `god_package` symbol counts and duplicated dependency-graph nodes
- Orphan-package findings for synthesised `pkg.test` import paths
- Users lose trust in `verikt check` output and start ignoring it

## Enforcement

- `internal/analyzer/analyzer.go` sets an explicit `Mode` without `Tests: true`
- `TestDetectTestingSeesTestFiles` asserts test files are still counted (1/5 on
  the hexagonal fixture) despite `Tests` being disabled — it fails if the
  separate scan is removed
- `TestDetectTestingIgnoresRunCallsOutsideTests` asserts a project with zero test
  files reports `minimal` with no evidence, so a testing pattern cannot be
  inferred from production code

## Violations

- Setting `Tests: true` in any `packages.Config` used for detector analysis
- Counting `.Run(` calls, BDD imports, or test files from `pkg.Syntax`
- Reporting a testing pattern other than `minimal` when `TestFiles == 0`
- Including `_test.go` files in the Rust engine's `collect_go_files`

## Applies to the engine too

The Rust engine must reach the same conclusion as the Go implementation. It
walks the filesystem directly rather than using go/packages, so the exclusion is
explicit in `collect_go_files` (`engine/crates/engine-bin/src/import_graph.rs`).

Including test files there produced dependency violations for imports that exist
only in tests: `verikt check` on this repository reported 14 such findings, every
one of them a test crossing a layer boundary in order to exercise it. A test
importing across a boundary is not an architecture violation.
