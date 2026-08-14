use crate::import_graph::{collect_go_files, file_to_package};
use crate::pb::{
    self, CheckComplete, CheckRequest, EngineResponse, Finding, RuleStatus,
    engine_response::Payload, rule::Spec, rule_status::Status,
};
use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;
use std::time::Instant;
use tree_sitter::{Node, Parser};

/// A pre-rule-assignment finding from a specific detector.
#[derive(Debug)]
struct DetFinding {
    detector: &'static str,
    file: String,
    line: u32,
    message: String,
}

pub fn handle_anti_pattern_check(req: &CheckRequest) -> Vec<EngineResponse> {
    let start = Instant::now();
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

    let ap_rules: Vec<_> = req
        .rules
        .iter()
        .filter(|r| matches!(&r.spec, Some(Spec::AntiPattern(_))))
        .collect();

    if ap_rules.is_empty() {
        return vec![];
    }

    let go_files = collect_go_files(&project, &req.target_files);

    let mut all_findings: Vec<DetFinding> = Vec::new();
    // god_packages: aggregate exported symbol counts per directory (dir_rel → count)
    let mut pkg_exports: HashMap<String, u32> = HashMap::new();
    // Cross-package state for the layering detectors, which cannot be decided
    // from a single file: package → its direct imports, and every package seen.
    let mut pkg_imports: HashMap<String, Vec<String>> = HashMap::new();

    for file_path in &go_files {
        let content = match fs::read_to_string(file_path) {
            Ok(c) => c,
            Err(_) => continue,
        };
        let rel_file = file_path
            .strip_prefix(&project)
            .unwrap_or(file_path)
            .to_string_lossy()
            .replace('\\', "/");

        // Anti-patterns in tests are acceptable — skip.
        if rel_file.ends_with("_test.go") {
            continue;
        }

        let pkg_path = file_to_package(file_path, &project);
        let source = content.as_bytes();

        let mut parser = Parser::new();
        parser
            .set_language(&tree_sitter_go::LANGUAGE.into())
            .expect("tree-sitter-go language load failed");
        let tree = match parser.parse(source, None) {
            Some(t) => t,
            None => continue,
        };
        let root = tree.root_node();

        all_findings.extend(detect_global_mutable_state(root, source, &rel_file));
        all_findings.extend(detect_init_abuse(root, source, &rel_file));
        all_findings.extend(detect_naked_goroutines(root, source, &rel_file));
        all_findings.extend(detect_swallowed_errors(root, source, &rel_file));
        all_findings.extend(detect_context_background(
            root, source, &rel_file, &pkg_path,
        ));
        all_findings.extend(detect_sql_concatenation(root, source, &rel_file));
        all_findings.extend(detect_uuid_v4_as_key(root, source, &rel_file));
        all_findings.extend(detect_fat_handlers(root, source, &rel_file, &pkg_path));
        all_findings.extend(detect_nil_map_write(root, source, &rel_file));
        all_findings.extend(detect_type_assertion_without_ok(root, source, &rel_file));

        // Exported symbol count for god_package detection.
        let dir_rel = file_path
            .parent()
            .and_then(|d| d.strip_prefix(&project).ok())
            .map(|d| d.to_string_lossy().replace('\\', "/"))
            .unwrap_or_default();
        let count = count_exported_symbols(root, source);
        *pkg_exports.entry(dir_rel).or_default() += count;

        pkg_imports.entry(pkg_path.clone()).or_default().extend(
            crate::import_graph::extract_imports_from_file(file_path, &content),
        );
    }

    // Cross-package layering detectors (post-pass: they need every package).
    let module_root = crate::import_graph::extract_module_root(&project.to_string_lossy());
    all_findings.extend(detect_domain_imports_adapter(&pkg_imports, &module_root));
    let mut packages: Vec<String> = pkg_imports.keys().cloned().collect();
    packages.sort();
    all_findings.extend(detect_mvc_in_hexagonal(&packages));

    // god_package findings (cross-file, per directory).
    for (dir, count) in &pkg_exports {
        if *count > 40 {
            all_findings.push(DetFinding {
                detector: "god_package",
                file: if dir.is_empty() {
                    ".".to_string()
                } else {
                    format!("{}/", dir)
                },
                line: 0,
                message: format!(
                    "package has {} exported symbols — consider splitting by responsibility",
                    count
                ),
            });
        }
    }

    // Match findings to rules, emit EngineResponse messages.
    let mut responses: Vec<EngineResponse> = Vec::new();
    let mut findings_total = 0u32;
    let mut findings_error = 0u32;
    let mut findings_warning = 0u32;
    let mut findings_info = 0u32;
    let mut rule_statuses: Vec<RuleStatus> = Vec::new();

    for rule in &ap_rules {
        let spec = match &rule.spec {
            Some(Spec::AntiPattern(s)) => s,
            _ => continue,
        };

        for f in &all_findings {
            // Empty detectors list = all detectors enabled.
            if !spec.detectors.is_empty() && !spec.detectors.iter().any(|d| d == f.detector) {
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
                    file: f.file.clone(),
                    line: f.line,
                    column: 0,
                    message: f.message.clone(),
                    r#match: f.detector.to_string(),
                    engine: "anti_pattern".to_string(),
                })),
            });
        }

        // Stale means the rule could not run — there was nothing in scope to
        // analyse. It does not mean the detectors found nothing: a clean project
        // is the success case, and reporting it as a broken rule is how the grep
        // engine came to fail every compliant repository.
        rule_statuses.push(RuleStatus {
            rule_id: rule.id.clone(),
            status: if go_files.is_empty() {
                Status::Stale
            } else {
                Status::Valid
            }
            .into(),
            error: String::new(),
        });
    }

    let duration_ms = start.elapsed().as_secs_f64() * 1000.0;

    responses.push(EngineResponse {
        payload: Some(Payload::CheckComplete(CheckComplete {
            files_checked: go_files.len() as u32,
            rules_evaluated: ap_rules.len() as u32,
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

// ─── Detectors ───────────────────────────────────────────────────────────────

/// Detect package-level mutable variables (maps, slices, channels, pointers,
/// composite literals, make() calls). Skips error sentinels and blank identifiers.
fn detect_global_mutable_state<'a>(root: Node<'a>, source: &[u8], file: &str) -> Vec<DetFinding> {
    let mut findings = Vec::new();

    for i in 0..root.named_child_count() {
        let node = match root.named_child(i) {
            Some(n) => n,
            None => continue,
        };
        if node.kind() != "var_declaration" {
            continue;
        }

        for j in 0..node.named_child_count() {
            let spec = match node.named_child(j) {
                Some(n) => n,
                None => continue,
            };
            if spec.kind() != "var_spec" {
                continue;
            }

            let mut names: Vec<(String, u32)> = Vec::new();
            let mut is_mutable = false;

            for k in 0..spec.named_child_count() {
                let child = match spec.named_child(k) {
                    Some(n) => n,
                    None => continue,
                };
                match child.kind() {
                    "identifier" => {
                        let name = node_text(child, source).to_string();
                        let line = line_of(child);
                        names.push((name, line));
                    }
                    k if is_mutable_type_kind(k) => {
                        is_mutable = true;
                    }
                    "expression_list" => {
                        if contains_mutable_value(child, source) {
                            is_mutable = true;
                        }
                    }
                    "composite_literal" | "call_expression"
                        if contains_mutable_value(child, source) =>
                    {
                        is_mutable = true;
                    }
                    _ => {}
                }
            }

            if is_mutable {
                for (name, line) in names {
                    if name == "_" || name.starts_with("Err") || name.starts_with("err") {
                        continue;
                    }
                    findings.push(DetFinding {
                        detector: "global_mutable_state",
                        file: file.to_string(),
                        line,
                        message: format!(
                            "global mutable variable {:?} — use dependency injection instead",
                            name
                        ),
                    });
                }
            }
        }
    }

    findings
}

/// Detect init() functions with > 5 statements or heavy side effects (I/O, network).
fn detect_init_abuse<'a>(root: Node<'a>, source: &[u8], file: &str) -> Vec<DetFinding> {
    let mut findings = Vec::new();

    for i in 0..root.named_child_count() {
        let node = match root.named_child(i) {
            Some(n) => n,
            None => continue,
        };
        if node.kind() != "function_declaration" {
            continue;
        }
        let name_node = match node.child_by_field_name("name") {
            Some(n) => n,
            None => continue,
        };
        if node_text(name_node, source) != "init" {
            continue;
        }
        let body = match node.child_by_field_name("body") {
            Some(b) => b,
            None => continue,
        };

        let stmt_count = count_all_statements(body);
        if stmt_count > 5 {
            findings.push(DetFinding {
                detector: "init_abuse",
                file: file.to_string(),
                line: line_of(node),
                message: format!(
                    "init() has {} statements — move complex logic to explicit setup functions",
                    stmt_count
                ),
            });
            continue; // don't also emit init_side_effects for the same init
        }

        if has_heavy_side_effects(body, source) {
            findings.push(DetFinding {
                detector: "init_side_effects",
                file: file.to_string(),
                line: line_of(node),
                message:
                    "init() performs I/O or network calls — use explicit initialization for testability"
                        .to_string(),
            });
        }
    }

    findings
}

/// True if the package is part of the domain (inner) layer.
///
/// Package paths are project-relative directories ("internal/domain"), so a
/// leading separator is added before testing: without it a top-level `domain/`
/// package would not match "/domain".
fn is_domain_package(pkg_path: &str) -> bool {
    let normalized = format!("/{pkg_path}");
    normalized.contains("/domain") || normalized.contains("/core") || normalized.contains("/port")
}

/// True if the import refers to an adapter (outer) layer package of *this*
/// project.
///
/// The module root is stripped before the markers are tested. Matching the raw
/// import path meant the module name was searched too, so every first-party
/// import in `github.com/acme/infra-tools` matched "/infra"; and a third-party
/// dependency called `handler-kit` counted as this project's handler layer.
/// Only packages below the module root can violate the project's own dependency
/// direction.
fn is_adapter_package(import_path: &str, module_root: &str) -> bool {
    let relative = match import_path.strip_prefix(module_root) {
        Some(rest) => rest,
        None => return false, // third-party or stdlib: not our adapter layer
    };
    let normalized = format!("/{}", relative.trim_start_matches('/'));
    [
        "/adapter",
        "/infrastructure",
        "/infra",
        "/handler",
        "/repository",
        "/controller",
    ]
    .iter()
    .any(|marker| normalized.contains(marker))
}

/// Domain packages that import adapter packages — the dependency rule inverted.
///
/// Cross-package, so it runs as a post-pass over every file's imports rather than
/// per file. The engine path skipped this check entirely until it was ported
/// here, which is why it is cross-package rather than per-file.
fn detect_domain_imports_adapter(
    pkg_imports: &HashMap<String, Vec<String>>,
    module_root: &str,
) -> Vec<DetFinding> {
    let mut pkgs: Vec<&String> = pkg_imports.keys().collect();
    pkgs.sort();

    let mut findings = Vec::new();
    for pkg in pkgs {
        if !is_domain_package(pkg) {
            continue;
        }
        let mut imports = pkg_imports[pkg].clone();
        imports.sort();
        imports.dedup();
        for import in imports {
            if is_adapter_package(&import, module_root) {
                findings.push(DetFinding {
                    detector: "domain_imports_adapter",
                    file: pkg.clone(),
                    line: 0,
                    message: format!(
                        "domain package imports adapter {import:?} — dependencies must point inward"
                    ),
                });
            }
        }
    }
    findings
}

/// MVC-shaped packages (models/, controllers/, views/) inside a project that also
/// shows hexagonal markers. Mirrors `checker.detectMVCInHexagonal`.
fn detect_mvc_in_hexagonal(packages: &[String]) -> Vec<DetFinding> {
    let has_hexagonal = packages.iter().any(|p| {
        let normalized = format!("/{p}");
        normalized.contains("/domain") || normalized.contains("/port")
    });
    if !has_hexagonal {
        return Vec::new();
    }

    let mut mvc: Vec<&String> = packages
        .iter()
        .filter(|p| {
            let last = p.rsplit('/').next().unwrap_or(p);
            matches!(last, "models" | "controllers" | "views")
        })
        .collect();
    mvc.sort();

    mvc.into_iter()
        .map(|pkg| DetFinding {
            detector: "mvc_in_hexagonal",
            file: pkg.clone(),
            line: 0,
            message: format!(
                "MVC package {pkg:?} in hexagonal project — use domain/port/adapter layers instead"
            ),
        })
        .collect()
}

/// Writes to a map that was declared but never allocated.
///
/// `var m map[K]V` allocates nothing and writing to a nil map panics at runtime,
/// unlike reading one which returns the zero value.
///
/// Analysis is per function, because that is the scope a local declaration lives
/// in. Tracking allocations across the whole file meant one `m := make(map…)`
/// anywhere suppressed every `var m map[K]V` bug elsewhere in it — and `m`,
/// `result` and `cache` are exactly the names that repeat, so the detector was
/// close to inert outside single-function examples.
///
/// Package-scope declarations are the exception: they are visible everywhere, so
/// an allocation anywhere in the file (an `init()`, typically) does make the
/// write safe.
fn detect_nil_map_write<'a>(root: Node<'a>, source: &[u8], file: &str) -> Vec<DetFinding> {
    let package_declared = unallocated_map_declarations(root, source, true);
    let file_assigned = assigned_identifiers(root, source);

    let mut scopes: Vec<Node> = Vec::new();
    collect_nodes(root, "function_declaration", &mut scopes);
    collect_nodes(root, "method_declaration", &mut scopes);

    let mut findings = Vec::new();
    for scope in &scopes {
        let mut declared = unallocated_map_declarations(*scope, source, false);
        let mut assigned = assigned_identifiers(*scope, source);

        // A package-level map is in scope here too, and any allocation of it in
        // the file counts.
        for name in &package_declared {
            declared.push(name.clone());
            if file_assigned.contains(name) {
                assigned.push(name.clone());
            }
        }

        findings.extend(nil_map_writes_in(
            *scope, source, file, &declared, &assigned,
        ));
    }
    findings
}

/// Names of maps declared with `var x map[K]V` and no value.
///
/// `package_scope` selects declarations directly under the file rather than
/// inside a function body. Every name of a grouped declaration is collected:
/// `child_by_field_name` returns only the first, so `var a, b map[string]int`
/// left `b` untracked.
fn unallocated_map_declarations(scope: Node, source: &[u8], package_scope: bool) -> Vec<String> {
    let mut specs: Vec<Node> = Vec::new();
    collect_nodes(scope, "var_spec", &mut specs);

    let mut names = Vec::new();
    for spec in specs {
        if package_scope != is_package_scope(spec) {
            continue;
        }
        let has_value = spec.child_by_field_name("value").is_some();
        let is_map = spec
            .child_by_field_name("type")
            .map(|t| t.kind() == "map_type")
            .unwrap_or(false);
        if !is_map || has_value {
            continue;
        }
        let mut cursor = spec.walk();
        for name in spec.children_by_field_name("name", &mut cursor) {
            names.push(node_text(name, source).to_string());
        }
    }
    names
}

/// True if the declaration sits at file scope rather than inside a function.
fn is_package_scope(spec: Node) -> bool {
    let mut node = spec;
    while let Some(parent) = node.parent() {
        match parent.kind() {
            "function_declaration" | "method_declaration" | "func_literal" => return false,
            _ => node = parent,
        }
    }
    true
}

/// Identifiers that appear on the left of an assignment or short declaration —
/// i.e. names given a value of their own, which for a map means allocated.
fn assigned_identifiers(scope: Node, source: &[u8]) -> Vec<String> {
    let mut assignments: Vec<Node> = Vec::new();
    collect_nodes(scope, "assignment_statement", &mut assignments);
    let mut short_decls: Vec<Node> = Vec::new();
    collect_nodes(scope, "short_var_declaration", &mut short_decls);

    let mut names = Vec::new();
    for node in assignments.iter().chain(short_decls.iter()) {
        if let Some(left) = node.child_by_field_name("left") {
            for i in 0..left.named_child_count() {
                if let Some(target) = left.named_child(i)
                    && target.kind() == "identifier"
                {
                    names.push(node_text(target, source).to_string());
                }
            }
        }
    }
    names
}

fn nil_map_writes_in(
    scope: Node,
    source: &[u8],
    file: &str,
    declared: &[String],
    assigned: &[String],
) -> Vec<DetFinding> {
    let mut assignments: Vec<Node> = Vec::new();
    collect_nodes(scope, "assignment_statement", &mut assignments);

    let mut findings = Vec::new();
    for node in &assignments {
        let left = match node.child_by_field_name("left") {
            Some(l) => l,
            None => continue,
        };
        for i in 0..left.named_child_count() {
            let target = match left.named_child(i) {
                Some(t) if t.kind() == "index_expression" => t,
                _ => continue,
            };
            let operand = match target.child_by_field_name("operand") {
                Some(o) => o,
                None => continue,
            };
            let name = node_text(operand, source).to_string();
            if !declared.contains(&name) || assigned.contains(&name) {
                continue;
            }
            findings.push(DetFinding {
                detector: "nil_map_write",
                file: file.to_string(),
                line: line_of(target),
                message: format!(
                    "write to map {name} which is declared but never allocated — writing to a nil map panics; allocate it with make() first"
                ),
            });
        }
    }
    findings
}

/// Single-value type assertions, which panic when the value holds another type.
///
/// The two-value form and a type switch both make the failure explicit. Warning
/// severity: asserting without the comma-ok form is legitimate when the type is
/// genuinely known.
fn detect_type_assertion_without_ok<'a>(
    root: Node<'a>,
    _source: &[u8],
    file: &str,
) -> Vec<DetFinding> {
    let mut assertions: Vec<Node> = Vec::new();
    collect_nodes(root, "type_assertion_expression", &mut assertions);

    let mut findings = Vec::new();
    for assertion in assertions {
        if assertion_has_ok_binding(assertion) {
            continue;
        }
        findings.push(DetFinding {
            detector: "type_assertion_without_ok",
            file: file.to_string(),
            line: line_of(assertion),
            message:
                "type assertion without the comma-ok form panics if the type differs — use `v, ok := x.(T)` or a type switch"
                    .to_string(),
        });
    }
    findings
}

/// True if the assertion is the whole right-hand side of a two-value assignment,
/// which is the safe `v, ok := x.(T)` form.
///
/// Both sides have to be counted. `s, n = v.(string), g()` also has two targets,
/// but the assertion supplies only one of them and still panics on a type
/// mismatch — so testing the left alone marked every parallel assignment safe.
fn assertion_has_ok_binding(assertion: Node) -> bool {
    let mut node = assertion;
    // The assertion sits inside an expression_list on the right of the assignment.
    while let Some(parent) = node.parent() {
        match parent.kind() {
            "assignment_statement" | "short_var_declaration" => {
                let targets = parent
                    .child_by_field_name("left")
                    .map(|left| left.named_child_count())
                    .unwrap_or(0);
                let values = parent
                    .child_by_field_name("right")
                    .map(|right| right.named_child_count())
                    .unwrap_or(0);
                return targets == 2 && values == 1;
            }
            "expression_list" => node = parent,
            _ => return false,
        }
    }
    false
}

/// Remedy reported for a bare `go` statement.
///
/// Naming the panic consequence is what makes this finding land: the previous
/// wording recommended errgroup, which propagates panics rather than containing
/// them, so following the advice left the bug in place.
const NAKED_GOROUTINE_MESSAGE: &str = "bare 'go' statement — an unrecovered panic in the goroutine body crashes the whole process \
(errgroup propagates panics, it does not contain them): add a recover boundary inside the goroutine \
and tie its lifetime to a context";

/// Detect bare `go` statements outside of server lifecycle methods
/// (Run, Start, ListenAndServe, Serve). Naked goroutines lack error propagation
/// and lifecycle management.
fn detect_naked_goroutines<'a>(root: Node<'a>, source: &[u8], file: &str) -> Vec<DetFinding> {
    let server_methods = ["Run", "Start", "ListenAndServe", "Serve"];

    // Collect line ranges of server lifecycle function bodies to exclude.
    let mut excluded_ranges: Vec<(usize, usize)> = Vec::new();
    for i in 0..root.named_child_count() {
        let node = match root.named_child(i) {
            Some(n) => n,
            None => continue,
        };
        let kind = node.kind();
        if kind != "function_declaration" && kind != "method_declaration" {
            continue;
        }
        let name_node = match node.child_by_field_name("name") {
            Some(n) => n,
            None => continue,
        };
        let name = node_text(name_node, source);
        if server_methods.contains(&name)
            && let Some(body) = node.child_by_field_name("body")
        {
            excluded_ranges.push((body.start_position().row, body.end_position().row));
        }
    }

    let mut findings = Vec::new();
    let mut go_nodes: Vec<Node> = Vec::new();
    collect_nodes(root, "go_statement", &mut go_nodes);

    for go_node in go_nodes {
        let row = go_node.start_position().row;
        let inside_server = excluded_ranges
            .iter()
            .any(|(start, end)| row >= *start && row <= *end);
        if !inside_server {
            findings.push(DetFinding {
                detector: "naked_goroutine",
                file: file.to_string(),
                line: line_of(go_node),
                // Must name the panic consequence: recommending errgroup alone
                // pointed at no working fix, because errgroup propagates panics
                // rather than recovering them. Kept in sync with
                // checker.nakedGoroutineMessage on the Go side.
                message: NAKED_GOROUTINE_MESSAGE.to_string(),
            });
        }
    }

    findings
}

/// Detect `if err != nil` blocks with an empty body or `return nil` (swallowed errors).
fn detect_swallowed_errors<'a>(root: Node<'a>, source: &[u8], file: &str) -> Vec<DetFinding> {
    let mut if_nodes: Vec<Node> = Vec::new();
    collect_nodes(root, "if_statement", &mut if_nodes);

    let mut findings = Vec::new();

    for if_node in if_nodes {
        let cond = match if_node.child_by_field_name("condition") {
            Some(c) => c,
            None => continue,
        };
        if !is_err_neq_nil(cond, source) {
            continue;
        }

        let body = match if_node.child_by_field_name("consequence") {
            Some(b) => b,
            None => continue,
        };

        let stmt_count = body
            .named_children(&mut body.walk())
            .filter(|n| n.is_named())
            .count();

        if stmt_count == 0 {
            findings.push(DetFinding {
                detector: "swallowed_error",
                file: file.to_string(),
                line: line_of(if_node),
                message: "error checked but silently discarded — handle, wrap, or log it"
                    .to_string(),
            });
        } else if stmt_count == 1 {
            // Check for `return nil`
            if let Some(stmt) = body.named_child(0)
                && stmt.kind() == "return_statement"
            {
                let mut exprs: Vec<Node> = Vec::new();
                collect_nodes(stmt, "nil", &mut exprs);
                // count named children of return_statement (the expressions)
                let ret_exprs: Vec<_> = stmt.named_children(&mut stmt.walk()).collect();
                if ret_exprs.len() == 1 && ret_exprs[0].kind() == "nil" {
                    findings.push(DetFinding {
                        detector: "swallowed_error",
                        file: file.to_string(),
                        line: line_of(if_node),
                        message:
                            "error checked but return nil discards it — propagate or wrap the error"
                                .to_string(),
                    });
                }
            }
        }
    }

    findings
}

/// Detect `context.Background()` in handler/adapter packages where the request
/// context should be used instead.
fn detect_context_background<'a>(
    root: Node<'a>,
    source: &[u8],
    file: &str,
    pkg_path: &str,
) -> Vec<DetFinding> {
    if !is_handler_package(pkg_path) {
        return vec![];
    }

    // Collect lines where context.Background() is legitimately used:
    // inside context.WithTimeout/WithDeadline (shutdown pattern) or init calls.
    let mut skip_lines: std::collections::HashSet<usize> = std::collections::HashSet::new();
    let mut call_nodes: Vec<Node> = Vec::new();
    collect_nodes(root, "call_expression", &mut call_nodes);

    for call in &call_nodes {
        let fn_text = call
            .child_by_field_name("function")
            .map(|f| node_text(f, source))
            .unwrap_or("");
        if fn_text == "context.WithTimeout" || fn_text == "context.WithDeadline" {
            // Mark any context.Background() inside this call's args as skip.
            if let Some(args) = call.child_by_field_name("arguments") {
                let mut inner_calls: Vec<Node> = Vec::new();
                collect_nodes(args, "call_expression", &mut inner_calls);
                for inner in inner_calls {
                    if is_context_background_call(inner, source) {
                        skip_lines.insert(inner.start_position().row);
                    }
                }
            }
        }
        if is_init_call(fn_text)
            && let Some(args) = call.child_by_field_name("arguments")
        {
            let mut inner_calls: Vec<Node> = Vec::new();
            collect_nodes(args, "call_expression", &mut inner_calls);
            for inner in inner_calls {
                if is_context_background_call(inner, source) {
                    skip_lines.insert(inner.start_position().row);
                }
            }
        }
    }

    let mut findings = Vec::new();
    for call in &call_nodes {
        if is_context_background_call(*call, source) {
            let row = call.start_position().row;
            if !skip_lines.contains(&row) {
                findings.push(DetFinding {
                    detector: "context_background_in_handler",
                    file: file.to_string(),
                    line: line_of(*call),
                    message:
                        "context.Background() in handler — use request context (r.Context()) for proper cancellation"
                            .to_string(),
                });
            }
        }
    }

    findings
}

/// Detect SQL string concatenation (injection risk).
fn detect_sql_concatenation<'a>(root: Node<'a>, source: &[u8], file: &str) -> Vec<DetFinding> {
    let mut bin_nodes: Vec<Node> = Vec::new();
    collect_nodes(root, "binary_expression", &mut bin_nodes);

    let sql_keywords = [
        "SELECT ", "INSERT ", "UPDATE ", "DELETE ", "FROM ", "WHERE ", "JOIN ",
    ];
    let mut findings = Vec::new();

    for bin_node in bin_nodes {
        // Only top-level + expressions (avoid duplicates from nested binary exprs).
        let op = bin_node
            .child_by_field_name("operator")
            .map(|n| node_text(n, source))
            .unwrap_or("");
        if op != "+" {
            continue;
        }

        // Skip if parent is also a binary + expression (we'll catch the root).
        if let Some(parent) = bin_node.parent()
            && parent.kind() == "binary_expression"
            && let Some(pop) = parent.child_by_field_name("operator")
            && node_text(pop, source) == "+"
        {
            continue;
        }

        if binary_contains_sql_keyword(bin_node, source, &sql_keywords) {
            findings.push(DetFinding {
                detector: "sql_concatenation",
                file: file.to_string(),
                line: line_of(bin_node),
                message:
                    "SQL string concatenation detected — use parameterized queries to prevent injection"
                        .to_string(),
            });
        }
    }

    findings
}

/// Detect `uuid.New()` / `uuid.NewString()` — suggests UUIDv7 for DB primary keys.
fn detect_uuid_v4_as_key<'a>(root: Node<'a>, source: &[u8], file: &str) -> Vec<DetFinding> {
    // UUIDv4 is fine for request IDs.
    let base = file.split('/').next_back().unwrap_or(file).to_lowercase();
    if base.contains("requestid") || base.contains("request_id") {
        return vec![];
    }

    let mut call_nodes: Vec<Node> = Vec::new();
    collect_nodes(root, "call_expression", &mut call_nodes);

    let mut findings = Vec::new();
    for call in call_nodes {
        let fn_text = call
            .child_by_field_name("function")
            .map(|f| node_text(f, source))
            .unwrap_or("");
        if fn_text == "uuid.New" || fn_text == "uuid.NewString" {
            findings.push(DetFinding {
                detector: "uuid_v4_as_key",
                file: file.to_string(),
                line: line_of(call),
                message: format!(
                    "{}() generates UUIDv4 (random) — use UUIDv7 for database primary keys to avoid index fragmentation",
                    fn_text
                ),
            });
        }
    }

    findings
}

/// Detect HTTP handlers with > 40 statements (fat handlers should delegate to services).
fn detect_fat_handlers<'a>(
    root: Node<'a>,
    source: &[u8],
    file: &str,
    pkg_path: &str,
) -> Vec<DetFinding> {
    if !is_handler_package(pkg_path) {
        return vec![];
    }

    let mut findings = Vec::new();

    for i in 0..root.named_child_count() {
        let node = match root.named_child(i) {
            Some(n) => n,
            None => continue,
        };
        let kind = node.kind();
        if kind != "function_declaration" && kind != "method_declaration" {
            continue;
        }
        if !is_http_handler_func(node, source) {
            continue;
        }
        let body = match node.child_by_field_name("body") {
            Some(b) => b,
            None => continue,
        };
        let stmts = count_all_statements(body);
        if stmts > 40 {
            let name_node = node.child_by_field_name("name");
            let name = name_node
                .map(|n| node_text(n, source))
                .unwrap_or("<anonymous>");
            findings.push(DetFinding {
                detector: "fat_handler",
                file: file.to_string(),
                line: line_of(node),
                message: format!(
                    "handler {} has {} statements — extract business logic to a service layer",
                    name, stmts
                ),
            });
        }
    }

    findings
}

/// Count exported top-level symbols (functions, types, vars, consts).
fn count_exported_symbols(root: Node, source: &[u8]) -> u32 {
    let mut count = 0u32;

    for i in 0..root.named_child_count() {
        let node = match root.named_child(i) {
            Some(n) => n,
            None => continue,
        };
        match node.kind() {
            "function_declaration" | "method_declaration" => {
                if let Some(name) = node.child_by_field_name("name") {
                    let text = node_text(name, source);
                    if is_exported(text) {
                        count += 1;
                    }
                }
            }
            "type_declaration" => {
                for j in 0..node.named_child_count() {
                    if let Some(spec) = node.named_child(j)
                        && spec.kind() == "type_spec"
                        && let Some(name) = spec.child_by_field_name("name")
                        && is_exported(node_text(name, source))
                    {
                        count += 1;
                    }
                }
            }
            "var_declaration" | "const_declaration" => {
                count_exported_in_decl(node, source, &mut count);
            }
            _ => {}
        }
    }

    count
}

fn count_exported_in_decl(decl: Node, source: &[u8], count: &mut u32) {
    let spec_kind = if decl.kind() == "var_declaration" {
        "var_spec"
    } else {
        "const_spec"
    };
    for i in 0..decl.named_child_count() {
        let spec = match decl.named_child(i) {
            Some(n) => n,
            None => continue,
        };
        if spec.kind() != spec_kind {
            continue;
        }
        for j in 0..spec.named_child_count() {
            if let Some(child) = spec.named_child(j)
                && child.kind() == "identifier"
                && is_exported(node_text(child, source))
            {
                *count += 1;
            }
        }
    }
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

fn node_text<'a>(node: Node, source: &'a [u8]) -> &'a str {
    node.utf8_text(source).unwrap_or("")
}

fn line_of(node: Node) -> u32 {
    (node.start_position().row + 1) as u32
}

/// Collect all descendant nodes with the given kind.
fn collect_nodes<'a>(root: Node<'a>, kind: &str, out: &mut Vec<Node<'a>>) {
    if root.kind() == kind {
        out.push(root);
    }
    for i in 0..root.child_count() {
        if let Some(child) = root.child(i) {
            collect_nodes(child, kind, out);
        }
    }
}

fn is_mutable_type_kind(kind: &str) -> bool {
    matches!(
        kind,
        "map_type" | "slice_type" | "array_type" | "channel_type" | "pointer_type_expression"
    )
}

fn contains_mutable_value(node: Node, source: &[u8]) -> bool {
    match node.kind() {
        "composite_literal" => true,
        "call_expression" => {
            let fn_text = node
                .child_by_field_name("function")
                .map(|f| node_text(f, source))
                .unwrap_or("");
            fn_text == "make"
        }
        "expression_list" => {
            for i in 0..node.named_child_count() {
                if let Some(child) = node.named_child(i)
                    && contains_mutable_value(child, source)
                {
                    return true;
                }
            }
            false
        }
        _ => false,
    }
}

/// Count all statement nodes recursively within a block (mirrors Go's ast.Inspect counter).
fn count_all_statements(node: Node) -> usize {
    const STMT_KINDS: &[&str] = &[
        "expression_statement",
        "return_statement",
        "if_statement",
        "for_statement",
        "range_statement",
        "switch_statement",
        "type_switch_statement",
        "select_statement",
        "go_statement",
        "defer_statement",
        "var_declaration",
        "short_var_declaration",
        "assignment_statement",
        "inc_statement",
        "dec_statement",
        "send_statement",
        "labeled_statement",
        "break_statement",
        "continue_statement",
        "goto_statement",
        "fallthrough_statement",
        "const_declaration",
        "type_declaration",
    ];

    let mut count = 0usize;
    if STMT_KINDS.contains(&node.kind()) {
        count += 1;
    }
    for i in 0..node.child_count() {
        if let Some(child) = node.child(i) {
            count += count_all_statements(child);
        }
    }
    count
}

fn has_heavy_side_effects(body: Node, source: &[u8]) -> bool {
    let heavy = [
        "http.Get",
        "http.Post",
        "http.Do",
        "sql.Open",
        "pgx.Connect",
        "mongo.Connect",
        "os.Open",
        "os.Create",
        "os.ReadFile",
        "net.Dial",
        "net.Listen",
    ];

    let mut call_nodes: Vec<Node> = Vec::new();
    collect_nodes(body, "call_expression", &mut call_nodes);

    for call in call_nodes {
        let fn_text = call
            .child_by_field_name("function")
            .map(|f| node_text(f, source))
            .unwrap_or("");
        if heavy.iter().any(|h| fn_text.contains(h)) {
            return true;
        }
    }
    false
}

/// Returns true if the condition is `err != nil` or `nil != err`.
fn is_err_neq_nil(cond: Node, source: &[u8]) -> bool {
    if cond.kind() != "binary_expression" {
        return false;
    }
    let op = cond
        .child_by_field_name("operator")
        .map(|n| node_text(n, source))
        .unwrap_or("");
    if op != "!=" {
        return false;
    }
    let left = cond
        .child_by_field_name("left")
        .map(|n| node_text(n, source))
        .unwrap_or("");
    let right = cond
        .child_by_field_name("right")
        .map(|n| node_text(n, source))
        .unwrap_or("");

    (left == "err" && right == "nil") || (left == "nil" && right == "err")
}

fn is_context_background_call(call: Node, source: &[u8]) -> bool {
    let fn_text = call
        .child_by_field_name("function")
        .map(|f| node_text(f, source))
        .unwrap_or("");
    fn_text == "context.Background"
}

fn is_handler_package(pkg_path: &str) -> bool {
    pkg_path.contains("handler")
        || pkg_path.contains("controller")
        || pkg_path.contains("adapter")
        || pkg_path.contains("transport")
        || pkg_path.contains("api")
}

fn is_init_call(fn_name: &str) -> bool {
    const INIT_SUFFIXES: &[&str] = &[
        "Fetch",
        "Connect",
        "Open",
        "Dial",
        "Init",
        "Listen",
        "Setup",
        "Configure",
    ];
    INIT_SUFFIXES
        .iter()
        .any(|s| fn_name.ends_with(s) || fn_name.ends_with(&format!(".{}", s)))
}

fn binary_contains_sql_keyword(node: Node, source: &[u8], keywords: &[&str]) -> bool {
    match node.kind() {
        "interpreted_string_literal" | "raw_string_literal" => {
            let text = node_text(node, source).to_uppercase();
            keywords.iter().any(|kw| text.contains(kw))
        }
        "binary_expression" => {
            let left = node.child_by_field_name("left");
            let right = node.child_by_field_name("right");
            left.is_some_and(|n| binary_contains_sql_keyword(n, source, keywords))
                || right.is_some_and(|n| binary_contains_sql_keyword(n, source, keywords))
        }
        _ => false,
    }
}

fn is_exported(name: &str) -> bool {
    name.chars()
        .next()
        .map(|c| c.is_uppercase())
        .unwrap_or(false)
}

/// Returns true if the function/method has (http.ResponseWriter, *http.Request) params.
fn is_http_handler_func(node: Node, source: &[u8]) -> bool {
    let params = match node.child_by_field_name("parameters") {
        Some(p) => p,
        None => return false,
    };

    let params_text = node_text(params, source);
    params_text.contains("ResponseWriter") && params_text.contains("Request")
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parse(src: &str) -> tree_sitter::Tree {
        let mut parser = Parser::new();
        parser
            .set_language(&tree_sitter_go::LANGUAGE.into())
            .expect("tree-sitter-go language load failed");
        parser.parse(src, None).expect("parse failed")
    }

    /// Mirrors checker.TestDetectNilMapWrite: writing to a map that was declared
    /// but never allocated panics at runtime.
    #[test]
    fn nil_map_write_cases() {
        let cases: Vec<(&str, &str, usize)> = vec![
            (
                "declared without make, then written",
                "package foo\nfunc f() {\n\tvar m map[string]int\n\tm[\"k\"] = 1\n}\n",
                1,
            ),
            (
                "package-level map written from a function",
                "package foo\nvar registry map[string]int\nfunc r() { registry[\"a\"] = 1 }\n",
                1,
            ),
            (
                "make before the write is safe",
                "package foo\nfunc f() {\n\tvar m map[string]int\n\tm = make(map[string]int)\n\tm[\"k\"] = 1\n}\n",
                0,
            ),
            (
                "composite literal is safe",
                "package foo\nfunc f() {\n\tm := map[string]int{}\n\tm[\"k\"] = 1\n}\n",
                0,
            ),
            (
                "reading a nil map is legal",
                "package foo\nfunc f() int {\n\tvar m map[string]int\n\treturn m[\"k\"]\n}\n",
                0,
            ),
            (
                "slice index write is not a map",
                "package foo\nfunc f() {\n\tvar s []int\n\ts[0] = 1\n}\n",
                0,
            ),
            // Allocation tracking was file-scoped, so an allocation in any
            // function suppressed the bug in every other function. Since m,
            // result and cache are the usual names, the detector was inert in
            // realistic files — it only fired in single-function examples.
            (
                "allocation in another function does not make this write safe",
                "package foo\n\
                 func Safe() map[string]int {\n\tm := make(map[string]int)\n\tm[\"ok\"] = 1\n\treturn m\n}\n\
                 func Bug() map[string]int {\n\tvar m map[string]int\n\tm[\"boom\"] = 1\n\treturn m\n}\n",
                1,
            ),
            (
                "each function keeps its own allocation",
                "package foo\n\
                 func A() {\n\tvar m map[string]int\n\tm = make(map[string]int)\n\tm[\"a\"] = 1\n}\n\
                 func B() {\n\tvar m map[string]int\n\tm = make(map[string]int)\n\tm[\"b\"] = 1\n}\n",
                0,
            ),
            // child_by_field_name returns the first name only, so the second and
            // subsequent variables of a grouped declaration were never tracked.
            (
                "every name in a grouped declaration is tracked",
                "package foo\nfunc f() {\n\tvar a, b map[string]int\n\ta[\"x\"] = 1\n\tb[\"y\"] = 2\n}\n",
                2,
            ),
            (
                "a package-level map allocated in init is safe",
                "package foo\nvar registry map[string]int\nfunc init() { registry = make(map[string]int) }\nfunc r() { registry[\"a\"] = 1 }\n",
                0,
            ),
        ];

        for (name, src, want) in cases {
            let tree = parse(src);
            let findings = detect_nil_map_write(tree.root_node(), src.as_bytes(), "test.go");
            assert_eq!(findings.len(), want, "{name}: got {findings:?}");
        }
    }

    /// Mirrors checker.TestDetectTypeAssertionWithoutOK.
    #[test]
    fn type_assertion_without_ok_cases() {
        let cases: Vec<(&str, &str, usize)> = vec![
            (
                "single-value assertion",
                "package foo\nfunc f(v any) string { return v.(string) }\n",
                1,
            ),
            (
                "two-value form is safe",
                "package foo\nfunc f(v any) string {\n\ts, ok := v.(string)\n\tif !ok { return \"\" }\n\treturn s\n}\n",
                0,
            ),
            (
                "type switch is safe",
                "package foo\nfunc f(v any) string {\n\tswitch t := v.(type) {\n\tcase string:\n\t\treturn t\n\t}\n\treturn \"\"\n}\n",
                0,
            ),
            (
                "assertion as a call argument",
                "package foo\nfunc g(s string) {}\nfunc f(v any) { g(v.(string)) }\n",
                1,
            ),
            (
                "two-value assignment to existing variables is safe",
                "package foo\nfunc f(v any) {\n\tvar s string\n\tvar ok bool\n\ts, ok = v.(string)\n\t_, _ = s, ok\n}\n",
                0,
            ),
            // Two targets alone do not make the comma-ok form: here the second
            // value comes from g(), so the assertion is still single-valued and
            // still panics. Counting only the left-hand side treated every
            // multi-assignment as safe.
            (
                "parallel assignment is not the comma-ok form",
                "package foo\nfunc g() int { return 0 }\nfunc f(v any) {\n\tvar s string\n\tvar n int\n\ts, n = v.(string), g()\n\t_, _ = s, n\n}\n",
                1,
            ),
        ];

        for (name, src, want) in cases {
            let tree = parse(src);
            let findings =
                detect_type_assertion_without_ok(tree.root_node(), src.as_bytes(), "test.go");
            assert_eq!(findings.len(), want, "{name}: got {findings:?}");
        }
    }

    /// These two detectors existed only in the Go implementation, so the engine
    /// path — the default — silently skipped both. Package paths are
    /// project-relative here, unlike the Go side's full import paths.
    #[test]
    fn domain_importing_adapter_is_reported() {
        let mut pkg_imports = HashMap::new();
        pkg_imports.insert(
            "internal/domain".to_string(),
            vec![
                "fmt".to_string(),
                "example.com/app/adapter/postgres".to_string(),
            ],
        );
        pkg_imports.insert(
            "internal/adapter/postgres".to_string(),
            vec!["example.com/app/internal/domain".to_string()],
        );

        let findings = detect_domain_imports_adapter(&pkg_imports, "example.com/app");

        assert_eq!(findings.len(), 1, "only the inward violation is a finding");
        assert_eq!(findings[0].detector, "domain_imports_adapter");
        assert_eq!(findings[0].file, "internal/domain");
        assert!(findings[0].message.contains("adapter/postgres"));
    }

    /// The module path is a prefix of every first-party import, so testing the
    /// raw import string for markers like "/infra" made the detector fire on
    /// every import in any project whose module name happens to contain one.
    /// `github.com/acme/infra-tools` flagged its own value objects, at error
    /// severity, and this detector had just started running for everyone.
    #[test]
    fn adapter_markers_are_matched_below_the_module_root() {
        let mut pkg_imports = HashMap::new();
        pkg_imports.insert(
            "internal/domain".to_string(),
            vec![
                "github.com/acme/infra-tools/internal/valueobject".to_string(),
                "github.com/acme/infra-tools/internal/adapter/postgres".to_string(),
            ],
        );

        let findings = detect_domain_imports_adapter(&pkg_imports, "github.com/acme/infra-tools");

        let messages: Vec<&str> = findings.iter().map(|f| f.message.as_str()).collect();
        assert_eq!(
            findings.len(),
            1,
            "only the adapter import crosses the layer boundary, got {messages:?}"
        );
        assert!(findings[0].message.contains("adapter/postgres"));
    }

    /// A third-party dependency is not this project's adapter layer, whatever it
    /// is called. Only first-party packages can violate the project's own
    /// dependency direction.
    #[test]
    fn third_party_imports_are_not_adapter_packages() {
        let mut pkg_imports = HashMap::new();
        pkg_imports.insert(
            "internal/domain".to_string(),
            vec![
                "github.com/vendor/handler-kit".to_string(),
                "github.com/vendor/go-repository".to_string(),
                "net/http".to_string(),
            ],
        );

        let findings = detect_domain_imports_adapter(&pkg_imports, "example.com/app");

        assert!(
            findings.is_empty(),
            "third-party imports are not the project's adapters"
        );
    }

    /// A top-level `domain/` package has no leading slash in its relative path;
    /// testing it without normalising would miss the violation entirely.
    #[test]
    fn top_level_domain_package_is_recognised() {
        assert!(is_domain_package("domain"));
        assert!(is_domain_package("internal/core"));
        assert!(is_domain_package("internal/port/http"));
        assert!(!is_domain_package("internal/adapter"));
    }

    #[test]
    fn adapter_imports_are_recognised() {
        let module = "example.com/app";
        assert!(is_adapter_package("example.com/app/adapter/db", module));
        assert!(is_adapter_package(
            "example.com/app/internal/repository",
            module
        ));
        assert!(is_adapter_package("example.com/app/infra/queue", module));
        assert!(!is_adapter_package("fmt", module));
        assert!(!is_adapter_package(
            "example.com/app/internal/domain",
            module
        ));
    }

    /// The marker must be found in the path below the module root, not in the
    /// module name itself.
    #[test]
    fn module_name_is_not_searched_for_adapter_markers() {
        let module = "github.com/acme/infra-tools";
        assert!(!is_adapter_package(
            "github.com/acme/infra-tools/internal/valueobject",
            module
        ));
        assert!(is_adapter_package(
            "github.com/acme/infra-tools/internal/adapter/postgres",
            module
        ));
    }

    #[test]
    fn mvc_packages_flagged_only_in_hexagonal_projects() {
        let hexagonal = vec![
            "internal/domain".to_string(),
            "internal/controllers".to_string(),
            "internal/models".to_string(),
        ];
        let findings = detect_mvc_in_hexagonal(&hexagonal);
        assert_eq!(findings.len(), 2);
        assert_eq!(findings[0].detector, "mvc_in_hexagonal");

        // No hexagonal markers: an MVC project is not a violation of itself.
        let mvc_only = vec![
            "internal/controllers".to_string(),
            "internal/models".to_string(),
        ];
        assert!(detect_mvc_in_hexagonal(&mvc_only).is_empty());
    }
}
