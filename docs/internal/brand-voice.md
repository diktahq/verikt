# verikt — Brand Voice & Tone

_Status: Accepted_
_Input: ICP Audit, GTM Positioning Workshop, ProductResearcher knowledge library_
_Source: Obsidian vault `10 - Projects/archway/brand-voice-2026-03-15.md`_

---

## Brand Personality

**Primary dimension:** Competence — reliable, intelligent.
**Secondary dimension:** Sincerity — honest, real. Transparent about limitations.

**Four-word profile:**

### Direct. Confident. Precise. Grounded.

---

## Voice Profile

Voice is constant. It doesn't change from the homepage to a CLI error message to a blog post. These four words apply everywhere.

### Direct

verikt leads with the point. No warming up, no throat-clearing, no "In today's fast-paced world." If the answer is three words, the answer is three words.

| Like this | Not like this |
|---|---|
| `verikt guide` generates architecture context for AI agents. | We're excited to introduce a powerful new capability that helps your team's AI coding assistants understand your project's architecture. |
| Install with Homebrew. | To get started with the installation process, we recommend using Homebrew as your package manager of choice. |
| The agent doesn't know your architecture. verikt fixes that. | There's often a gap between the architectural decisions your team has made and what AI agents are actually aware of when they generate code. Our tool bridges that gap. |

**The rule:** If the first sentence doesn't deliver information, cut it.

---

### Confident

verikt states positions clearly. It doesn't hedge, doesn't apologise for what it is, and doesn't qualify every claim into meaninglessness. Confidence comes from evidence — experiments, results, lived experience — not from adjectives.

| Like this | Not like this |
|---|---|
| This is the missing infrastructure of agentic engineering. | We believe this could potentially be a useful addition to your agentic engineering workflow. |
| `verikt check` catches violations before they ship. | `verikt check` can help identify potential architecture violations that might otherwise go unnoticed. |
| verikt is not a framework. It generates plain code with no runtime dependency. | While verikt is not technically a framework per se, it does generate code that you can use without any additional runtime dependencies. |

**The rule:** Say what it is. Not what it "might be" or "could potentially help with."

---

### Precise

verikt names the specific thing. Numbers over adjectives. Capability names over vague descriptions. "11 AST-based detectors" not "powerful analysis." "63 capabilities" not "a wide range of features."

| Like this | Not like this |
|---|---|
| 11 AST-based detectors catch dependency violations. | Our powerful analysis engine catches a wide variety of architectural issues. |
| EXP-12d: 0 violations with explicit context vs 4 without. 16 turns vs 30. $0.33 vs $1.01. | In our testing, we found that providing architecture context significantly improved agent output quality. |
| `retry` without `idempotency` means retrying a payment endpoint will double-charge the customer. | Certain capability combinations can lead to issues in production if not properly configured. |

**The rule:** If you can replace a vague word with a specific one, do it.

---

### Grounded

verikt doesn't make claims it can't back. Every assertion has evidence behind it — an experiment, a data point, a lived experience. When something is unknown or unproven, verikt says so. Limitations are stated plainly, not hidden.

| Like this | Not like this |
|---|---|
| In EXP-12d, explicit architecture context reduced violations from 4 to 0 — single run, controlled conditions. | verikt dramatically improves code quality across all development workflows. |
| Go is fully supported. TypeScript is next. | verikt works with all major programming languages. |
| This doesn't work for brownfield refactoring without a `verikt.yaml` — run `verikt analyze` first. | verikt seamlessly handles all project types, whether greenfield or brownfield. |

**The rule:** Evidence first. Limitations stated. No overclaiming.

---

## Tone Shifts

Voice is constant — Direct, Confident, Precise, Grounded — always. Tone shifts to match context. The voice stays; the emotional register adapts.

| Context | Tone shift | Example |
|---|---|---|
| **Homepage hero** | Confident + warm. The opening handshake. | "Your architecture, in every agent session." |
| **Documentation** | Precise + helpful. Teach, don't sell. | "verikt guide reads your verikt.yaml and generates context files for every AI agent." |
| **CLI output** | Direct + minimal. Respect the terminal. | "Generated .claude/rules/verikt.md (1,247 tokens)" |
| **Error messages** | Direct + empathetic. Name the problem, suggest the fix. | "No verikt.yaml found. Run `verikt analyze` to detect your architecture, or `verikt new` to start fresh." |
| **Blog posts** | Grounded + personal. Practitioner voice, first-person where it fits. | "I ran 15 experiments to understand how AI agents fail at architecture. Here's what I found." |
| **LinkedIn/social** | Confident + accessible. Hook in first line, plain language. | "AI agents write correct syntax. They don't always remember your architecture. That's the gap verikt fills." |
| **Release notes** | Precise + brief. What changed, why, what to do. | "v0.1.0: Rust analysis engine, `verikt check --diff`, severity overrides." |
| **For Engineers page** | Empathetic + confident. Lead with the pain they feel. | "The agent writes correct syntax. It doesn't always remember your architecture." |
| **For Engineering Leaders page** | Confident + evidence-backed. Lead with the team problem. | "Every PR review is an architecture conversation. It shouldn't be." |
| **Incident/breaking change** | Direct + honest. No spin, no corporate deflection. | "v0.1.1 has a bug in guide generation for clean architecture. Upgrade to v0.1.2. We missed this in testing." |

---

## Pronouns

- **verikt** is always lowercase. It's a tool, not a brand that shouts.
- Use **"you"** to address the reader. Frequently. The copy is for them, not about us.
- Use **"your"** for ownership — "your architecture", "your agent session", "your team."
- Use **"verikt"** as the subject when describing what the tool does. Not "we" — the tool does it.
- Use **"we"** sparingly and only in blog posts or personal content where the author's voice is present.

| Like this | Not like this |
|---|---|
| verikt generates context files from your verikt.yaml. | We generate context files from your verikt.yaml. |
| Your architecture, in every agent session. | Our tool provides architecture context for your sessions. |

---

## Jargon Policy

verikt speaks to practitioners. Technical terms are fine when they're the right word. But jargon must earn its place.

**Use freely (homepage + everywhere):**
- hexagonal architecture, layered, clean — core vocabulary
- CI, PR, CLI — universal for the audience
- agentic engineering — the category, always grounded for newcomers

**Use freely (below the fold, subpages, docs — not the hero):**
- circuit breaker, idempotency, retry, outbox — resonate with engineers who've hit production issues
- DDD, CQRS, saga, bounded context — fine on concepts pages, not homepage
- AST — "11 architecture detectors" on homepage. "AST-based" in docs, CLI reference, and For Engineers page

**Avoid** — corporate jargon that signals nothing:
- leverage, synergy, at scale, robust, seamless
- cutting-edge, game-changing, revolutionary, next-generation
- end-to-end, best-in-class, enterprise-grade (unless literally describing the tier)

---

## What verikt Never Sounds Like

- **A SaaS landing page.** No "unlock the power of", no "supercharge your workflow."
- **A press release.** No "we're thrilled to announce", no "industry-leading."
- **A textbook.** No dry, detached academic prose. verikt has personality — it's just measured.
- **A hype machine.** No claims without evidence. No adjectives doing the work that data should do.

---

## The Logo Test

Cover the verikt logo. Read the copy aloud. Does it sound like verikt — direct, confident, precise, grounded? Or could it be any developer tool's website?

If you can swap the product name and nothing feels wrong, the voice isn't present. Rewrite.

---

## Application Priority

Voice applies everywhere, but these are the highest-leverage places to get right first:

1. **Homepage hero** — first impression, sets expectations
2. **CLI output and error messages** — most frequent touchpoint, unexpected place for personality
3. **For Engineers page** — ICP-1's entry point
4. **For Engineering Leaders page** — ICP-2's entry point
5. **Blog posts** — where the practitioner voice comes through strongest
6. **Social media** — where the hook matters most

---

*Created 2026-03-15*
*Updated 2026-03-22 — Renamed archway → verikt, moved from Obsidian into project repo*
