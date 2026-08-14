# ADR-011: Enforcement Follows the Language's Strengths

**Status:** accepted
**Date:** 2026-08-14
**Deciders:** Daniel Gomes
**Amends:** ADR-006 (Polyglot Architecture) — see *Amendment to ADR-006* below

## Context

verikt enforces declared architecture by analysing code: `verikt check` reads the import
graph and reports violations. That is the only enforcement mechanism the product has, and
it was designed for Go, where nothing else will do the job — the Go compiler happily
allows `domain` to import `adapters`.

Adding languages exposed that this is a Go-shaped assumption.

In Rust, the unit of encapsulation is the crate, and dependencies are declared in
`Cargo.toml`. A workspace whose `domain` crate lists no internal dependencies makes
"domain must not import adapters" a **compile error** — permanently, for every developer
and every agent, with verikt absent from the loop. Writing a checker to police a rule the
compiler already enforces would be redundant work that can only be weaker than the
compiler.

The same reasoning generalises. Enforcement strength differs per language:

| Mechanism | Strength | Available in |
|---|---|---|
| Compiler / build failure | Absolute — cannot be bypassed | Rust (crates), Java (modules), C# (assemblies) |
| Visibility rules | Strong within a compilation unit | Rust (`pub(crate)`), Go (`internal/`) |
| Static analysis at commit or CI time | Bypassable, but universal | every language |

Choosing the wrong mechanism has a real cost in both directions: a checker that duplicates
the compiler is wasted effort, and relying on a compiler that does not exist leaves the
rule unenforced.

## Decision

**Enforcement follows the language's strengths. For each language, verikt uses the
strongest mechanism that language offers, and static analysis only for what remains.**

Concretely:

- **Rust** — the architecture is enforced by **crate topology**. `verikt new` scaffolds a
  workspace whose `Cargo.toml` dependency lists encode the declared component graph.
  `verikt check` verifies the manifests still match the declaration; it does **not**
  re-derive the boundary from source.
- **Go, TypeScript, Python** — the architecture is enforced by **`verikt check`**, because
  no compile-time boundary exists. `internal/` and module visibility help but do not
  express component rules.

This makes `verikt new` an enforcement mechanism in its own right for some languages, not
only a convenience. Scaffolding that encodes a boundary the toolchain enforces is stronger
than any rule verikt could check afterwards.

### Consequence for checkers

A language whose boundaries are compiler-enforced gets a **deliberately smaller** checker,
and that is not a gap to be closed. For Rust, `check` reads `Cargo.toml` — TOML parsing,
no tree-sitter grammar. Anti-pattern detection remains valuable per language (`unwrap()` in
library code, `panic!` on process edges, blocking I/O inside `async fn`) because those are
not boundary questions.

NEVER add a source-analysing dependency checker for a language whose dependency graph is
already declared in a manifest the build enforces.

### Consequence for capability scope

Capability sets are per-language and are not expected to converge. Go ships 65 and
TypeScript 39 because those are what each ecosystem needs, not because one is incomplete.
A new language ships the capabilities that make one real service, and grows on demand.

## Amendment to ADR-006

ADR-006 states:

> **Rust owns:** Code parsing (tree-sitter), import graph analysis, architecture
> detection, anti-pattern checking, rule engine, structural queries.

Two corrections, reflecting what was actually built and what should be:

1. **Architecture and convention detection remain in Go.** They are implemented in
   `internal/analyzer/detector/` against `go/packages`, and no Rust equivalent exists.
   ADR-006 assigned them to Rust; that never happened, and porting them is not planned.
   `verikt analyze` is a Go-side capability.
2. **The boundary is syntax versus semantics, not Go versus Rust.** Syntactic analysis
   belongs in Rust via tree-sitter, for every language. Semantic analysis — questions
   needing type information, such as *does this type satisfy `port.Repository`?* — belongs
   in a per-language tool, and only where it buys a check that cannot otherwise be written.
   `go/packages` gives type information tree-sitter cannot.

ADR-006's "clean cut — no fallback" instruction stands and is the subject of #3: the Go
duplicates of anti-pattern, dependency and metric analysis are to be deleted, because
Rust already implements them. That is distinct from the Go-only analysis above, which has
no Rust counterpart.

## Consequences

### Positive

- Rust support is cheaper than a full detector port, and its guarantee is stronger
- A future contributor cannot "complete" Rust by writing a redundant source checker —
  this ADR says not to
- Capability and checker asymmetry between languages becomes intentional and explainable
  rather than looking like unfinished work

### Negative

- Enforcement strength varies by language, so users must be told what they get per
  language. A published support matrix is required, not optional.
- Two enforcement models to explain in documentation
- A Rust project that abandons the scaffolded workspace shape (single crate, modules only)
  loses compiler enforcement and gets no checker in return. That case needs the
  single-crate `layered` architecture and a module-level checker before it can be
  supported.
