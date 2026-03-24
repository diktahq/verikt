use crate::import_graph::collect_go_files;
use crate::pb::{
    self, CheckComplete, CheckRequest, EngineResponse, Finding, RuleStatus,
    engine_response::Payload,
    rule::Spec,
    rule_status::Status,
};
use std::fs;
use std::path::PathBuf;
use std::time::Instant;
use tree_sitter::{Node, Parser};

pub fn handle_metric_check(req: &CheckRequest) -> Vec<EngineResponse> {
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

    let metric_rules: Vec<_> = req
        .rules
        .iter()
        .filter(|r| matches!(&r.spec, Some(Spec::FunctionMetric(_))))
        .collect();

    if metric_rules.is_empty() {
        return vec![];
    }

    let go_files = collect_go_files(&project, &req.target_files);

    let mut responses: Vec<EngineResponse> = Vec::new();
    let mut findings_total = 0u32;
    let mut findings_error = 0u32;
    let mut findings_warning = 0u32;
    let mut findings_info = 0u32;
    let mut rule_statuses: Vec<RuleStatus> = Vec::new();

    for rule in &metric_rules {
        let spec = match &rule.spec {
            Some(Spec::FunctionMetric(s)) => s,
            _ => continue,
        };

        let mut rule_matched = false;

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

            // Skip test files.
            if rel_file.ends_with("_test.go") {
                continue;
            }

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

            let mut fn_nodes: Vec<Node> = Vec::new();
            collect_function_nodes(root, &mut fn_nodes);

            for fn_node in fn_nodes {
                let name = fn_node
                    .child_by_field_name("name")
                    .map(|n| n.utf8_text(source).unwrap_or("<anonymous>"))
                    .unwrap_or("<anonymous>");

                let line = (fn_node.start_position().row + 1) as u32;

                // Check max_lines.
                if spec.max_lines > 0 {
                    if let Some(body) = fn_node.child_by_field_name("body") {
                        let start_row = body.start_position().row;
                        let end_row = body.end_position().row;
                        let lines = (end_row - start_row) as i32;
                        if lines > spec.max_lines {
                            rule_matched = true;
                            findings_total += 1;
                            tally_severity(rule.severity, &mut findings_error, &mut findings_warning, &mut findings_info);
                            responses.push(finding_response(
                                rule,
                                &rel_file,
                                line,
                                &format!(
                                    "{} — {} lines (max: {})",
                                    name, lines, spec.max_lines
                                ),
                                "function_lines",
                            ));
                        }
                    }
                }

                // Check max_params.
                if spec.max_params > 0 {
                    if let Some(params) = fn_node.child_by_field_name("parameters") {
                        let param_count = count_parameter_decls(params) as i32;
                        if param_count > spec.max_params {
                            rule_matched = true;
                            findings_total += 1;
                            tally_severity(rule.severity, &mut findings_error, &mut findings_warning, &mut findings_info);
                            responses.push(finding_response(
                                rule,
                                &rel_file,
                                line,
                                &format!(
                                    "{} — {} params (max: {})",
                                    name, param_count, spec.max_params
                                ),
                                "function_params",
                            ));
                        }
                    }
                }

                // Check max_returns.
                if spec.max_returns > 0 {
                    let return_count = count_return_values(fn_node, source) as i32;
                    if return_count > spec.max_returns {
                        rule_matched = true;
                        findings_total += 1;
                        tally_severity(rule.severity, &mut findings_error, &mut findings_warning, &mut findings_info);
                        responses.push(finding_response(
                            rule,
                            &rel_file,
                            line,
                            &format!(
                                "{} — {} return values (max: {})",
                                name, return_count, spec.max_returns
                            ),
                            "function_returns",
                        ));
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
            rules_evaluated: metric_rules.len() as u32,
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

// ─── Helpers ─────────────────────────────────────────────────────────────────

/// Collect all function_declaration and method_declaration nodes recursively.
fn collect_function_nodes<'a>(root: Node<'a>, out: &mut Vec<Node<'a>>) {
    if root.kind() == "function_declaration" || root.kind() == "method_declaration" {
        out.push(root);
        return; // don't recurse into nested function literals (closures)
    }
    for i in 0..root.child_count() {
        if let Some(child) = root.child(i) {
            collect_function_nodes(child, out);
        }
    }
}

/// Count parameter declarations in a parameter_list node.
/// Each parameter_declaration may declare multiple names: func(a, b int) = 2 params.
fn count_parameter_decls(params: Node) -> usize {
    let mut count = 0usize;
    for i in 0..params.named_child_count() {
        let child = match params.named_child(i) {
            Some(n) => n,
            None => continue,
        };
        if child.kind() == "parameter_declaration" || child.kind() == "variadic_parameter_declaration" {
            // Count identifiers in this parameter_declaration.
            let names = child
                .named_children(&mut child.walk())
                .filter(|n| n.kind() == "identifier")
                .count();
            count += names.max(1); // at least 1 even if unnamed (e.g., func(int))
        }
    }
    count
}

/// Count return values from a function's result node.
fn count_return_values(fn_node: Node, source: &[u8]) -> usize {
    let result = match fn_node.child_by_field_name("result") {
        Some(r) => r,
        None => return 0,
    };

    match result.kind() {
        // Single unnamed return type: func() int
        "type_identifier" | "pointer_type_expression" | "qualified_type"
        | "array_type" | "map_type" | "slice_type" | "channel_type"
        | "interface_type" | "struct_type" => 1,
        // Multiple return values wrapped in parameter_list: func() (int, error)
        "parameter_list" => {
            let mut count = 0usize;
            for i in 0..result.named_child_count() {
                if let Some(child) = result.named_child(i) {
                    if child.kind() == "parameter_declaration" {
                        let names = child
                            .named_children(&mut child.walk())
                            .filter(|n| n.kind() == "identifier")
                            .count();
                        count += names.max(1);
                    }
                }
            }
            count
        }
        _ => {
            // Fallback: count comma-separated type tokens in the text.
            let text = result.utf8_text(source).unwrap_or("");
            text.chars().filter(|&c| c == ',').count() + 1
        }
    }
}

fn tally_severity(severity: i32, errors: &mut u32, warnings: &mut u32, infos: &mut u32) {
    match severity {
        s if s == pb::Severity::Error as i32 => *errors += 1,
        s if s == pb::Severity::Warning as i32 => *warnings += 1,
        s if s == pb::Severity::Info as i32 => *infos += 1,
        _ => {}
    }
}

fn finding_response(rule: &pb::Rule, file: &str, line: u32, message: &str, match_str: &str) -> EngineResponse {
    EngineResponse {
        payload: Some(Payload::Finding(Finding {
            rule_id: rule.id.clone(),
            severity: rule.severity,
            file: file.to_string(),
            line,
            column: 0,
            message: message.to_string(),
            r#match: match_str.to_string(),
            engine: "metric".to_string(),
        })),
    }
}
