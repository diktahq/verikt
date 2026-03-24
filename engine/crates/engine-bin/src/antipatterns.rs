use crate::import_graph::{collect_go_files, file_to_package};
use crate::pb::{
    self, CheckComplete, CheckRequest, EngineResponse, Finding, RuleStatus,
    engine_response::Payload,
    rule::Spec,
    rule_status::Status,
};
use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;
use std::time::Instant;
use tree_sitter::{Node, Parser};

/// A pre-rule-assignment finding from a specific detector.
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
        all_findings.extend(detect_context_background(root, source, &rel_file, &pkg_path));
        all_findings.extend(detect_sql_concatenation(root, source, &rel_file));
        all_findings.extend(detect_uuid_v4_as_key(root, source, &rel_file));
        all_findings.extend(detect_fat_handlers(root, source, &rel_file, &pkg_path));

        // Exported symbol count for god_package detection.
        let dir_rel = file_path
            .parent()
            .and_then(|d| d.strip_prefix(&project).ok())
            .map(|d| d.to_string_lossy().replace('\\', "/"))
            .unwrap_or_default();
        let count = count_exported_symbols(root, source);
        *pkg_exports.entry(dir_rel).or_default() += count;
    }

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

        let mut rule_matched = false;

        for f in &all_findings {
            // Empty detectors list = all detectors enabled.
            if !spec.detectors.is_empty() && !spec.detectors.iter().any(|d| d == f.detector) {
                continue;
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
                    file: f.file.clone(),
                    line: f.line,
                    column: 0,
                    message: f.message.clone(),
                    r#match: f.detector.to_string(),
                    engine: "anti_pattern".to_string(),
                })),
            });
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
fn detect_global_mutable_state<'a>(
    root: Node<'a>,
    source: &[u8],
    file: &str,
) -> Vec<DetFinding> {
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
                    "composite_literal" | "call_expression" => {
                        if contains_mutable_value(child, source) {
                            is_mutable = true;
                        }
                    }
                    _ => {}
                }
            }

            if is_mutable {
                for (name, line) in names {
                    if name == "_"
                        || name.starts_with("Err")
                        || name.starts_with("err")
                    {
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
        if server_methods.contains(&name) {
            if let Some(body) = node.child_by_field_name("body") {
                excluded_ranges.push((
                    body.start_position().row,
                    body.end_position().row,
                ));
            }
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
                message:
                    "bare 'go' statement — use errgroup.Go() or structured concurrency for error propagation and lifecycle"
                        .to_string(),
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
            if let Some(stmt) = body.named_child(0) {
                if stmt.kind() == "return_statement" {
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
        if is_init_call(fn_text) {
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

    let sql_keywords = ["SELECT ", "INSERT ", "UPDATE ", "DELETE ", "FROM ", "WHERE ", "JOIN "];
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
        if let Some(parent) = bin_node.parent() {
            if parent.kind() == "binary_expression" {
                if let Some(pop) = parent.child_by_field_name("operator") {
                    if node_text(pop, source) == "+" {
                        continue;
                    }
                }
            }
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
                    if let Some(spec) = node.named_child(j) {
                        if spec.kind() == "type_spec" {
                            if let Some(name) = spec.child_by_field_name("name") {
                                if is_exported(node_text(name, source)) {
                                    count += 1;
                                }
                            }
                        }
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
            if let Some(child) = spec.named_child(j) {
                if child.kind() == "identifier" && is_exported(node_text(child, source)) {
                    *count += 1;
                }
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
                if let Some(child) = node.named_child(i) {
                    if contains_mutable_value(child, source) {
                        return true;
                    }
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
        "Fetch", "Connect", "Open", "Dial", "Init", "Listen", "Setup", "Configure",
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
            left.map_or(false, |n| binary_contains_sql_keyword(n, source, keywords))
                || right.map_or(false, |n| binary_contains_sql_keyword(n, source, keywords))
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
