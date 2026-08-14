use crate::pb::{
    self, CheckComplete, CheckRequest, EngineResponse, Finding, RuleStatus,
    engine_response::Payload, rule::Spec, rule_status::Status,
};
use globset::{Glob, GlobSet, GlobSetBuilder};
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

        let pattern = match Regex::new(&spec.pattern) {
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

        let must_contain = compile_optional(&spec.must_contain);
        let must_not_contain = compile_optional(&spec.must_not_contain);
        let file_must_contain = compile_optional(&spec.file_must_contain);

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

            // File-level prerequisite
            if let Some(ref fmc) = file_must_contain
                && !fmc.is_match(&content)
            {
                continue;
            }

            for (line_num, line) in content.lines().enumerate() {
                if !pattern.is_match(line) {
                    continue;
                }

                if let Some(ref mc) = must_contain
                    && !mc.is_match(line)
                {
                    continue;
                }

                if let Some(ref mnc) = must_not_contain
                    && mnc.is_match(line)
                {
                    continue;
                }

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
                        line: (line_num + 1) as u32,
                        column: 0,
                        message: rule.message.clone(),
                        r#match: line.trim().to_string(),
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

fn compile_optional(pattern: &str) -> Option<Regex> {
    if pattern.is_empty() {
        return None;
    }
    Regex::new(pattern).ok()
}

fn build_globset(patterns: &[String]) -> Option<GlobSet> {
    if patterns.is_empty() {
        return None;
    }
    let mut builder = GlobSetBuilder::new();
    for p in patterns {
        if let Ok(g) = Glob::new(p) {
            builder.add(g);
        }
    }
    builder.build().ok()
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

        // Skip hidden dirs, vendor, node_modules, target, .git
        if path.is_dir() {
            if name_str.starts_with('.')
                || name_str == "vendor"
                || name_str == "node_modules"
                || name_str == "target"
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
}
