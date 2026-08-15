# INV-005: A Detector Establishes Evidence, Not Shape

A detector must establish that the risk it names is actually reachable before reporting it.
Matching a shape — a substring, a declaration kind, a directory containing `.go` files — is
not enough when the context makes the risk impossible.

## What This Means

- A textual match is evidence of text, not of the thing the text resembles. `sql_concatenation`
  must establish that the file deals with SQL before reading a concatenated string as a query.
- A declaration kind is evidence of syntax, not of behaviour. `global_mutable_state` must
  establish that the variable is reachable and written before calling it mutable state.
- A directory is evidence of files, not of membership. `orphan_package` must establish that
  the directory belongs to this module before deriving an import path for it.
- **A detector ships at warning severity.** It earns error severity by demonstrating precision
  on real code — not on fixtures its author wrote.
- Prefer a false negative. A detector that misses something is a gap; one that reports
  something false costs the reader their trust in every other finding.

## Why

Three false positives found by dogfooding on one project in one afternoon shared a structure:
the detector matched a shape and never asked whether the risk was reachable.

| Detector | Matched | Never asked |
|---|---|---|
| `sql_concatenation` | a keyword substring | is there any SQL here? |
| `global_mutable_state` | a package-level composite | is it written, and reachable? |
| `orphan_package` | a directory with `.go` files | is it our module? |

All three were error severity, so each failed a build. None could be fixed in the reporting
project's code: there was no query to parameterise, no way to express a lookup table without a
package-level `var`, and no way to make a vendored module stop being a separate module. The
only escapes were a waiver — which the tool's own guidance calls the wrong answer to a false
positive — or rewriting correct code to avoid a pattern.

That is the cost being avoided. Not the finding itself, but a user who cannot act on it and
learns to skim the section it appears in. One false positive teaches a reader to skim; a few
teach them to ignore the tool. A detector's value is bounded by the trust of the person
reading its output.

## Proven by

- `internal/engineclient/detector_restraint_test.go` — `TestNoDetectorFiresOnCleanCode`,
  over `testdata/detector-restraint`, which is seeded from real code that produced false
  positives, not from invented examples
- `internal/engineclient/severity_test.go` — `TestHeuristicDetectorsAreNotErrorSeverity`
- `engine/crates/engine-bin/src/antipatterns.rs` — `sql_concatenation_needs_evidence_of_sql`,
  `global_mutable_state_ignores_unmutated_unexported_tables`

A restraint fixture written from imagination catches only the false positives its author
already pictured. The ones that cost trust are the ones nobody pictured, so cases are added
from real projects as they are found.

---

**Status:** Active
**Established:** 2026-08-15
