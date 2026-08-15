# Language Support Matrix

verikt does not offer the same depth in every language, and the differences are
deliberate. This page states what each language actually gets, per command.

Two reasons the depth differs:

1. **Enforcement follows the language's strengths** (ADR-011). Where a compiler can enforce
   a boundary, verikt encodes it at scaffold time instead of checking for it afterwards. A
   smaller checker there is by design, not a gap.
2. **Analysis is per-language work.** Every detector needs a grammar and queries for that
   language. Capability templates and analysis depth grow independently.

## What each language gets

| | Go | TypeScript |
|---|---|---|
| `verikt new` — scaffold | ✅ 4 architectures, 65 capabilities | ✅ 2 architectures, 39 capabilities |
| `verikt add` — add capability | ✅ | ✅ |
| `verikt guide` — agent context | ✅ | ✅ |
| `verikt check` — dependency rules | ✅ import graph | ✅ import graph |
| `verikt check` — structure & coverage | ✅ | ✅ |
| `verikt check` — anti-pattern detectors | ✅ 14 detectors | ❌ none |
| `verikt check` — function metrics | ✅ lines, params, returns | ❌ none |
| `verikt analyze` — architecture detection | ✅ | ❌ none |
| `verikt analyze` — convention detection | ✅ | ❌ none |

Architectures: Go has `hexagonal`, `layered`, `clean`, `flat`. TypeScript has `hexagonal`,
`flat`.

## What this means in practice

**Go** is complete. Scaffolding, agent context, dependency enforcement, anti-patterns,
metrics and codebase analysis all work.

**TypeScript** is complete for the generative commands — `new`, `add` and `guide` are on
par with Go. For verification it covers dependency rules, structure and component
coverage, and nothing else: no anti-pattern detection, no function metrics, and
`verikt analyze` does not support TypeScript projects at all.

If you run `verikt check` on a TypeScript project and see fewer findings than you expect
on a Go project of similar quality, that is why. It is not that the project is cleaner.

## Roadmap

Depth is added where it is missing rather than uniformly:

- **TypeScript** — anti-pattern detectors and function metrics are the gap. Both need
  tree-sitter queries for TypeScript in the engine.
- **Rust** — planned (PRD-002), scaffolding first. Its `check` will be intentionally
  narrow: the architecture is enforced by cargo workspace boundaries at build time, so
  verikt verifies the manifests rather than re-deriving boundaries from source (ADR-011).
- **Python** — a candidate for the checking half, where no compile-time boundary exists and
  static analysis carries the whole load.

## Adding a language

A language needs two independent pieces of work:

1. **A provider** (`providers/<lang>/`) — templates, capability manifests, feature matrix.
   This delivers `new`, `add` and `guide`. No analysis work required.
2. **Engine support** (`engine/crates/engine-bin/`) — a tree-sitter grammar plus queries per
   analysis type. This delivers the `check` depth, incrementally.

A language can ship with (1) alone. This matrix exists so that when it does, users can see
exactly what they are getting.
