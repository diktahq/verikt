# verikt — GTM Positioning

_Status: Accepted_
_Input: ICP Audit (2026-03-15), business-strategy.md, product spec_
_Source: Obsidian vault `10 - Projects/archway/gtm-positioning-workshop-2026-03-15.md`_

---

## Positioning Statement

**For** software engineers and engineering leaders using AI coding agents **who** lose time correcting architecturally incorrect output — because every agent session starts without context — **verikt** is **Agentic Engineering Infrastructure** that **gives AI agents your exact architecture before they write the first line — so every session is reliable, every engineer is consistent, and your architecture doesn't drift.** Unlike **manual CLAUDE.md files, re-explaining architecture every session, or hoping the agent figures it out,** verikt **generates context from your authoritative `verikt.yaml`, stays current as your architecture evolves, and works with every major AI coding agent.**

---

## Headline & Copy

**Headline:** "Your architecture, in every agent session."

**Pain line:** "The agent writes correct syntax. It doesn't always remember your architecture."

**Value proposition:** Give AI agents your architecture before they write the first line — so every session is reliable, every engineer is consistent, and your architecture doesn't drift.

**Category:** Agentic Engineering Infrastructure.

**Value pillars:** Reliability, Consistency, Predictability.

---

## Market Category

**Agentic Engineering Infrastructure** — a new category with no incumbents.

Tested against adjacent frames:

| Frame | Verdict |
|---|---|
| "Code generator / scaffolding tool" | Misses guide and enforce pillars. |
| "Architecture linter" | Misses guide and compose. |
| "AI agent plugin / extension" | Positions verikt as subordinate. verikt is infrastructure, not a plugin. |
| "Developer tooling" | Too broad, no signal. |
| "Agentic Engineering Infrastructure" | New category, names the shift, positions verikt as the tooling layer. |

**Market style: Create a New Game.** No competitor occupies this space. The timing is right — "agentic engineering" coined by Karpathy (February 8, 2026), converging fast across practitioners and industry.

---

## ICPs

### ICP-1: The Engineer

Senior backend engineer using AI coding agents daily. Cares about architecture, feels the pain of context loss every session, spends time correcting instead of shipping.

**Trigger:** Starting a greenfield project or experiencing inconsistent agent output on an existing one.

**Primary value:** `verikt guide` — agents write correct code from the first prompt, every session.

**GTM motion:** Pure PLG. Free CLI, zero friction, immediate value.

**Messaging:** "Your architecture, in every agent session." + "The agent writes correct syntax. It doesn't always remember your architecture."

**Persona:** Mateus, Senior Backend Engineer — 6 years experience, uses Claude Code daily, 8 services, 3 architectures. Spends 20 minutes per session re-establishing context.

### ICP-2: The Engineering Leader

Engineering manager or tech lead seeing architecture consistency degrade as their team adopts AI agents.

**Trigger:** Discovering architecture violations in code review — the wrong person catching the wrong problem at the wrong time.

**Primary value:** `verikt.yaml` as versioned spec + `verikt guide` distributed to every engineer + `verikt check` in CI.

**GTM motion:** PLG to sales-assist. ICP-1 adoption within team → leader notices → introduces verikt.yaml as org standard → cloud tier.

**Messaging:** "One `verikt.yaml`. Every engineer. Every agent. Every session." + "Every PR review is an architecture conversation. It shouldn't be."

**Persona:** Sofia, Engineering Manager — 12 engineers, 6 services. Architecture consistency dropped after Claude Code adoption 4 months ago.

---

## Competitive Alternatives

What engineers actually do today (the real competition is inertia, not tools):

| Alternative | Why it fails |
|---|---|
| Manual CLAUDE.md / .cursorrules | Written once, goes stale. No enforcement. |
| Re-explain architecture every session | Doesn't scale, breaks under context limits. |
| Hope the agent figures it out | Agents default to training data patterns, not your patterns. |
| ADRs and wiki pages | Agents don't read them. Engineers don't update them. |
| Code review | Catches violations too late, too expensive. |

---

## Unique Attributes

| Attribute | Why it's unique |
|---|---|
| `verikt guide` generates agent-native context files | No other tool outputs to `.claude/rules/`, `.cursorrules`, `.github/copilot-instructions.md`, `.windsurfrules` |
| Context generated from `verikt.yaml` — always current | Manual files go stale; verikt regenerates from the authoritative source |
| Composition model (architecture + capabilities) | No scaffolding tool also enforces and guides |
| 11 AST-based architecture detectors | None connected to a composition model or agent context |
| Smart capability suggestions (18 rules) | No tool proactively warns about dangerous combinations |

---

## Sales Narrative

**1. Setup (the shift)**
Agentic engineering is how software gets built now. Engineers orchestrate AI agents that write code. The shift from AI-assisted to AI-driven development is real and accelerating.

**2. Problem (what's broken)**
The agents have a blind spot: they don't remember your architecture. Every session starts cold. The standard workaround — hand-written CLAUDE.md files — doesn't scale.

**3. Solution**
verikt makes architecture a machine-readable, distributable, enforceable artifact. Declare in `verikt.yaml`. `verikt guide` generates context for every major AI agent. `verikt check` enforces in CI.

**4. Proof**
EXP-12d: explicit architecture context reduced violations from 4 to 0, turns from 30 to 16, cost from $1.01 to $0.33.

**5. Ask**
Install verikt. Run `verikt guide`. Commit the generated file. The difference is immediate.

```bash
brew install diktahq/tap/verikt
verikt guide
```

---

## GTM Motion

**Pure PLG for ICP-1.** `brew install` → `verikt guide` → visible output in under 2 minutes. Zero friction, zero cost, immediate value.

**PLG-to-sales for ICP-2.** ICP-1 adoption within team → leader notices consistency → cloud tier for dashboard, cross-repo enforcement, SSO (future).

---

## Pricing

**Freemium.** Free CLI forever (all core features).

| Tier | Price | Features | Buyer |
|---|---|---|---|
| Free | $0 | CLI: guide, new, check, analyze. All architectures, all capabilities. | ICP-1 (individual) |
| Team | ~$15-25/dev/mo | Dashboard, team rule sync, audit trail, CI integration | ICP-2 (team lead) |
| Enterprise | ~$50-100/dev/mo | SSO/SAML, RBAC, cross-repo enforcement, custom rule marketplace, SLA | ICP-2 (org-level) |

---

## Point of View

Agentic engineering — where AI agents write the code and humans architect and review — is becoming how software gets built. But the agents are missing a critical layer: they don't know your architecture. Every session starts cold. Conventions are reinvented. Patterns drift.

Architecture must become a machine-readable, distributable, enforceable artifact — not a document that humans write and agents ignore.

verikt is that infrastructure.

---

*Created 2026-03-15*
*Updated 2026-03-22 — Renamed archway → verikt, moved from Obsidian into project repo*
