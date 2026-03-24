# ADR-010: Declare-Distribute-Enforce Workflow Model

**Status:** accepted
**Date:** 2026-03-23
**Deciders:** Daniel Gomes
**Supersedes:** ADR-003 (Terraform Mental Model)

## Context

ADR-003 adopted the Terraform mental model (init → analyze → plan → apply → check) as the unifying workflow for verikt. This served well early on — it gave the CLI a coherent narrative and made `verikt.yaml` the central artifact.

But the product outgrew the analogy. Three problems:

1. **`verikt guide` has no place in the Terraform model.** Guide is verikt's primary differentiator — the command that feeds AI agents architectural context. It's the most important thing verikt does, and the mental model doesn't account for it.

2. **`verikt apply` doesn't make sense.** ADR-009 established that the CLI is plumbing — the agent is the execution layer. There's no "apply" step because the agent applies. verikt declares and enforces; it doesn't execute changes.

3. **`verikt plan` mapped to the wrong concept.** Terraform's `plan` diffs desired vs actual infrastructure. verikt's equivalent is `verikt check` (diff declared architecture vs actual code). A separate `plan` command for migration planning was theorized but never built — and may not be needed.

The actual workflow that emerged:

```
declare (verikt.yaml) → distribute (guide) → compose (new/add) → enforce (check) → detect (analyze)
```

This is "Declare-Distribute-Enforce" — not "Plan-Apply-Check."

## Decision

Replace the Terraform mental model with the **Declare-Distribute-Enforce** model. The three pillars:

**Declare** — `verikt.yaml` is the single source of truth for architecture, capabilities, components, and rules. Created by `verikt init` (brownfield) or `verikt new` (greenfield). Updated by `verikt add`.

**Distribute** — `verikt guide` generates architecture context for every AI agent (Claude Code, Cursor, Copilot, Windsurf). `verikt setup` registers verikt globally with installed agents. The declared architecture reaches every session without the engineer re-explaining it.

**Enforce** — `verikt check` validates that code matches the declared architecture. 11 AST-based detectors catch dependency violations, anti-patterns, and structural drift. Runs locally or in CI. `verikt analyze` detects architecture in existing codebases for brownfield adoption.

Supporting commands:

| Command | Role in model |
|---|---|
| `verikt init` | Declare (brownfield — detect and map) |
| `verikt new` | Declare + Compose (greenfield — scaffold with verikt.yaml) |
| `verikt add` | Declare (extend capabilities) |
| `verikt guide` | Distribute (generate agent context) |
| `verikt setup` | Distribute (register with AI agents globally) |
| `verikt check` | Enforce (validate architecture compliance) |
| `verikt diff` | Enforce (compare check results between commits) |
| `verikt analyze` | Detect (understand existing codebase) |
| `verikt decide` | Govern (resolve architecture decisions) |

## What changed from ADR-003

| ADR-003 (Terraform) | ADR-010 (Declare-Distribute-Enforce) |
|---|---|
| init → analyze → plan → apply → check | declare → distribute → enforce |
| `verikt plan` (diff desired vs actual) | Removed — `verikt check` covers this |
| `verikt apply` (execute changes) | Removed — the agent is the execution layer (ADR-009) |
| No distribution concept | `verikt guide` + `verikt setup` are first-class |
| Mental model borrowed from infrastructure | Mental model native to architecture + agentic engineering |

## What stays the same

- `verikt.yaml` remains the central artifact — the declarative desired state
- All commands read from or write to `verikt.yaml`
- Greenfield and brownfield converge on the same artifact
- The workflow is a loop: declare → distribute → compose → enforce → detect → refine

## Consequences

- "Terraform for Code Architecture" positioning is retired. The product is "Agentic Engineering Infrastructure" (per GTM workshop).
- Documentation and CLI help text should reference declare/distribute/enforce, not plan/apply/check.
- No `verikt plan` or `verikt apply` commands will be built. If migration planning is needed, it's a different feature with a different name.
- The model naturally extends: `verikt learn` (PRD-001) fits as a new "Distribute" capability — learning patterns and distributing them via the guide.
