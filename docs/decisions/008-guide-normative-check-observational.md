# ADR-008: Guide Is Normative, Check Is Observational

**Status:** accepted
**Date:** 2026-03-14
**Deciders:** Daniel Gomes

## Context

DT-006 (codebase mapping in guide) raised the question of whether `archway guide` should scan the filesystem to show agents what directories currently exist and how they map to architectural layers. EXP-06d validated that explicit directory-to-layer mapping helps agents produce better brownfield refactoring — zero violations vs four without the mapping.

The design question: should the mapping come from the config (static, derived from `archway.yaml` globs) or from the filesystem (dynamic, scanning actual directories at guide-generation time)?

## Decision

`archway guide` is normative — it describes architectural intent. `archway check` is observational — it reports what the code actually looks like. The guide does not scan the filesystem.

The codebase mapping in the guide is derived statically from `archway.yaml` component definitions. The existing `writeLayerRules` already emits glob-based component rules (`domain/** → domain layer`). DT-006 enhances this with a human-readable mapping table, but the data source is always the config, never the filesystem.

Unmapped directories — Go files that don't match any component — are the checker's domain, not the guide's. The guide references this with: "Run `archway check` to identify directories not covered by any component."

## Consequences

- Guide output is reproducible: same `archway.yaml` produces the same guide on any machine, branch, or CI environment
- Guide output is stable: adding files doesn't change the guide, only changing `archway.yaml` does
- Guide works correctly on empty projects (just scaffolded, no code yet) — it describes the target architecture
- Token budget (INV-001) stays bounded by component count, not project size
- The SessionStart hook that regenerates the guide on `archway.yaml` change remains correct — no need to watch for file changes
- If filesystem coverage information is ever needed in the guide, it ships as an opt-in flag (`--with-coverage`), never as the default

## Alternatives Considered

**Dynamic scanning (rejected):** Scan the filesystem at `archway guide` time, match files to components, report coverage. Rejected because: output varies by machine/branch (not reproducible), goes stale the moment a file is added (staleness inversion), token budget grows unboundedly with project size, useless on empty projects ("no directories found"), and duplicates what `archway check` already does with orphan detection.

**Hybrid — static default + dynamic opt-in (deferred):** Static mapping always present, `--with-coverage` adds a compact coverage table from filesystem scan. Not rejected — deferred until there is evidence that coverage in the guide (vs in `archway check`) provides value that justifies the complexity. EXP-06d validated static mapping, not dynamic.

## References

- EXP-06d: agent with explicit static mapping produced zero violations vs four without
- DT-006 design topic in Obsidian
- INV-001: rules file sizing constraints (500-1500 tokens)
