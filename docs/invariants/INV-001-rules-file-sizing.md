# Invariant 001: Rules File Sizing Constraints

**Status:** active
**Date:** 2026-03-10

## Constraint

Generated rules files (from `verikt guide`) MUST stay within these bounds:

- **15-25 instructions** maximum per file
- **500-1,500 tokens** per file
- Critical rules (NEVER/ALWAYS) placed at **start and end** of file
- Each instruction paired with **motivation** (why, not just what)
- **3-5 code examples** included (most reliable steering mechanism)

### Always-loaded files and on-demand references

`verikt guide` writes two tiers, and the bound above applies to every file in the
first:

| Tier | Files | Bound |
|---|---|---|
| Always loaded | `.claude/rules/verikt-index.md` and the per-category `verikt-<category>.md`, `.cursor/rules/verikt.mdc`, `.github/copilot-instructions.md`, `.windsurfrules`, the verikt section of `AGENTS.md` | ≤30 instructions, ≤1,500 tokens each |
| On demand | `.verikt/references/architecture.md`, `capabilities.md`, `review.md`, and `interview.md` in catalog-only output | not budgeted: read only when the task calls for it |

The single-file targets (cursor, copilot, windsurf, AGENTS.md) used to receive one
composite file carrying everything the Claude path spreads across several, bounded at
3,100 tokens as an exception. Measured as bytes/4, that composite reached ~3,600
tokens for a 16-capability hexagonal config, over its own exception. Every target now receives
the same compact writer rules, which name the reference to read for each kind of
task; the capability catalog, smart suggestions, verification checklist, interaction
warnings, design questions, full layer detail and anti-patterns moved to the
references. The exception is gone: every always-loaded file meets 1,500.

### Onboarding content does not belong in a normative guide

The scaffold interview is emitted **only** in catalog-only output — the shape a
project without a `verikt.yaml` receives — and there as the on-demand
`.verikt/references/interview.md`. A guide generated *from* a `verikt.yaml`
describes an architecture that already exists, so instructions for scaffolding a new
service steer nothing.

Emitting it unconditionally cost 664 tokens, 23% of the composite file and the
majority of the Claude index. Removing it took the composite from ~2,935 to ~2,229
tokens and the index from ~1,092 to ~410. `verikt init --ai` and the `/verikt:init`
skill compose the protocol themselves, so the capability is unaffected.

A small project's index can fall below the 500-token lower bound. That bound
describes how much context an instruction-bearing file needs, not a requirement to
pad a file that has less to say.

## Rationale

Research on LLM instruction following (2025-2026 frontier models) shows:

1. **Compliance = f(instruction_count)** — more instructions exponentially reduces full compliance. At 10 instructions, GPT-4o achieves 15% full compliance. Even Gemini 2.5 Pro only achieves 68.9% at 500 instructions. (Source: IFScale, NeurIPS 2025 Workshop — https://arxiv.org/abs/2507.11538)

2. **Position matters** — "Lost in the Middle" effect: 20%+ performance drop for instructions in the middle of context. Start and end positions get highest attention. (Source: Liu et al., TACL 2023 — https://arxiv.org/abs/2307.03172)

3. **Agentic scenarios are hardest** — best models follow <30% of instructions perfectly in agentic contexts (exactly where coding agents operate). (Source: AgentIF, NeurIPS 2025 — https://arxiv.org/abs/2505.16944)

4. **Rephrasing fragility** — even GPT-5 drops 18% with subtle prompt variations. Consistent phrasing matters. (Source: IFEval++ — https://arxiv.org/abs/2512.14754)

## Consequences of Violation

- Bloated rules files (50+ instructions) will have lower total compliance than focused ones
- Users will experience inconsistent AI agent behavior
- The value proposition of `verikt guide` degrades

## Enforcement

- `TestGuideOutput_InstructionCount` (`internal/guide/budget_test.go`) fails if any always-loaded file has more than 30 instructions. An instruction is a bullet, numbered item, or table body row outside frontmatter and code fences; prose directives are not counted, so the count is a floor.
- `TestGuideOutput_TokenBudget` (same file) fails if any always-loaded file exceeds 1,500 tokens, approximated as `ceil(bytes / 4)`, which over-counts multi-byte characters and so errs toward failing.
- Both generate every target across minimal (flat), typical (hexagonal, four capabilities), capability-heavy (27 capabilities, every language feature, open decisions and proxy rules, in passive, audit and prompted modes) and catalog-only configs.
- `TestGuideOutput_CriticalRulesAtStartAndEnd` checks that each always-loaded file opens with its usage instructions and closes with a `## Before You Finish` section naming `verikt check`.
- `TestBudget_FailsClosedOnBloatedOutput` proves the budget tests can fail: the former monolithic guide exceeds both limits.
