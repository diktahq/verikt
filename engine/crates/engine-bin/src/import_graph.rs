use crate::pb::{
    self, CheckComplete, CheckRequest, EngineResponse, Finding, RuleStatus,
    engine_response::Payload,
    rule::Spec,
    rule_status::Status,
};
use globset::{Glob, GlobSet, GlobSetBuilder};
use petgraph::graph::{DiGraph, NodeIndex};
use std::collections::HashMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::time::Instant;
use tree_sitter::Parser;

/// A single directed edge: `from` package imports `to` package.
#[derive(Debug, Clone)]
struct Import {
    from_file: String,
    from_pkg: String,
    to_pkg: String,
}

/// Build an import graph from all Go files under the project root.
/// Returns (imports, graph_indices) for rule evaluation.
pub fn handle_import_graph_check(req: &CheckRequest) -> Vec<EngineResponse> {
    let start = Instant::now();
    let project = PathBuf::from(&req.project_path);

    // Collect import_graph rules.
    let ig_rules: Vec<_> = req
        .rules
        .iter()
        .filter(|r| matches!(&r.spec, Some(Spec::ImportGraph(_))))
        .collect();

    if ig_rules.is_empty() {
        return vec![];
    }

    // Walk Go files and extract imports.
    let go_files = collect_go_files(&project, &req.target_files);
    let imports = extract_all_imports(&project, &go_files);

    // Build adjacency map: package -> Vec<(from_file, to_pkg)>
    let mut pkg_imports: HashMap<String, Vec<(String, String)>> = HashMap::new();
    for imp in &imports {
        pkg_imports
            .entry(imp.from_pkg.clone())
            .or_default()
            .push((imp.from_file.clone(), imp.to_pkg.clone()));
    }

    let mut responses: Vec<EngineResponse> = Vec::new();
    let mut rule_statuses: Vec<RuleStatus> = Vec::new();
    let mut findings_total: u32 = 0;
    let mut findings_error: u32 = 0;
    let mut findings_warning: u32 = 0;

    for rule in &ig_rules {
        let spec = match &rule.spec {
            Some(Spec::ImportGraph(s)) => s,
            _ => continue,
        };

        let pkg_glob = match build_globset(&[spec.package_pattern.clone()]) {
            Some(g) => g,
            None => {
                rule_statuses.push(RuleStatus {
                    rule_id: rule.id.clone(),
                    status: Status::Invalid.into(),
                    error: "invalid package_pattern glob".to_string(),
                });
                continue;
            }
        };

        let forbidden_globs: Vec<GlobSet> = spec
            .forbidden
            .iter()
            .filter_map(|p| build_globset(&[p.clone()]))
            .collect();

        let allowed_globs: Vec<GlobSet> = spec
            .allowed_only
            .iter()
            .filter_map(|p| build_globset(&[p.clone()]))
            .collect();

        let mut rule_matched = false;

        for (pkg, pkg_imp_list) in &pkg_imports {
            // Only check packages matching the package_pattern.
            let pkg_rel = strip_module_prefix(pkg, &req.project_path);
            if !pkg_glob.is_match(&pkg_rel) {
                continue;
            }

            for (from_file, to_pkg) in pkg_imp_list {
                let to_rel = strip_module_prefix(to_pkg, &req.project_path);

                // Check forbidden patterns.
                let is_forbidden = forbidden_globs.iter().any(|g| g.is_match(&to_rel));
                if is_forbidden {
                    let is_allowed = !allowed_globs.is_empty()
                        && allowed_globs.iter().all(|g| g.is_match(&to_rel));
                    if !is_allowed {
                        rule_matched = true;
                        findings_total += 1;
                        match rule.severity {
                            s if s == pb::Severity::Error as i32 => findings_error += 1,
                            s if s == pb::Severity::Warning as i32 => findings_warning += 1,
                            _ => {}
                        }
                        responses.push(EngineResponse {
                            payload: Some(Payload::Finding(Finding {
                                rule_id: rule.id.clone(),
                                severity: rule.severity,
                                file: from_file.clone(),
                                line: 0,
                                column: 0,
                                message: format!(
                                    "{} must not import {}",
                                    pkg_rel, to_rel
                                ),
                                r#match: format!("import \"{}\"", to_pkg),
                                engine: "import_graph".to_string(),
                            })),
                        });
                    }
                }

                // Check allowed_only — anything not in the allowed list is a violation.
                if !allowed_globs.is_empty() {
                    let is_allowed = allowed_globs.iter().any(|g| g.is_match(&to_rel));
                    // Only flag internal imports — stdlib and external are not constrained here.
                    let is_internal = to_pkg.contains(&extract_module_root(&req.project_path));
                    if is_internal && !is_allowed && !is_forbidden {
                        rule_matched = true;
                        findings_total += 1;
                        match rule.severity {
                            s if s == pb::Severity::Error as i32 => findings_error += 1,
                            s if s == pb::Severity::Warning as i32 => findings_warning += 1,
                            _ => {}
                        }
                        responses.push(EngineResponse {
                            payload: Some(Payload::Finding(Finding {
                                rule_id: rule.id.clone(),
                                severity: rule.severity,
                                file: from_file.clone(),
                                line: 0,
                                column: 0,
                                message: format!(
                                    "{} imports {} which is not in the allowed list",
                                    pkg_rel, to_rel
                                ),
                                r#match: format!("import \"{}\"", to_pkg),
                                engine: "import_graph".to_string(),
                            })),
                        });
                    }
                }
            }
        }

        rule_statuses.push(RuleStatus {
            rule_id: rule.id.clone(),
            status: if rule_matched {
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
            files_checked: go_files.len() as u32,
            rules_evaluated: ig_rules.len() as u32,
            findings_total,
            findings_error,
            findings_warning,
            findings_info: 0,
            duration_ms,
            rule_statuses,
        })),
    });

    responses
}

/// Extract imports from a single Go file using tree-sitter.
fn extract_imports_from_file(_path: &Path, source: &str) -> Vec<String> {
    let mut parser = Parser::new();
    parser
        .set_language(&tree_sitter_go::LANGUAGE.into())
        .expect("tree-sitter-go language load failed");

    let tree = match parser.parse(source, None) {
        Some(t) => t,
        None => return vec![],
    };

    let root = tree.root_node();
    let mut imports = Vec::new();
    collect_imports(root, source, &mut imports);
    imports
}

/// Recursively collect import path strings from the AST.
/// Uses the named "path" field on import_spec — version-stable across tree-sitter-go.
fn collect_imports(node: tree_sitter::Node, source: &str, imports: &mut Vec<String>) {
    if node.kind() == "import_spec" {
        if let Some(path_node) = node.child_by_field_name("path") {
            let raw = &source[path_node.byte_range()];
            let trimmed = raw.trim_matches('"').trim_matches('`').to_string();
            if !trimmed.is_empty() {
                imports.push(trimmed);
            }
        }
        return; // no need to recurse into import_spec children
    }
    for i in 0..node.child_count() {
        collect_imports(node.child(i).unwrap(), source, imports);
    }
}

/// Extract the package path from a Go file (directory relative to project root).
pub(crate) fn file_to_package(file_path: &Path, project_root: &Path) -> String {
    if let Some(parent) = file_path.parent() {
        let rel = parent.strip_prefix(project_root).unwrap_or(parent);
        rel.to_string_lossy().replace('\\', "/")
    } else {
        String::new()
    }
}

/// Walk all Go files under the project, respecting target_files filter.
pub(crate) fn collect_go_files(project: &Path, target_files: &[String]) -> Vec<PathBuf> {
    if !target_files.is_empty() {
        return target_files
            .iter()
            .filter(|f| f.ends_with(".go"))
            .map(|f| project.join(f))
            .collect();
    }

    let mut files = Vec::new();
    collect_go_files_recursive(project, project, &mut files);
    files
}

fn collect_go_files_recursive(root: &Path, dir: &Path, files: &mut Vec<PathBuf>) {
    let entries = match fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };
    for entry in entries.flatten() {
        let path = entry.path();
        let name = entry.file_name();
        let name_str = name.to_string_lossy();
        if path.is_dir() {
            if name_str.starts_with('.') || name_str == "vendor" || name_str == "target" {
                continue;
            }
            collect_go_files_recursive(root, &path, files);
        } else if name_str.ends_with(".go") {
            files.push(path);
        }
    }
}

fn extract_all_imports(project: &Path, files: &[PathBuf]) -> Vec<Import> {
    let mut all_imports = Vec::new();
    for file_path in files {
        let content = match fs::read_to_string(file_path) {
            Ok(c) => c,
            Err(_) => continue,
        };
        let from_pkg = file_to_package(file_path, project);
        let rel_file = file_path
            .strip_prefix(project)
            .unwrap_or(file_path)
            .to_string_lossy()
            .replace('\\', "/");

        for to_pkg in extract_imports_from_file(file_path, &content) {
            all_imports.push(Import {
                from_file: rel_file.clone(),
                from_pkg: from_pkg.clone(),
                to_pkg,
            });
        }
    }
    all_imports
}

/// Strip the Go module root prefix from a package path, returning the relative path.
/// e.g. "github.com/dcsg/archway/internal/cli" -> "internal/cli"
fn strip_module_prefix(pkg: &str, project_path: &str) -> String {
    let module_root = extract_module_root(project_path);
    if let Some(stripped) = pkg.strip_prefix(&module_root) {
        stripped.trim_start_matches('/').to_string()
    } else {
        pkg.to_string()
    }
}

/// Extract the Go module name from go.mod in the project root.
/// Falls back to the last path component of project_path.
fn extract_module_root(project_path: &str) -> String {
    let go_mod = PathBuf::from(project_path).join("go.mod");
    if let Ok(content) = fs::read_to_string(&go_mod) {
        for line in content.lines() {
            if let Some(module) = line.strip_prefix("module ") {
                return module.trim().to_string();
            }
        }
    }
    // Fallback: use last path segment.
    PathBuf::from(project_path)
        .file_name()
        .map(|n| n.to_string_lossy().to_string())
        .unwrap_or_default()
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
        // Also match the bare directory when pattern ends with /**
        // so that "service/**" matches both "service" and "service/sub".
        if let Some(bare) = p.strip_suffix("/**") {
            if let Ok(g) = Glob::new(bare) {
                builder.add(g);
            }
        }
    }
    builder.build().ok()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn extract_single_import() {
        let src = "package main\n\nimport \"fmt\"\n";
        let imports = extract_imports_from_file(Path::new("main.go"), src);
        assert_eq!(imports, vec!["fmt"]);
    }

    #[test]
    fn extract_grouped_imports() {
        let src = "package main\n\nimport (\n    \"fmt\"\n    \"os\"\n    \"github.com/dcsg/archway/internal/scaffold\"\n)\n";
        let mut imports = extract_imports_from_file(Path::new("main.go"), src);
        imports.sort();
        assert_eq!(imports, vec![
            "fmt",
            "github.com/dcsg/archway/internal/scaffold",
            "os",
        ]);
    }

    #[test]
    fn extract_aliased_imports() {
        let src = "package main\n\nimport (\n    log \"github.com/rs/zerolog\"\n    _ \"github.com/lib/pq\"\n)\n";
        let mut imports = extract_imports_from_file(Path::new("main.go"), src);
        imports.sort();
        assert_eq!(imports, vec!["github.com/lib/pq", "github.com/rs/zerolog"]);
    }
}

// Keep petgraph in scope — used for future cycle detection.
#[allow(dead_code)]
fn build_digraph(imports: &[Import]) -> DiGraph<String, ()> {
    let mut graph = DiGraph::new();
    let mut indices: HashMap<String, NodeIndex> = HashMap::new();

    let get_or_insert = |g: &mut DiGraph<String, ()>, indices: &mut HashMap<String, NodeIndex>, pkg: &str| -> NodeIndex {
        if let Some(&idx) = indices.get(pkg) {
            idx
        } else {
            let idx = g.add_node(pkg.to_string());
            indices.insert(pkg.to_string(), idx);
            idx
        }
    };

    for imp in imports {
        let from = get_or_insert(&mut graph, &mut indices, &imp.from_pkg);
        let to = get_or_insert(&mut graph, &mut indices, &imp.to_pkg);
        graph.add_edge(from, to, ());
    }

    graph
}
