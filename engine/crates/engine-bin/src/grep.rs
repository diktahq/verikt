use crate::pb::{
    self, CheckComplete, CheckRequest, EngineResponse, Finding, RuleStatus,
    engine_response::Payload,
    rule::Spec,
    rule_status::Status,
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

        let mut rule_matched = false;

        for file_path in &files {
            let rel = file_path.strip_prefix(&project).unwrap_or(file_path);
            let rel_str = rel.to_string_lossy();

            // Scope filtering
            if let Some(ref inc) = include_set {
                if !inc.is_match(rel) {
                    continue;
                }
            }
            if let Some(ref exc) = exclude_set {
                if exc.is_match(rel) {
                    continue;
                }
            }

            let content = match fs::read_to_string(file_path) {
                Ok(c) => c,
                Err(_) => continue, // skip binary/unreadable files
            };

            // File-level prerequisite
            if let Some(ref fmc) = file_must_contain {
                if !fmc.is_match(&content) {
                    continue;
                }
            }

            for (line_num, line) in content.lines().enumerate() {
                if !pattern.is_match(line) {
                    continue;
                }

                if let Some(ref mc) = must_contain {
                    if !mc.is_match(line) {
                        continue;
                    }
                }

                if let Some(ref mnc) = must_not_contain {
                    if mnc.is_match(line) {
                        continue;
                    }
                }

                rule_matched = true;
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
            status: if rule_matched { Status::Valid } else { Status::Stale }.into(),
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

fn walk_files(root: &Path) -> Vec<PathBuf> {
    let mut files = Vec::new();
    walk_dir(root, root, &mut files);
    files
}

fn walk_dir(root: &Path, dir: &Path, files: &mut Vec<PathBuf>) {
    let entries = match fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let path = entry.path();
        let name = entry.file_name();
        let name_str = name.to_string_lossy();

        // Skip hidden dirs, vendor, node_modules, target, .git
        if path.is_dir() {
            if name_str.starts_with('.')
                || name_str == "vendor"
                || name_str == "node_modules"
                || name_str == "target"
            {
                continue;
            }
            walk_dir(root, &path, files);
        } else {
            files.push(path);
        }
    }
}
