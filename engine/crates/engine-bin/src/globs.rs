//! The one place a verikt.yaml glob pattern is turned into a matcher on the Rust
//! side.
//!
//! There were two copies, and they behaved differently: import_graph's added a
//! bare variant for `dir/**` while grep's did not, and neither set
//! `literal_separator`, so `*` crossed path separators in both. The Go side
//! (internal/pathglob) exists for the same reason — five copies of the matcher
//! there disagreed about `in: ["**"]`, one of them reporting every package in a
//! project as an orphan.
//!
//! Semantics are specified by internal/pathglob/testdata/parity-cases.txt, which
//! both languages read. `glob_parity_with_the_go_matcher` in this module checks
//! this side against it; `TestGoMatcherAgreesWithSharedParityFixture` checks the
//! other. When the fixture was first written, this side diverged on 5 of 35
//! cases.

use globset::{GlobBuilder, GlobSet, GlobSetBuilder};

/// Build a matcher for a set of verikt.yaml patterns, or None if none compile.
pub(crate) fn build_globset(patterns: &[String]) -> Option<GlobSet> {
    if patterns.is_empty() {
        return None;
    }
    let mut builder = GlobSetBuilder::new();
    let mut added = 0usize;
    for p in patterns {
        for variant in glob_variants(p) {
            // literal_separator(true) stops `*` and `?` crossing a path
            // separator. Without it globset's `*` spanned separators while the Go
            // matcher's did not, so "internal/*/store" claimed
            // "internal/a/b/store" and "*.go" claimed "internal/main.go" here but
            // not there — this side matching more than the author wrote, which
            // silences findings rather than adding them.
            if let Ok(g) = GlobBuilder::new(&variant).literal_separator(true).build() {
                builder.add(g);
                added += 1;
            }
        }
    }
    if added == 0 {
        return None;
    }
    builder.build().ok()
}

/// Expand one pattern into the globs that together give pathglob's semantics.
///
/// globset alone is not those semantics, and the gap was silent: component
/// patterns are matched by pathglob for orphan_package and missing_component but
/// by globset for dependency and layer rules, so `in: ["internal"]` claimed every
/// package on the Go side and nothing here — layer violations in that component
/// went unreported.
fn glob_variants(pattern: &str) -> Vec<String> {
    // A trailing slash names a directory and covers the tree below it.
    let pattern = match pattern.strip_suffix('/') {
        Some(dir) => format!("{dir}/**"),
        None => pattern.to_string(),
    };

    let mut out = vec![pattern.clone()];
    match pattern.strip_suffix("/**") {
        // globset's "dir/**" requires at least one further segment, so the
        // directory itself needs the bare glob too.
        Some(bare) if !bare.is_empty() => out.push(bare.to_string()),
        // A bare name carrying no metacharacters also covers the tree below it.
        None if !pattern.contains(['*', '?', '[']) => out.push(format!("{pattern}/**")),
        _ => {}
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    /// This side agrees with the same fixture the Go matcher is checked against,
    /// in internal/pathglob/testdata/parity-cases.txt.
    ///
    /// Component `in:` patterns are matched by two implementations on opposite
    /// sides of the protobuf boundary: pathglob in Go decides orphan_package and
    /// missing_component, globset here decides dependency and layer rules, and
    /// both decide proxy rule scopes. pathglob's package comment asserted the two
    /// agree; nothing checked it, and that is how five Go matchers came to
    /// disagree about `in: ["**"]` while each carried a comment claiming
    /// otherwise.
    ///
    /// Both sides read one written-down specification rather than comparing to
    /// each other, so a divergence names which side is wrong.
    #[test]
    fn glob_parity_with_the_go_matcher() {
        let fixture = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("../../../internal/pathglob/testdata/parity-cases.txt");
        let content = std::fs::read_to_string(&fixture)
            .unwrap_or_else(|e| panic!("cannot read {}: {e}", fixture.display()));

        let mut checked = 0usize;
        let mut divergences: Vec<String> = Vec::new();

        for (i, line) in content.lines().enumerate() {
            let trimmed = line.trim();
            if trimmed.is_empty() || trimmed.starts_with('#') {
                continue;
            }
            let fields: Vec<&str> = line.split('\t').collect();
            assert_eq!(fields.len(), 3, "line {} is malformed: {line:?}", i + 1);

            let pattern = fields[0].trim();
            let path = fields[1].trim();
            let want = match fields[2].trim() {
                "match" => true,
                "no-match" => false,
                other => panic!("line {}: expected match|no-match, got {other:?}", i + 1),
            };

            let set = build_globset(std::slice::from_ref(&pattern.to_string()))
                .unwrap_or_else(|| panic!("line {}: globset rejected {pattern:?}", i + 1));
            let got = set.is_match(path);
            checked += 1;

            if got != want {
                divergences.push(format!(
                    "  line {}: pattern {pattern:?} vs path {path:?} — this side says {got}, fixture says {want}",
                    i + 1
                ));
            }
        }

        assert!(checked > 0, "no cases loaded; this test guards nothing");
        assert!(
            divergences.is_empty(),
            "this side diverges from the shared glob specification on {} of {checked} cases:\n{}",
            divergences.len(),
            divergences.join("\n")
        );
    }
}
