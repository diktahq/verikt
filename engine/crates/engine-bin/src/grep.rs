use crate::globs::build_globset;
use crate::pb::{
    self, CheckComplete, CheckRequest, EngineResponse, Finding, RuleStatus,
    engine_response::Payload, rule::Spec, rule_status::Status,
};
use regex::Regex;
use std::fs;
use std::path::{Path, PathBuf};
use std::time::Instant;

pub fn handle_check(req: CheckRequest) -> Vec<EngineResponse> {
    let start = Instant::now();
    let mut responses: Vec<EngineResponse> = Vec::new();
    let mut rule_statuses: Vec<RuleStatus> = Vec::new();
    let mut findings_total: u32 = 0;
    let mut findings_error: u32 = 0;
    let mut findings_warning: u32 = 0;
    let mut findings_info: u32 = 0;

    let project = match PathBuf::from(&req.project_path).canonicalize() {
        Ok(p) => p,
        Err(e) => {
            return vec![EngineResponse {
                payload: Some(Payload::Error(pb::EngineError {
                    message: format!("invalid project_path: {e}"),
                    code: "INVALID_PROJECT_PATH".to_string(),
                })),
            }];
        }
    };

    // Collect target files — either explicit list or walk project.
    // All paths are validated to stay within the project boundary.
    let files = if req.target_files.is_empty() {
        walk_files(&project)
    } else {
        req.target_files
            .iter()
            .filter_map(|f| {
                let resolved = project.join(f);
                match resolved.canonicalize() {
                    Ok(p) if p.starts_with(&project) => Some(p),
                    _ => None, // skip paths outside project boundary
                }
            })
            .collect()
    };

    let files_checked = files.len() as u32;

    for rule in &req.rules {
        let spec = match &rule.spec {
            Some(Spec::Grep(g)) => g,
            _ => continue, // non-grep rules handled by other modules
        };

        // An empty pattern means "no line check" (a file-must-contain-only
        // rule), not a regex that matches every line.
        let pattern = match compile_pattern(&spec.pattern) {
            Ok(p) => p,
            Err(e) => {
                rule_statuses.push(RuleStatus {
                    rule_id: rule.id.clone(),
                    status: Status::Invalid.into(),
                    error: format!("bad regex: {e}"),
                });
                continue;
            }
        };

        let matchers = Matchers {
            pattern,
            must_contain: compile_optional(&spec.must_contain),
            must_not_contain: compile_optional(&spec.must_not_contain),
            file_must_contain: compile_optional(&spec.file_must_contain),
            file_must_contain_src: &spec.file_must_contain,
        };

        // Build scope globs
        let include_set = build_globset(&rule.scope.as_ref().map_or(vec![], |s| s.include.clone()));
        let exclude_set = build_globset(&rule.scope.as_ref().map_or(vec![], |s| s.exclude.clone()));

        // Staleness is about reach, not results: a rule is stale when its scope
        // expands to no files, which is the same definition the Go loader applies
        // in ExpandScope. Counting findings here instead meant a rule that ran
        // cleanly was reported as broken, and stale rules fail the build.
        let mut files_in_scope: u32 = 0;

        for file_path in &files {
            let rel = file_path.strip_prefix(&project).unwrap_or(file_path);
            let rel_str = rel.to_string_lossy();

            // Scope filtering
            if let Some(ref inc) = include_set
                && !inc.is_match(rel)
            {
                continue;
            }
            if let Some(ref exc) = exclude_set
                && exc.is_match(rel)
            {
                continue;
            }

            files_in_scope += 1;

            let content = match fs::read_to_string(file_path) {
                Ok(c) => c,
                Err(_) => continue, // skip binary/unreadable files
            };

            for (line, text) in matchers.violations(&content) {
                findings_total += 1;
                match rule.severity {
                    s if s == pb::Severity::Error as i32 => findings_error += 1,
                    s if s == pb::Severity::Warning as i32 => findings_warning += 1,
                    s if s == pb::Severity::Info as i32 => findings_info += 1,
                    _ => {}
                }

                responses.push(EngineResponse {
                    payload: Some(Payload::Finding(Finding {
                        rule_id: rule.id.clone(),
                        severity: rule.severity,
                        file: rel_str.to_string(),
                        line,
                        column: 0,
                        message: rule.message.clone(),
                        r#match: text,
                        engine: "grep".to_string(),
                    })),
                });
            }
        }

        rule_statuses.push(RuleStatus {
            rule_id: rule.id.clone(),
            status: if files_in_scope > 0 {
                Status::Valid
            } else {
                Status::Stale
            }
            .into(),
            error: String::new(),
        });
    }

    let duration_ms = start.elapsed().as_secs_f64() * 1000.0;

    responses.push(EngineResponse {
        payload: Some(Payload::CheckComplete(CheckComplete {
            files_checked,
            rules_evaluated: req.rules.len() as u32,
            findings_total,
            findings_error,
            findings_warning,
            findings_info,
            duration_ms,
            rule_statuses,
        })),
    });

    responses
}

/// The compiled regexes of one grep rule.
///
/// The semantics mirror the Go implementation (internal/rules/grep.go), and the
/// Go/Rust parity tests hold the two to it:
///
/// - `file_must_contain` that does not match the file is one finding at line 0.
///   It does not stop the line check.
/// - A line matching `pattern` is a finding when it lacks `must_contain`, or
///   has `must_not_contain`, or — when neither is set — by matching alone.
struct Matchers<'a> {
    pattern: Option<Regex>,
    must_contain: Option<Regex>,
    must_not_contain: Option<Regex>,
    file_must_contain: Option<Regex>,
    file_must_contain_src: &'a str,
}

impl Matchers<'_> {
    /// Every finding in one file, as (line, match text). Line 0 is the file.
    fn violations(&self, content: &str) -> Vec<(u32, String)> {
        let mut out = Vec::new();
        if let Some(fmc) = &self.file_must_contain
            && !fmc.is_match(content)
        {
            out.push((
                0,
                format!(
                    "file does not contain pattern: {}",
                    self.file_must_contain_src
                ),
            ));
        }
        let Some(pattern) = &self.pattern else {
            return out;
        };
        for (idx, line) in content.lines().enumerate() {
            if pattern.is_match(line) && self.line_violates(line) {
                out.push(((idx + 1) as u32, match_text(line)));
            }
        }
        out
    }

    fn line_violates(&self, line: &str) -> bool {
        if let Some(mc) = &self.must_contain
            && !mc.is_match(line)
        {
            return true;
        }
        if let Some(mnc) = &self.must_not_contain
            && mnc.is_match(line)
        {
            return true;
        }
        self.must_contain.is_none() && self.must_not_contain.is_none()
    }
}

/// Longest source line a finding echoes, in bytes, as in the Go implementation.
const MAX_MATCH_BYTES: usize = 120;

/// The line a finding echoes: trimmed, and cut at MAX_MATCH_BYTES on a char
/// boundary. It used to be echoed whole, however long.
fn match_text(line: &str) -> String {
    let line = line.trim();
    if line.len() <= MAX_MATCH_BYTES {
        return line.to_string();
    }
    let mut end = MAX_MATCH_BYTES;
    while !line.is_char_boundary(end) {
        end -= 1;
    }
    format!("{}...", &line[..end])
}

fn compile_pattern(pattern: &str) -> Result<Option<Regex>, regex::Error> {
    if pattern.is_empty() {
        return Ok(None);
    }
    Regex::new(pattern).map(Some)
}

fn compile_optional(pattern: &str) -> Option<Regex> {
    if pattern.is_empty() {
        return None;
    }
    Regex::new(pattern).ok()
}

/// True if the entry is itself a symbolic link, without following it.
///
/// Used to honour INV-002: a symlinked directory is not project-local code, and
/// following one can escape the project or loop forever.
pub(crate) fn is_symlink(entry: &fs::DirEntry) -> bool {
    entry.file_type().map(|ft| ft.is_symlink()).unwrap_or(false)
}

fn walk_files(root: &Path) -> Vec<PathBuf> {
    let mut files = Vec::new();
    walk_dir(root, &mut files);
    files
}

fn walk_dir(dir: &Path, files: &mut Vec<PathBuf>) {
    let entries = match fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let path = entry.path();
        let name = entry.file_name();
        let name_str = name.to_string_lossy();

        // A symlink of any kind points outside the project boundary and is never
        // project-local code (INV-002). Tested before the directory check because
        // `is_dir()` follows symlinks.
        if is_symlink(&entry) {
            continue;
        }

        // Skip hidden dirs, vendor, node_modules, target, .git, testdata.
        //
        // testdata is skipped for the same reason the Go and TypeScript
        // collectors skip it: fixtures contain the patterns rules look for, on
        // purpose. Without it the same directory was simultaneously out of scope
        // for every detector and in scope for every proxy grep rule, so a
        // fixture holding a deliberate `fmt.Sprintf("SELECT ...")` failed the
        // build at error severity while the detectors correctly ignored it.
        //
        // A fixture is still greppable by pointing --path at it, which is the
        // same escape hatch the detectors have.
        //
        // `_`-prefixed directories are ignored by the Go toolchain too, so code
        // there is not part of the build and should not be part of the check.
        if path.is_dir() {
            if name_str.starts_with('.')
                || name_str.starts_with('_')
                || name_str == "vendor"
                || name_str == "node_modules"
                || name_str == "target"
                || name_str == "testdata"
            {
                continue;
            }
            walk_dir(&path, files);
        } else {
            files.push(path);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pb::{GrepSpec, Rule, RuleScope};

    fn fixture(name: &str, files: &[(&str, &str)]) -> PathBuf {
        let base = std::env::temp_dir().join(name);
        let _ = fs::remove_dir_all(&base);
        fs::create_dir_all(&base).unwrap();
        for (rel, content) in files {
            let path = base.join(rel);
            fs::create_dir_all(path.parent().unwrap()).unwrap();
            fs::write(path, content).unwrap();
        }
        base
    }

    fn grep_rule(id: &str, pattern: &str, include: &[&str]) -> Rule {
        Rule {
            id: id.to_string(),
            severity: pb::Severity::Error as i32,
            message: "test rule".to_string(),
            engine: pb::EngineType::Grep as i32,
            scope: Some(RuleScope {
                language: String::new(),
                include: include.iter().map(|s| s.to_string()).collect(),
                exclude: vec![],
            }),
            spec: Some(Spec::Grep(GrepSpec {
                pattern: pattern.to_string(),
                must_contain: String::new(),
                must_not_contain: String::new(),
                file_must_contain: String::new(),
            })),
        }
    }

    fn status_of(responses: &[EngineResponse], rule_id: &str) -> i32 {
        for response in responses {
            if let Some(Payload::CheckComplete(complete)) = &response.payload {
                for status in &complete.rule_statuses {
                    if status.rule_id == rule_id {
                        return status.status;
                    }
                }
            }
        }
        panic!("no status reported for rule {rule_id}");
    }

    fn check(project: &Path, rule: Rule) -> Vec<EngineResponse> {
        handle_check(CheckRequest {
            project_path: project.to_string_lossy().to_string(),
            rules: vec![rule],
            target_files: vec![],
        })
    }

    /// A rule that ran over its scope and found nothing has passed. Stale means
    /// the scope matched no files — a rule that could not run at all.
    ///
    /// These were conflated: `rule_matched` was only set when a finding was
    /// emitted. Because a stale rule fails the build, every clean repository
    /// using proxy rules exited 1, and the closer a codebase was to compliant
    /// the more of its rules were reported broken.
    #[test]
    fn rule_that_finds_nothing_in_scope_is_valid_not_stale() {
        let project = fixture(
            "verikt-grep-clean-scope",
            &[("internal/agent/a.go", "package agent\n")],
        );

        let responses = check(
            &project,
            grep_rule("no-sprintf", "Sprintf", &["internal/**/*.go"]),
        );

        assert_eq!(
            status_of(&responses, "no-sprintf"),
            Status::Valid as i32,
            "a rule whose scope matched a file has run, regardless of findings"
        );

        let _ = fs::remove_dir_all(&project);
    }

    /// The counterpart: a scope that expands to nothing is genuinely stale, and
    /// must stay stale — that signal is why the status exists.
    #[test]
    fn rule_whose_scope_matches_no_files_is_stale() {
        let project = fixture(
            "verikt-grep-empty-scope",
            &[("internal/agent/a.go", "package agent\n")],
        );

        let responses = check(&project, grep_rule("gone", "Sprintf", &["removed/**/*.go"]));

        assert_eq!(
            status_of(&responses, "gone"),
            Status::Stale as i32,
            "a scope matching no files is a rule that did not run"
        );

        let _ = fs::remove_dir_all(&project);
    }

    /// Fixtures are out of scope for grep rules, as they are for detectors.
    ///
    /// testdata holds the patterns rules look for on purpose. The Go and
    /// TypeScript collectors skip it and this walk did not, so the same
    /// directory was simultaneously invisible to every detector and visible to
    /// every proxy rule.
    #[test]
    fn walk_skips_testdata_and_underscore_dirs() {
        let project = fixture(
            "verikt-grep-testdata-skip",
            &[
                ("internal/real.go", "package p\nvar _ = Sprintf\n"),
                (
                    "internal/testdata/fixture/bad.go",
                    "package q\nvar _ = Sprintf\n",
                ),
                ("_scratch/old.go", "package r\nvar _ = Sprintf\n"),
                ("vendor/dep.go", "package s\nvar _ = Sprintf\n"),
            ],
        );

        let files = walk_files(&project);
        let names: Vec<String> = files
            .iter()
            .map(|f| f.file_name().unwrap().to_string_lossy().to_string())
            .collect();

        assert_eq!(names, vec!["real.go".to_string()], "got {names:?}");

        let _ = fs::remove_dir_all(&project);
    }

    /// A rule with findings is valid too — the case that accidentally worked.
    #[test]
    fn rule_with_findings_is_valid() {
        let project = fixture(
            "verikt-grep-findings",
            &[("internal/agent/a.go", "package agent\nvar _ = Sprintf\n")],
        );

        let responses = check(
            &project,
            grep_rule("no-sprintf", "Sprintf", &["internal/**/*.go"]),
        );

        assert_eq!(status_of(&responses, "no-sprintf"), Status::Valid as i32);

        let _ = fs::remove_dir_all(&project);
    }

    fn spec_rule(id: &str, spec: GrepSpec, include: &[&str]) -> Rule {
        let mut rule = grep_rule(id, "", include);
        rule.spec = Some(Spec::Grep(spec));
        rule
    }

    fn spec(pattern: &str) -> GrepSpec {
        GrepSpec {
            pattern: pattern.to_string(),
            must_contain: String::new(),
            must_not_contain: String::new(),
            file_must_contain: String::new(),
        }
    }

    /// Every finding as "file:line:match", sorted.
    fn findings(responses: &[EngineResponse]) -> Vec<String> {
        let mut out: Vec<String> = responses
            .iter()
            .filter_map(|r| match &r.payload {
                Some(Payload::Finding(f)) => Some(format!("{}:{}:{}", f.file, f.line, f.r#match)),
                _ => None,
            })
            .collect();
        out.sort();
        out
    }

    const KAFKA: &str = "package k\n\nfunc bad() { msg, _ := reader.ReadMessage(ctx) }\nfunc good() { msg, err := reader.ReadMessage(ctx) }\n";

    /// must-contain names text a matching line is required to have, so the
    /// finding is the line that lacks it. The engine skipped that line and
    /// reported the compliant one.
    #[test]
    fn must_contain_reports_the_line_that_lacks_it() {
        let project = fixture("verikt-grep-must-contain", &[("k.go", KAFKA)]);
        let mut s = spec(r"ReadMessage\(");
        s.must_contain = "err :?=".to_string();

        let got = findings(&check(&project, spec_rule("kafka-err", s, &["**/*.go"])));

        assert_eq!(
            got,
            vec!["k.go:3:func bad() { msg, _ := reader.ReadMessage(ctx) }"]
        );
        let _ = fs::remove_dir_all(&project);
    }

    /// must-not-contain is the inverse: the finding is the line that has it.
    #[test]
    fn must_not_contain_reports_the_line_that_has_it() {
        let project = fixture("verikt-grep-must-not-contain", &[("k.go", KAFKA)]);
        let mut s = spec(r"ReadMessage\(");
        s.must_not_contain = ", _ :=".to_string();

        let got = findings(&check(&project, spec_rule("kafka-err", s, &["**/*.go"])));

        assert_eq!(
            got,
            vec!["k.go:3:func bad() { msg, _ := reader.ReadMessage(ctx) }"]
        );
        let _ = fs::remove_dir_all(&project);
    }

    /// file-must-contain reports, once and at line 0, the file that lacks the
    /// text. With no pattern there is no line check: an empty pattern used to
    /// match every line of every compliant file.
    #[test]
    fn file_must_contain_reports_only_the_file_that_lacks_it() {
        let project = fixture(
            "verikt-grep-file-must-contain",
            &[
                ("proto/ok.proto", "syntax = \"proto3\";\npackage y;\n"),
                ("proto/missing.proto", "package z;\n"),
            ],
        );
        let mut s = spec("");
        s.file_must_contain = r#"syntax = "proto3""#.to_string();

        let got = findings(&check(&project, spec_rule("proto3", s, &["**/*.proto"])));

        assert_eq!(
            got,
            vec![r#"proto/missing.proto:0:file does not contain pattern: syntax = "proto3""#]
        );
        let _ = fs::remove_dir_all(&project);
    }

    /// A missing file-must-contain does not stop the line check in that file.
    #[test]
    fn file_must_contain_and_pattern_are_both_reported() {
        let project = fixture(
            "verikt-grep-fmc-and-pattern",
            &[("a.go", "package a\n\nvar _ = Sprintf\n")],
        );
        let mut s = spec("Sprintf");
        s.file_must_contain = "MUST-BE-HERE".to_string();

        let got = findings(&check(&project, spec_rule("both", s, &["**/*.go"])));

        assert_eq!(
            got,
            vec![
                "a.go:0:file does not contain pattern: MUST-BE-HERE",
                "a.go:3:var _ = Sprintf"
            ]
        );
        let _ = fs::remove_dir_all(&project);
    }

    /// The echoed line is trimmed and cut at 120 bytes on a char boundary, as
    /// the Go implementation does. It was echoed whole, however long.
    #[test]
    fn match_text_is_trimmed_and_cut_on_a_char_boundary() {
        let head = format!("Sprintf{}", "x".repeat(119 - "Sprintf".len()));
        let line = format!("\t\t{head}é{}", "y".repeat(50));
        let content = format!("package a\n\n{line}\n");
        let project = fixture("verikt-grep-match-text", &[("a.go", content.as_str())]);

        let got = findings(&check(
            &project,
            grep_rule("no-sprintf", "Sprintf", &["**/*.go"]),
        ));

        assert_eq!(got, vec![format!("a.go:3:{head}...")]);
        let _ = fs::remove_dir_all(&project);
    }
}
