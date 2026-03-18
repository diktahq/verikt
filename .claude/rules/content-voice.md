# Content Voice

Rules for writing any user-facing content: website pages, README, docs, release notes, error messages. Applies to all prose in the project.

## Voice

Write as a practitioner explaining to a fellow engineer. First-person where it fits. Direct, confident, no hedging. Problem before solution — always.

**Register:** Direct technical practitioner. Not a SaaS landing page, not a tutorial, not a press release. An engineer describing what they built and why.

## Hard Rules

- **Problem first.** Never introduce a feature or page without establishing the problem it solves.
- **No AI slop.** No hollow openers ("In today's fast-paced world"), no fake transitions ("It's worth noting that"), no listicle creep, no restating what was just said. It should read like a human wrote it.
- **No marketing fluff.** No "revolutionary", "game-changing", "transformative", "the future of". No CardGrid with icons when prose would do. No "Who Is It For" sections with bullet points that could describe any product.
- **No invented facts.** Every claim must have evidence — experiment data, code, or lived experience. If the source material doesn't support a claim, don't make it. "We ran EXP-09 and the guide reduced violations from 8 to 1" is a fact. "The guide dramatically improves code quality" is not — it's a generalization without evidence.
- **Evidence with transparency.** When citing experiment results, include the conditions, the sample size, and what didn't work. Single-run results are flagged as single-run. Falsified hypotheses are reported, not hidden. Cherry-picking is not acceptable.
- **No jargon without purpose.** Technical terms are fine when they're the right word. Corporate jargon (leverage, synergy, at scale) is never the right word.

## Tone

- **Confident but not arrogant.** State earned positions clearly. Don't hedge every sentence.
- **Curious over authoritative.** Posture is learner, not professor.
- **Direct.** Get to the point. No throat-clearing.
- **Measured conviction.** The writing carries the weight, not the punctuation.
- **Honest about limitations.** "This doesn't work for brownfield refactoring" is more credible than silence.

## Structure

- **Short sentences for emphasis.** "That's the gap." / "Same agent. Different outcome."
- **Prose over bullets.** Ideas that flow should be paragraphs, not bullet lists. Lists for concrete items (commands, file paths, capability names).
- **No padding.** Each paragraph moves something forward. If a paragraph doesn't add information, delete it.
- **Code examples with context.** Don't drop code blocks raw — one sentence before explaining what the reader is looking at.

## What This Applies To

- README.md
- Website pages (website/src/content/docs/)
- CLI help text and error messages
- Release notes
- Any markdown the user reads

## Brand Voice: Direct. Confident. Precise. Grounded.

Four words that define who archway sounds like. Voice is constant — it doesn't change from the homepage to a CLI error message.

- **Direct** — leads with the point, no preamble. If the first sentence doesn't deliver information, cut it.
- **Confident** — states positions clearly, doesn't hedge. Confidence comes from evidence, not adjectives.
- **Precise** — numbers over adjectives, capability names over vague descriptions. "11 architecture detectors" not "powerful analysis."
- **Grounded** — every assertion has evidence behind it. Limitations stated plainly. No overclaiming.

**Tone shifts by context** (voice stays, register adapts):
- Homepage hero: confident + warm
- Documentation: precise + helpful
- CLI output: direct + minimal
- Error messages: direct + empathetic
- Blog posts: grounded + personal
- Social media: confident + accessible

Full definition: `docs/internal/brand-voice-2026-03-15.md`

## What This Does NOT Apply To

- Generated guide files (those follow their own format for AI agent consumption)
- Test file comments
- Internal code comments (see code-quality.md)
- Commit messages (see CLAUDE.md)
