# ADR-009: CLI as Agentic Engineering Infrastructure

**Date:** 2026-03-15
**Status:** Accepted

---

## Context

archway ships as a CLI tool. Every feature — `archway guide`, `archway new`, `archway check`, `archway analyze` — is a CLI command. The risk is that the CLI becomes the product, and the value proposition becomes "a good CLI for Go architecture."

That's the wrong frame.

The vision is broader: archway exists to make agentic engineering reliable, consistent, and predictable at the architecture level. Engineers use AI coding agents (Claude Code, Cursor, Copilot, Windsurf) to produce code. Those agents don't remember architecture between sessions. archway fixes that.

## Decision

**The CLI is the delivery mechanism. The product is what the agent can do that it couldn't before.**

Every CLI command exists to enable a better agent session — not to be used directly. The mental model:

- `archway guide` → agent loads architectural context silently before every session
- `archway new` → agent scaffolds correct structure from day one, giving guide an accurate source of truth
- `archway check` → agent (or CI) validates that what was built matches what was declared
- `archway analyze` → agent understands an existing codebase's architecture before touching it

The interface is the prompt the engineer writes in their agent session. The CLI is the plumbing underneath.

## Consequences

**For product decisions:**
- Every feature must answer: "What does this make possible in an agent session?"
- CLI UX matters, but agent UX matters more — what context does the agent get, what prompts become possible, what errors become preventable?
- Skills/slash commands are built only where the agent cannot infer the action from the guide context alone. They are the exception, not the rule.

**For marketing and content:**
- Homepage and persona pages lead with the agent session experience — the prompts that work, the outcomes that become reliable — not the CLI commands
- CLI commands appear as supporting context ("what runs under the hood") not as the primary value
- Canonical prompts (what an engineer types into Claude Code or Cursor that archway makes reliable) are first-class product assets, not documentation afterthoughts

**For the guide output:**
- The guide file must be written for the agent, not for the human. It is an instruction set, not a README.
- Suggested prompts in the guide are the highest-value section — they teach the engineer what to ask and teach the agent how to respond

**For roadmap:**
- Skills/slash commands are considered only where: (a) the prompt is non-obvious, and (b) the guide context alone is insufficient to route correctly
- Since archway installs into the repo (`archway.yaml` + guide files), the agent already has context and tools — skills fill gaps, they don't replace the guide

## The Model

| Scenario | Mechanism |
|---|---|
| Agent writes code → needs to know architecture | `archway guide` context file — passive, always on |
| Engineer asks "what am I missing before production?" | Agent reads guide, answers from context — no extra tooling |
| Engineer asks "add kafka capability" | Agent runs `archway add kafka-consumer && archway guide` — CLI as plumbing |
| Engineer wants a pre-built workflow | Skill/slash command — only where prompt is non-obvious |

---

*Captured from ICP audit session, 2026-03-15*
