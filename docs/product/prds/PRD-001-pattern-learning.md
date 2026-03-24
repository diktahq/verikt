# PRD-001: Pattern Learning & Project-Aware Generation

_Status: Draft — design in progress_
_Author: Daniel Gomes_
_Date: 2026-03-23_

---

## Problem

verikt scaffolds new services from shipped templates (`verikt new`). Once the project exists, those templates become irrelevant. The project evolves its own patterns — how handlers are structured, how repositories are wired, how domain entities look. The shipped templates might not even match how the team actually writes code.

Today, when a developer wants to add an endpoint to an existing hexagonal service, verikt has nothing to offer. The agent figures it out from the guide context, but verikt itself can't generate project-consistent artifacts.

The best way to learn a codebase is to read pull requests that do the thing you want to do. A PR that adds an endpoint teaches you everything: what files were created, what files were modified, naming conventions, import structure, where the route gets registered, how the test is laid out. verikt should capture that knowledge and make it reusable.

## Insight

Not all endpoints are the same in terms of code and domain logic, but the structure and design repeat. A project's own code is the best template for new code in that project. Shipped templates bootstrap the project; the project's own patterns should take over from there.

## Proposal

Extend verikt with three capabilities:

1. **`verikt learn`** — capture structural patterns from existing code
2. **`verikt new <pattern> <Entity>`** — generate artifacts from learned patterns (or output agent specs)
3. **Guide integration** — embed the pattern catalog in the guide so agents always know how to add things

### Pattern lifecycle

```
Day 0 (greenfield):    verikt new → shipped templates → working service
Day 1 (first feature): developer adds an endpoint (manually or via agent)
Day 2 (learning):      verikt learn endpoint → captures the pattern from real code
Day N (generation):    verikt new endpoint Order → generates from learned patterns
```

Shipped templates don't go away. They bootstrap the project. Once real code exists, learned patterns override shipped templates for in-project generation.

## User-defined patterns

The pattern catalog is not a fixed list. Users define what's worth learning. A payments service might learn "webhook-handler", "idempotency-wrapper", "reconciliation-job". An event-driven service might learn "event-consumer", "saga-step", "projection". The pattern name is freeform — verikt doesn't need to know the domain semantics. It needs to know what files constitute the pattern, how they're named, and where they live.

## Learning from exemplars

### Single exemplar

One exemplar gives you the structure:

```bash
verikt learn endpoint --exemplar adapter/httphandler/user_handler.go
```

verikt extracts: file location, naming convention, struct shape, constructor, method signatures, imports, test file location, registration point.

### Multiple exemplars

Three exemplars give you the pattern — what varies and what stays constant:

```bash
verikt learn endpoint \
  --exemplar adapter/httphandler/user_handler.go \
  --exemplar adapter/httphandler/product_handler.go \
  --exemplar adapter/httphandler/payment_handler.go
```

verikt diffs the exemplars and identifies:
- **Constant:** struct shape, constructor signature, error handling, response serialization
- **Variable:** entity name, route path, method set, validation logic

The variable parts become parameters. The constant parts become the pattern. This is what a new engineer does when reading three PRs — "this part is always the same, this part changes per entity."

### Exemplar sets (multi-file patterns)

Most patterns span multiple files. An "endpoint" pattern includes a handler, a service port, a service implementation, a domain entity, and a test. The user provides exemplar sets:

```bash
verikt learn endpoint \
  --exemplar adapter/httphandler/user_handler.go \
  --exemplar port/user_service.go \
  --exemplar service/user_service.go \
  --exemplar domain/user.go \
  --exemplar adapter/httphandler/user_handler_test.go
```

verikt detects that these files share a common entity reference ("user") and extracts the multi-file pattern.

## Interaction model

### Interactive (TUI)

```bash
verikt learn endpoint
# "Which files are part of this pattern?"
# User selects from file list or provides paths
# "I found these files share the entity 'User'. Is 'Entity' the right parameter name?"
# User confirms
# "I detected these naming conventions: ..."
# User confirms/refines
# "This pattern registers routes in adapter/httphandler/routes.go. Correct?"
# User confirms
# Pattern saved to .verikt/patterns/endpoint.yaml
```

### Non-interactive

```bash
verikt learn endpoint \
  --exemplar adapter/httphandler/user_handler.go \
  --exemplar port/user_service.go \
  --no-wizard
# Dumps .verikt/patterns/endpoint.yaml directly
# User reviews in editor, commits in a PR
```

Both paths produce the same artifact: a YAML file in `.verikt/patterns/`. The TUI is the fast path; the YAML is what gets versioned and reviewed by the team.

## Pattern file format

```yaml
# .verikt/patterns/endpoint.yaml
name: endpoint
description: "HTTP endpoint — handler, service port, implementation, entity, test"

parameters:
  - name: Entity
    description: "Domain entity name"
    example: "Order"
    transforms:
      snake: "order"
      plural: "orders"
      pascal: "Order"

learned_from:
  - adapter/httphandler/user_handler.go
  - adapter/httphandler/product_handler.go
  - adapter/httphandler/payment_handler.go

creates:
  - path: "adapter/httphandler/{{.Entity | snake}}_handler.go"
    exemplar: adapter/httphandler/user_handler.go
    layer: adapters
  - path: "port/{{.Entity | snake}}_service.go"
    exemplar: port/user_service.go
    layer: ports
  - path: "service/{{.Entity | snake}}_service.go"
    exemplar: service/user_service.go
    layer: service
  - path: "domain/{{.Entity | snake}}.go"
    exemplar: domain/user.go
    layer: domain

modifies:
  - path: adapter/httphandler/routes.go
    description: "Register {{.Entity}} routes"

tests:
  - path: "adapter/httphandler/{{.Entity | snake}}_handler_test.go"
    exemplar: adapter/httphandler/user_handler_test.go
```

The `exemplar` field points to a real file in the project — not an abstracted template. The real code is the template. When generating, verikt reads the exemplar, substitutes entity-specific parts, and produces the skeleton.

## Generation modes

### 1. Files (direct generation)

```bash
verikt new endpoint Order
# Created: adapter/httphandler/order_handler.go
# Created: port/order_service.go
# Created: service/order_service.go
# Created: domain/order.go
# Created: adapter/httphandler/order_handler_test.go
# Action needed: register routes in adapter/httphandler/routes.go
```

Produces real files with the structural skeleton. Domain logic is stubbed. Good for getting the wiring right fast.

### 2. Spec (agent instructions)

```bash
verikt new endpoint Order --spec
```

Outputs a structured prompt the agent can execute. Includes exemplar code, file locations, naming conventions, dependency rules. The agent generates with full project context.

### 3. Guide integration (passive)

The pattern catalog is embedded in the guide automatically. Every agent session knows: "this project has these patterns, here's how to create each one, here are the exemplar files to follow." No explicit `verikt new` needed — the agent reads the guide and knows.

## What verikt extracts vs. what the user provides

**User provides:**
- Pattern name (freeform: "endpoint", "saga-step", "webhook-handler")
- Exemplar files
- Optionally: parameter names, descriptions, additional context

**verikt extracts:**
- File locations and naming conventions (by comparing exemplars)
- Import patterns (what gets imported, from which layers)
- Structural skeleton (struct shape, method signatures, constructor pattern)
- Registration points (which files get modified when a new instance is added)
- Test structure (corresponding test files and their shape)
- Component/layer membership (which `verikt.yaml` component each file belongs to)
- Naming transforms (snake_case files, PascalCase types, kebab-case routes)

**verikt validates:**
- The pattern respects the declared architecture (no dependency violations)
- New instances generated from the pattern will pass `verikt check`

## Relationship to existing features

### `verikt new` (greenfield)

Stays as-is for creating whole services from shipped templates. Extends to support `verikt new <pattern> <Entity>` for in-project generation.

Priority: learned patterns override shipped templates. If `.verikt/patterns/endpoint.yaml` exists, `verikt new endpoint` uses it. If not, falls back to shipped capability templates.

### `verikt add` (capabilities)

Unchanged. `verikt add redis` still adds the redis capability from shipped templates. Pattern learning is about project-specific artifacts, not verikt-catalog capabilities.

### `verikt analyze`

The analyzer already detects architecture, framework, conventions, and dependency graphs. Pattern learning extends this: the analyzer provides the structural understanding, pattern learning captures the recipes.

Future: `verikt learn --auto` could use the analyzer to suggest patterns. "I found 5 files matching `adapter/httphandler/*_handler.go` — want to learn this as a pattern?"

### `verikt guide`

The guide gains a "Pattern Catalog" section listing all learned patterns with their file locations, naming conventions, and exemplar references. This is the highest-leverage integration — every agent session has the pattern knowledge without the user running `verikt new`.

### `verikt check`

Validation: when generating from a pattern, verikt verifies the output respects the declared architecture. A pattern that creates a file in `domain/` importing from `adapter/` would be flagged during learning, not during generation.

## Incremental delivery

### v0.4: Pattern catalog (manual definition, guide integration)

- `verikt learn <name> --exemplar <file> [--exemplar <file>...]`
- Interactive TUI: asks for pattern name, description, parameters
- `--no-wizard` dumps YAML directly
- Saves to `.verikt/patterns/<name>.yaml`
- Guide generation includes pattern catalog in "Adding code" section
- `verikt patterns` lists learned patterns

### v0.5: Pattern generation

- `verikt new <pattern> <Entity>` generates files from learned patterns
- Reads exemplar, substitutes entity name, writes skeleton files
- `--spec` flag outputs agent instructions instead of files
- `--dry-run` shows what would be created
- Handles `modifies` entries (registration points) — either auto-modifies or instructs

### v0.6: Multi-exemplar inference

- Multiple `--exemplar` flags
- verikt diffs exemplars to separate constant from variable parts
- Detects all varying parameters (not just entity name)
- Confidence scoring: "3/3 exemplars follow this structure" vs. "2/3 match"
- Pattern refinement: `verikt learn endpoint --add-exemplar payment_handler.go`

### v0.7: Auto-detection

- `verikt learn --auto` scans the project and suggests patterns
- Groups files by naming convention and structural similarity
- User confirms, refines, or dismisses each suggestion
- "I found 5 handlers, 3 repositories, 2 event consumers — want to learn these?"

## Key design tension: curation vs. generation

During design discussion, a fundamental question emerged: **given that AI agents already perform well on well-structured codebases (as shown in experiments like EXP-12d), does verikt need a pattern learning and code generation engine at all?**

The agent is good at pattern matching. In a small, clean codebase, it reads existing handlers, infers the pattern, and produces matching code. The guide already provides architecture context. Building a parametric substitution engine that replaces `User` with `Order` is strictly worse than what the agent produces with full context — the agent understands intent, not just structure.

**But the agent doesn't know which code to learn from.** It sees all code as equally valid. It has no concept of "this is the canonical way" vs. "this is technical debt we haven't cleaned up." In a real codebase with history — legacy handlers, quick hacks that stuck, patterns the team deprecated last quarter — the agent might read the wrong exemplar. In a large codebase with multiple domains, it might copy from the wrong domain's implementation style.

This is the same problem verikt already solves at the architecture level: `verikt check` says "domain can't import from adapters" and the agent can't drift. But there's no equivalent for "when you add a handler, follow *this* handler, not *that* one."

### The reframe: canonical exemplar curation

The core value might not be generation — it might be **curation**. The team declares which files represent the canonical patterns. verikt distributes that declaration to every agent session via the guide. The agent doesn't discover patterns — it receives them.

An exemplar isn't a single file. An endpoint is a conjunction of things: handler, port, service, domain entity, test, route registration. These files belong together as a unit. The agent needs to know they're a unit — not just "here's a good handler" but "when you add an endpoint, you create these five files and modify this sixth one."

This could be as simple as a section in `verikt.yaml`:

```yaml
exemplars:
  endpoint:
    description: "HTTP endpoint — full vertical slice"
    files:
      - adapter/httphandler/user_handler.go
      - port/user_service.go
      - service/user_service.go
      - domain/user.go
      - adapter/httphandler/user_handler_test.go
    modifies:
      - adapter/httphandler/routes.go
```

The names are freeform. The paths point to real files. verikt embeds these in the guide with actual code from those files and the instruction: "when creating new instances of this pattern, follow these exemplars as a unit."

### What we don't know yet

Whether curation alone is enough depends on how agents actually behave with structured exemplar sets:

- Does the agent consistently produce the full set of files when told "an endpoint is these 5 files"?
- Does it follow the exemplar structure reliably, or does it drift?
- Does it remember the registration step (modifying routes.go)?
- How does behavior differ with vs. without the exemplar section in the guide?

**This is an empirical question, not a design question.** An experiment (EXP-TBD) should answer it before committing to the engineering investment. See "Validation experiment" below.

### Decision tree based on experiment results

**If the agent reliably follows structured exemplar sets:**
→ The feature is exemplar curation in `verikt.yaml` + guide enrichment. Small engineering effort, high value. The generation engine (v0.5+) is unnecessary.

**If the agent is inconsistent (forgets files, drifts from exemplars, misses registration):**
→ There's a case for verikt doing more: generating skeleton files directly, producing step-by-step checklists the agent executes, or post-generation validation that all expected files exist.

## Validation experiment (EXP-TBD)

**Goal:** Determine whether AI agents reliably follow structured exemplar sets in the guide, and whether this reduces pattern drift compared to unguided generation.

**Setup:**
- Take an existing verikt-scaffolded hexagonal Go service with 3+ existing endpoints
- Two conditions: (A) standard guide without exemplar section, (B) guide with structured exemplar section listing the canonical endpoint files

**Task:** "Add an Order endpoint with CRUD operations"

**Measurements per run (10 runs per condition):**
1. **Completeness:** Did the agent create all expected files? (handler, port, service, entity, test)
2. **Structural fidelity:** Do the created files follow the exemplar's structure? (constructor pattern, method signatures, error handling)
3. **Registration:** Did the agent modify routes.go to register the new endpoint?
4. **Layer correctness:** Do all files respect the architecture (no dependency violations)?
5. **Naming consistency:** Do file names, type names, and route paths follow the project's conventions?
6. **Unprompted drift:** Did the agent introduce patterns not present in the exemplar? (different error handling, extra abstraction layers, etc.)

**Success criteria:**
- Condition B should show ≥90% completeness (all 5 files created) across 10 runs
- Condition B should show measurably less drift than Condition A
- If Condition B shows <80% completeness or significant drift, the generation engine is justified

**Design this experiment before building the feature.**

## Open design questions

1. **Exemplar freshness** — when the exemplar file changes (refactored, renamed), should the pattern auto-update? Or is the pattern a snapshot? Leaning toward: the pattern references the exemplar by path, reads it at generation time, so it's always current. But structural changes (adding/removing methods) might invalidate the pattern.

2. **Parameter detection** — with a single exemplar, how does verikt know which parts are entity-specific? Options: (a) user tells it, (b) verikt uses naming heuristics (file name matches struct name matches package references), (c) multi-exemplar diff is required for reliable detection.

3. **Registration points** — modifying existing files (adding a route to routes.go) is harder than creating new files. Options: (a) verikt generates the line to add and tells the user where, (b) verikt modifies the file directly (risky — needs to find the right insertion point), (c) leave it to the agent via the spec.

4. **Pattern versioning** — as the project evolves, patterns might need updating. Should `verikt learn` support `--update` to re-learn from newer exemplars? Or is the user expected to delete and re-learn?

5. **Cross-language patterns** — a TypeScript project with the same architecture would have structurally similar patterns but different file extensions, import syntax, and type systems. Should patterns be language-aware? Likely yes — the pattern file already includes the exemplar paths, which are language-specific.

6. **Composition** — can patterns compose? "endpoint" might include "entity" as a sub-pattern. Should a user be able to say `verikt new endpoint Order` and have it invoke the entity pattern as part of the endpoint pattern? Or keep patterns flat?

7. **Agent-generated code feeding back** — when an agent creates a new endpoint following the guide's pattern catalog, should `verikt learn` be able to detect "the agent just created something that matches the endpoint pattern — want to add it as an exemplar?" This closes the loop: guide teaches agent, agent produces code, code strengthens the pattern.

8. **Curation vs. generation** — the fundamental tension. Is the value in verikt generating files, or in verikt telling the agent which files to follow? The answer depends on how reliably agents follow structured exemplar sets. See "Validation experiment" above.

---

## Next steps

1. **Design and run EXP-TBD** — the validation experiment. Results determine whether the feature is exemplar curation (small) or pattern generation (large).
2. **Resolve open design questions** based on experiment data.
3. **Write technical spec** once the scope is clear.

---

*This PRD captures the design discussion as of 2026-03-23. Design is ongoing. The validation experiment must run before committing to scope.*
