# PRD-002: Rust Language Support

_Status: Draft — design in progress_
_Author: Daniel Gomes_
_Date: 2026-08-14_

---

## Problem

Rust has no Rails, no Spring, no `create-next-app` for services. Every team re-decides the same set: axum or actix, sqlx or SeaORM or diesel, `thiserror` or `anyhow` and where the boundary between them sits, tokio runtime setup, tracing subscriber wiring, config layering, graceful shutdown, workspace layout. Individually small, collectively a week — and getting the error-type boundary wrong propagates through every function signature and is expensive to reverse.

Agents are worse at Rust service structure than at Go. They produce code that does not compile, misuse `async`, over-clone to escape the borrow checker, reach for `unwrap()`, and put `anyhow` in library APIs. A guide stating *this project's* conventions before the agent starts prevents most of that.

This is the half of verikt that needs no analysis work: `new`, `add`, `guide`. It is also the half verikt leads with.

## Insight

**Enforcement should follow the language's strengths.**

For Go, verikt declares boundaries and `check` polices them, because nothing else will. For Rust the compiler can police them: a cargo workspace whose `domain` crate lists no internal dependencies in its `Cargo.toml` makes "domain must not import adapters" a build failure — permanently, for every developer and every agent, with verikt out of the loop.

That is a stronger guarantee than any checker verikt could write, and it changes the deliverable. Rust support is primarily a **workspace shape**, not a detector set.

It also makes Rust checking cheap rather than expensive. If the architecture lives in crate boundaries, drift detection is reading `Cargo.toml` dependency lists — TOML parsing, no tree-sitter grammar, no per-language AST work. Anti-pattern detectors are additive and can come later.

## Proposal

### Architectures

The meaningful axis in Rust is crate topology, not directory naming.

| Architecture | Shape | Enforced by |
|---|---|---|
| `flat` | one crate, `src/main.rs` + modules | nothing — CLIs, tools, prototypes |
| `hexagonal` | workspace: `domain`, `ports`, `adapters`, `app` | **the compiler**, via `Cargo.toml` dependency lists |

`clean` is deliberately omitted: in Rust it collapses into hexagonal with different names. A single-crate `layered` variant using `pub(crate)` for boundaries is a candidate for v2 — cheaper to scaffold, weaker to enforce.

### Capabilities — 12 for v1

Go ships 65 and TypeScript 39. Matching either is not a goal; covering one real service is.

| Capability | Crates | Rationale |
|---|---|---|
| `http-api` | axum, tower, tower-http | timeout / cors / trace as tower layers |
| `postgres` | sqlx | `query!` macros give compile-time checked SQL |
| `migrations` | sqlx migrate | pairs with postgres |
| `platform` | figment, tracing-subscriber, tokio signal | config + logging init + graceful shutdown |
| `observability` | tracing, opentelemetry-otlp | separate from platform, as in Go |
| `health` | axum routes | `/healthz`, `/readyz` with pluggable checks |
| `worker` | tokio, JoinSet | shutdown-aware task pool |
| `retry` | backoff | |
| `circuit-breaker` | template-owned | no dominant crate — verikt writes the implementation |
| `testing` | tokio-test, testcontainers | the integration harness is the real win |
| `linting` | clippy.toml, `[workspace.lints]` | second place to encode architecture as a build failure |
| `docker` | multi-stage + cargo-chef | biggest non-obvious win in Rust build times |

Deferred until requested: `grpc` (tonic), `redis`, `event-bus`, `outbox`, `saga`, `graphql`.

### Three decisions that are structural, not optional

These belong in every architecture rather than behind a capability flag, because they are where agents and teams most often go wrong and because reversing them later touches everything:

1. **Error strategy** — `thiserror` for domain and library errors; `anyhow` only in the binary.
2. **Workspace lints** — `[workspace.lints.clippy]` plus `#![forbid(unsafe_code)]`.
3. **MSRV and edition in the feature matrix** — ADR-007's version-gated features map directly: edition 2024 needs 1.85, `async fn` in traits needs 1.75. The engine crate already requires 1.85.

### What `verikt check` promises for Rust in v1

Crate-graph drift only: parse each crate's `Cargo.toml`, compare its internal dependencies against the declared components, report violations. No tree-sitter grammar, no AST work.

Anti-pattern detectors — `unwrap()`/`expect()` in library code, `panic!` on process edges, blocking I/O inside `async fn`, `Arc<Mutex<>>` as a design smell, `unsafe` blocks — are a v2 tree-sitter set.

This asymmetry must be published, not implied. verikt currently suggests uniform support per language while TypeScript's `check` depth is far below Go's and nothing documents that. **A per-language support matrix (command × language × depth) is a prerequisite of this PRD**, not a follow-up.

## Non-goals

- Matching Go's capability count
- Anti-pattern detection for Rust in v1
- A `clean` architecture variant
- Porting verikt's Go-side architecture and convention detection to Rust (see #3)

## Cost

The Go-side work is small and precedented: the `LanguageProvider` interface is seven methods, and the TypeScript provider is 267 lines plus its template tree. The cost is templates and CI:

- ~12 capability directories on the three-file convention (manifest, wizard, files)
- 2 architecture template sets
- a feature matrix keyed on Rust version
- **a new CI job** — `template-matrix` builds scaffolded Go output across five Go versions; Rust needs the cargo equivalent across an MSRV range, and it will be the slowest job in CI

## Sequencing

1. After 0.2.0 ships.
2. Blocked on #3 — whether the Go analysis fallback is deleted determines whether this provider ships a checker at all.
3. Publish the per-language support matrix.
4. Spike `flat` + `http-api` + `postgres` to validate the provider interface against a crate-based language before writing the full template set.
5. Then the remaining capabilities and `hexagonal`.

## Open questions

1. **Does `hexagonal` default to a workspace?** A workspace is the whole point, but heavier than newcomers expect for a small service. Recommendation: yes, and add single-crate `layered` in v2 for the lighter case.
2. **sqlx or SeaORM?** sqlx's compile-time checked queries fit the theme, but `query!` needs a reachable database at build time, which complicates CI for scaffolded output. Recommendation: sqlx with `query` (runtime-checked) in templates, `query!` documented as an opt-in — but this needs validating against the template-matrix job.
3. **Are Rust teams the customer?** The pitch is architecture drift, and Rust teams have the most compiler help against it. The value is concentrated in bootstrapping rather than enforcement. Worth validating against usage data before committing weeks of template work.

Settled unless challenged: axum over actix (momentum, tower ecosystem, and tower layers map cleanly onto capabilities).

## Risks

- **Capability-count expectation.** "65 Go / 39 TypeScript / 12 Rust" reads as unfinished. The support matrix has to be explicit about scope per language per command.
- **Ecosystem churn.** The Rust service ecosystem moves faster than Go's; these templates will need more maintenance per year than the Go set.
- **Detector-set fragmentation.** ADR-006 already warned that anti-pattern queries are per-language work. Each language multiplies that surface, and #3 is the bill for getting it wrong once.

## Success criteria

- `verikt new my-svc --language rust --arch hexagonal --cap http-api,postgres` produces a workspace that builds, runs, and serves a request with no manual edits
- Adding `adapters` as a dependency of the `domain` crate fails `cargo build` — the architecture is enforced without verikt
- `verikt check` detects that violation from `Cargo.toml` alone
- Scaffolded output builds across the supported MSRV range in CI
- A published support matrix states what `check`, `analyze` and `guide` deliver per language

## Why this is worth doing

Bootstrapping is a genuine, unsolved pain in Rust, and a harder problem than the Go equivalent — which makes it more valuable to template, not less. The compiler-enforced workspace is a claim that cannot be made for Go or TypeScript:

> verikt scaffolds your Rust workspace so the compiler enforces your architecture.

And once the provider exists, verikt can scaffold and check the shape of its own Rust engine.
