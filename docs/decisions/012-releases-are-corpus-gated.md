# ADR-012: Releases Are Gated on a Corpus of Real Repositories

**Status:** Accepted
**Date:** 2026-08-15
**Supersedes:** none

## Context

A detector's value is bounded by the trust of the person reading its output. One
false positive teaches a reader to skim the section it appears in; a few teach
them to ignore the tool. Precision is therefore not a quality attribute of verikt
— it is the product.

Two ways of pursuing it were tried on the same day, and the results were not close.

**Fixtures written from imagination found nothing.** A restraint fixture of ten
hand-written files, built specifically to catch false positives, passed while
five distinct false-positive classes were live. It could only ever contain the
cases its author had already pictured, and the ones that cost trust are precisely
the ones nobody pictured.

**Real code found everything.** One project, dogfooded for an afternoon, produced
three. A corpus of seven repositories — roughly 1,900 Go files, including
third-party code — produced two more, one of which had been introduced hours
earlier *by the fix for another*. Reviewing that fix by reading it had not caught
it; running it over real code did, immediately.

Four of the five were error severity, so each failed a build, and none could be
fixed in the reporting project's code. The escapes available were a waiver, which
the tool's own guidance calls the wrong answer to a false positive, or rewriting
correct code — which one project did, rewording English error messages so a
detector would stop firing on them.

## Decision

**Detectors are validated against real repositories before a release.**
`scripts/corpus-audit.sh` copies each repository to a scratch directory, analyses
the copy, and reports every finding with the line that produced it, grouped by
detector and by concentration.

**This is run by hand today, not in CI.** Saying otherwise would repeat the
mistake this decision exists to correct: ADR-006 claimed a clean cut that the
code had never implemented, and a record that describes an intention as a fact is
worse than no record. A CI gate needs the corpus pinned by commit, which needs
repositories that are public and permissively licensed; the most informative ones
used so far are private. Until that exists, the obligation is a step in the
release procedure and nothing stronger.

Three rules follow:

0. **Run it before tagging.** `scripts/corpus-audit.sh <repo>...` over whatever
   real Go repositories are to hand, and read the suspected-false-positive
   section. It takes about a minute over 1,900 files.
1. **The corpus is real code, not written for the purpose.** Including code
   neither the author nor the project controls, because shared idioms produce
   shared blind spots.
2. **Repositories are never written to.** Analysing a copy is not a convenience;
   `verikt check` requires config in the project root, and writing into someone
   else's checkout to audit it is the wrong trade even temporarily.
3. **A finding nobody would act on is a defect in verikt**, not something for the
   reporting project to configure around. It becomes a case in
   `testdata/detector-restraint` (INV-005).

## Consequences

A false positive is found by the corpus rather than by a user. The residual ones
are bounded by severity discipline: a heuristic detector ships at warning and
earns error severity by demonstrating precision here, so the ones that slip
through are noise rather than a failed build.

The corpus costs a minute to run over 1,900 files, and it grows: every reported
false positive is a repository or a reduced case worth adding.

It does not make false positives impossible. Heuristics over an AST have edges,
and a corpus of nine repositories is not the world. What it changes is who finds
them — and that a fixture passing is no longer mistaken for evidence.
