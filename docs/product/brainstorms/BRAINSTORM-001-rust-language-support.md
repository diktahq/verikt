# BRAINSTORM-001: Rust Language Support

_Status: Brainstorm — nothing decided, open questions listed at the end_
_Author: Daniel Gomes_
_Date: 2026-08-14_
_Sequencing: after 0.2.0. Blocked on the decision in #3._

---

## Problem

Rust has no Rails, no Spring, no `create-next-app` for services. Every team re-decides
the same set: axum or actix, sqlx or SeaORM or diesel, `thiserror` or `anyhow` and
where the boundary sits, tokio runtime setup, tracing subscriber wiring, config
layering, graceful shutdown, workspace layout. The decisions are individually small and
collectively a week, and getting the error-type boundary wrong is expensive to undo
later.

Agents are measurably worse at Rust service structure than at Go. They produce code
that does not compile, misuse `async`, over-clone to escape the borrow checker, reach
for `unwrap()`, and put `anyhow` in library APIs. A guide that states *this project's*
conventions before the agent starts prevents most of that.

That is the half of verikt that needs no analysis work: `new`, `add`, `guide`. It is
also the half verikt leads with.

## Insight: enforcement follows the language's strengths

For Go, verikt declares boundaries and `check` polices them, because nothing else will.

For Rust, the **compiler** can police them. A cargo workspace where the `domain` crate's
`Cargo.toml` lists no internal dependencies makes "domain must not import adapters" a
build failure — permanently, for every developer and every agent, with no verikt in the
loop.

That is a stronger guarantee than any checker verikt could write, and it reframes what
Rust support means: the primary deliverable is a **workspace shape**, not a detector set.

It also makes Rust `check` cheap rather than expensive. If the architecture lives in
crate boundaries, drift detection is reading `Cargo.toml` dependency lists — TOML
parsing, no tree-sitter grammar, no per-language AST work. Anti-pattern detectors
(`unwrap()` in libraries, `panic!` on process edges, blocking inside `async`, `unsafe`)
are additive and can come later.

## Proposal sketch

### Architectures

The meaningful axis in Rust is crate topology, not directory naming.

| Architecture | Shape | Enforced by |
|---|---|---|
| `flat` | one crate, `src/main.rs` + modules | nothing — CLIs, tools, prototypes |
| `hexagonal` | workspace: `domain`, `ports`, `adapters`, `app` | **the compiler**, via `Cargo.toml` dependency lists |
| `layered` *(later)* | one crate, modules + `pub(crate)` | visibility — cheaper, weaker |

`hexagonal` is the flagship and the reason to do this at all. `clean` is deliberately
omitted: in Rust it collapses into hexagonal with different names.

### Capabilities — 12 for v1, not 65

| Capability | Crates | Note |
|---|---|---|
| `http-api` | axum, tower, tower-http | timeout / cors / trace as tower layers |
| `postgres` | sqlx | `query!` macros give compile-time checked SQL |
| `migrations` | sqlx migrate | pairs with postgres |
| `platform` | figment or config, tracing-subscriber, tokio signal | config + logging init + graceful shutdown |
| `observability` | tracing, opentelemetry-otlp | separate from platform, as in Go |
| `health` | axum routes | `/healthz`, `/readyz` with pluggable checks |
| `worker` | tokio, JoinSet | shutdown-aware task pool |
| `retry` | backoff | |
| `circuit-breaker` | template-owned | no dominant crate — verikt writes the implementation |
| `testing` | tokio-test, testcontainers | the integration harness is the real win |
| `linting` | clippy.toml, `[workspace.lints]` | second place to encode architecture as a build failure |
| `docker` | multi-stage + cargo-chef | biggest non-obvious DX win in Rust builds |

Deferred until asked for: `grpc` (tonic), `redis`, `event-bus`, `outbox`, `saga`,
`graphql`. Go ships 65 capabilities and TypeScript 39; matching either number is not a goal.

### Three things that are structural, not optional

These are where the Rust provider differs most from the Go one, and where agents and
teams most often go wrong. They belong in every architecture rather than behind a
capability flag:

1. **Error strategy.** `thiserror` for domain and library errors; `anyhow` only in the
   binary. Wrong choices here propagate through every signature and are painful to
   reverse.
2. **Workspace lints.** `[workspace.lints.clippy]` plus `#![forbid(unsafe_code)]`.
3. **MSRV and edition in the feature matrix.** ADR-007's version-gated features map
   directly: edition 2024 needs 1.85, `async fn` in traits needs 1.75. The engine crate
   already requires 1.85, so this is trodden ground.

## What this costs

The provider interface is seven methods (`internal/provider/provider.go`), and the
TypeScript provider is 267 lines plus its template tree — so the Go-side work is small
and well-precedented. The cost is in templates and CI:

- ~12 capability directories on the three-file convention (manifest, wizard, files)
- 2 architecture template sets
- a feature matrix keyed on Rust version
- **a new CI job**: `template-matrix` builds scaffolded Go output across five Go
  versions. Rust needs the cargo equivalent across an MSRV range, and it will be the
  slowest job in CI.

## Open questions

Nothing below is decided.

1. **axum or actix as the default?** axum has the momentum and the tower ecosystem;
   actix still wins some benchmarks. Ship one, or offer `--set HttpFramework` as
   TypeScript does for Express/Fastify/Hono?
2. **sqlx or SeaORM?** sqlx's compile-time checked queries fit the "compiler enforces
   it" theme, but require a reachable database at build time — which complicates CI for
   scaffolded output. Is that acceptable, or does it force SeaORM as the default?
3. **Does `hexagonal` default to a workspace?** A workspace is the whole point, but it
   is heavier than newcomers expect for a small service. Single-crate `layered` as the
   default and workspace as opt-in?
4. **Does the provider interface hold for a crate-based language?** `Scaffold` and
   `Analyze` assume a package-shaped project. A spike of `flat` + `http-api` +
   `postgres` would answer this before the full template set is written.
5. **What does `verikt check` promise for Rust in v1?** Crate-graph drift from
   `Cargo.toml` only, or also a tree-sitter detector set? Related: the per-language
   support matrix — today TypeScript's `check` depth is far below Go's and nothing
   documents that.
6. **Are Rust teams the customer?** The pitch is architecture drift. Rust teams have the
   most compiler help against drift, so the value is concentrated in bootstrapping
   rather than enforcement. Worth validating before spending weeks on templates.

## Risks

- **Capability-count expectation.** Users comparing "65 Go / 39 TypeScript / 12 Rust"
  may read Rust as unfinished. The support matrix has to be explicit about scope per
  language per command.
- **Ecosystem churn.** The Rust service ecosystem moves faster than Go's. Templates will
  need more maintenance per year than the Go set.
- **Detector-set fragmentation.** ADR-006 already warned that anti-pattern queries are
  per-language work. Each language added multiplies that surface, and #3 is the current
  bill for getting it wrong once.

## Why this is worth doing anyway

Bootstrapping is a genuine, unsolved pain in Rust, and it is a harder problem than the
Go equivalent — which makes it more valuable to template, not less. The compiler-enforced
workspace is a claim no competitor can make for Go or TypeScript:

> verikt scaffolds your Rust workspace so the compiler enforces your architecture.

And once the provider exists, verikt can scaffold and check the shape of its own Rust
engine. The dogfooding demo writes itself.
