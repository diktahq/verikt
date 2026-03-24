# verikt — ICP Audit

_Status: Accepted_
_Source: Obsidian vault `10 - Projects/archway/icp-audit-2026-03-15.md`_

---

## ICP-1: The Engineer

**Segment:** Software engineer or architect using AI coding agents on backend services — greenfield or existing codebase.

**Characteristics:**
- Uses Claude Code, Cursor, Copilot, or Windsurf daily
- Cares about code architecture — hexagonal, layered, clean, DDD
- Works on production services, not prototypes
- May or may not call what they do "agentic engineering" — but they do it

**The problem:**
The agent writes correct syntax. It doesn't always remember the architecture. Every session starts fresh — wrong layer, wrong file, reinventing patterns that already exist. At the surface: time spent correcting instead of shipping. At depth: missing circuit breakers, no idempotency, domain code importing adapters.

**Review layer pain:** When multiple engineers use different agents, inconsistency compounds. The senior engineer catches architecture problems in code review — too late, too expensive. Every PR becomes an architecture conversation. `verikt check` moves the catch point from code review to commit time.

**Current solution (competitive alternative):**
Manual CLAUDE.md files (written once, go stale), re-explaining architecture every session, or hoping the agent figures it out from context. All three are fragile and don't scale.

**Trigger (two paths):**
1. Starting a greenfield project — wants to set it up right for AI from day one
2. Already using agents and experiencing inconsistent, architecturally incorrect output

**Value delivered:**
`verikt guide` generates context files from `verikt.yaml` — always current, always accurate, works with every agent. The agent knows the architecture before it writes the first line. Adding a capability updates the guide in one command.

**WTP:** $0 (free CLI). Future: $15-25/dev/mo team tier.

**Adoption stage:** Early adopter. Buys on vision and competitive advantage.

**Acquisition:** PLG — installs from Homebrew, self-serves, refers teammates organically.

### Persona: Mateus, Senior Backend Engineer

6 years experience. Uses Claude Code daily. 8 services, 3 architectures, 2 teams.

**Day in the life:** Opens Claude Code, pastes a prompt, gets a response that puts service logic in the adapter layer. Corrects it. Next session, same thing. Spends 20 minutes per session re-establishing context. Has written a CLAUDE.md once, hasn't touched it since the last architecture refactor.

**Goals:**
- Ship faster without sacrificing architecture quality
- Stop being the last line of defence against agent-introduced drift
- Have consistent output across all agent sessions

**Frustrations:**
- "I've explained hexagonal architecture to this agent 50 times"
- "The agent writes good code but I can't trust where it puts things"
- "Every new engineer copies from a different service so our patterns diverge"
- "I'm finding architecture problems in code review — that's not where I should be finding them"

**Objections:**
- "I already have a CLAUDE.md" → verikt.yaml is the authoritative source, guide stays current as architecture evolves
- "My project is already set up" → `verikt analyze` detects existing architecture, `verikt guide` runs from there

**Quote:** "The agent is good. It just doesn't know my codebase the way I do. verikt fixes that."

---

## ICP-2: The Engineering Leader

**Segment:** Engineering manager, tech lead, or architect responsible for architecture consistency across a team or org using agentic engineering.

**Characteristics:**
- Manages 3–20 engineers, all using AI coding agents
- Has established (or is establishing) architecture standards
- Sees inconsistency compounding: new services look different, patterns diverge, onboarding takes longer

**The problem:**
Every engineer's AI agent session is stateless. Each agent defaults to its own patterns. Three engineers, three services, three different interpretations of "hexagonal". The architecture diagram stops matching the code. Onboarding takes a week just to understand structural inconsistencies.

**Review layer pain:** Architecture violations accumulate undetected until code review — or worse, production. The tech lead becomes the last line of defence. `verikt check` in CI makes the tooling the enforcer, not the person.

**Current solution (competitive alternative):**
Architecture decision records nobody reads, internal wiki pages that go stale, periodic architecture reviews that find drift after the fact, hoping engineers follow the standards they were told once during onboarding.

**Trigger (two paths):**
1. Team is adopting AI agents and the leader wants governance before drift compounds
2. Architecture governance problem already exists; agentic engineering makes it urgent

**Value delivered:**
`verikt.yaml` as the versioned, commitable architecture spec. `verikt guide` distributed to every engineer. `verikt check` in CI. One source of truth, enforced everywhere.

**WTP:** $50-100/dev/mo enterprise tier — dashboard, cross-repo enforcement, SSO, RBAC, audit trail (future).

**Adoption:** Follows ICP-1. Leaders notice when multiple engineers on the team are using verikt.

### Persona: Sofia, Engineering Manager

12 engineers, 6 backend services. Team adopted Claude Code 4 months ago. Productivity increased but architecture consistency dropped.

**Day in the life:** Reviews PRs and notices the payments service uses hexagonal but the new notifications service the agent scaffolded is flat with everything in main.go. Writes a Slack message reminding the team of the architecture standards. Same message as last month.

**Goals:**
- Consistent architecture across all services, enforced — not documented
- New engineers productive within days, not weeks
- AI agent sessions that reinforce team standards, not diverge from them

**Frustrations:**
- "We have architecture standards. The agents don't know them."
- "Every new service is a different codebase to learn"
- "I spend code review time on structural decisions, not business logic"
- "Every PR review is an architecture conversation. It shouldn't be."

**Objections:**
- "We already have ADRs and architecture docs" → verikt.yaml is the machine-readable ADR, guide distributes it automatically
- "Will this slow down the team?" → engineers install once, guide runs in seconds, check runs in CI

**Quote:** "I don't want to be the enforcer. I want the tooling to be the enforcer."

---

*Created 2026-03-15*
*Updated 2026-03-22 — Renamed archway → verikt, moved from Obsidian into project repo*
