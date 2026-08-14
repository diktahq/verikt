use crate::import_graph::collect_go_files;
use crate::pb::{
    self, CheckComplete, CheckRequest, EngineResponse, Finding, RuleStatus,
    engine_response::Payload, rule::Spec, rule_status::Status,
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
                if spec.max_lines > 0
                    && let Some(body) = fn_node.child_by_field_name("body")
                {
                    // Body lines, counted between the braces.
                    //
                    // The body node spans the opening brace row to the closing
                    // brace row, so end - start counted one of the two brace rows
                    // as a line of code: a function with exactly max_lines lines
                    // in it was reported as max_lines + 1 and failed. Subtracting
                    // both brace rows makes `max_lines: 50` mean what it reads
                    // like — a body may be up to 50 lines — and makes the number
                    // in the message the number a reader counts.
                    let start_row = body.start_position().row;
                    let end_row = body.end_position().row;
                    let lines = (end_row.saturating_sub(start_row).saturating_sub(1)) as i32;
                    if lines > spec.max_lines {
                        findings_total += 1;
                        tally_severity(
                            rule.severity,
                            &mut findings_error,
                            &mut findings_warning,
                            &mut findings_info,
                        );
                        responses.push(finding_response(
                            rule,
                            &rel_file,
                            line,
                            &format!("{} — {} lines (max: {})", name, lines, spec.max_lines),
                            "function_lines",
                        ));
                    }
                }

                // Check max_params.
                if spec.max_params > 0
                    && let Some(params) = fn_node.child_by_field_name("parameters")
                {
                    let param_count = count_parameter_decls(params) as i32;
                    if param_count > spec.max_params {
                        findings_total += 1;
                        tally_severity(
                            rule.severity,
                            &mut findings_error,
                            &mut findings_warning,
                            &mut findings_info,
                        );
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

                // Check max_returns.
                if spec.max_returns > 0 {
                    let return_count = count_return_values(fn_node) as i32;
                    if return_count > spec.max_returns {
                        findings_total += 1;
                        tally_severity(
                            rule.severity,
                            &mut findings_error,
                            &mut findings_warning,
                            &mut findings_info,
                        );
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
            // Stale means the rule could not run — no files in scope. A run
            // that analysed files and found no over-long functions has passed.
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
        if child.kind() == "parameter_declaration"
            || child.kind() == "variadic_parameter_declaration"
        {
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
fn count_return_values(fn_node: Node) -> usize {
    let result = match fn_node.child_by_field_name("result") {
        Some(r) => r,
        None => return 0,
    };

    match result.kind() {
        // Multiple return values are the only case wrapped in a parameter_list:
        // func() (int, error). Everything else is a single type node, however
        // many commas its text happens to contain.
        //
        // This used to enumerate the type kinds it knew and fall through to
        // counting commas in the node's *text*, so one return value whose type
        // contained commas was counted once per comma: `func() func(a, b, c int,
        // d, e string) error` was reported as 5 returns, and a generic like
        // `Result[string, error]` as 2. `max_return_values: 3` then failed
        // functions with a single return value, and the count in the message was
        // simply wrong. Listing kinds could never be complete — tree-sitter-go has
        // more type kinds than the list, and gains more over time — so the
        // relationship is inverted: a parameter_list means several, anything else
        // means one.
        "parameter_list" => {
            let mut count = 0usize;
            for i in 0..result.named_child_count() {
                if let Some(child) = result.named_child(i)
                    && child.kind() == "parameter_declaration"
                {
                    let names = child
                        .named_children(&mut child.walk())
                        .filter(|n| n.kind() == "identifier")
                        .count();
                    count += names.max(1);
                }
            }
            count
        }
        // Any other node is a single type, and therefore one return value.
        _ => 1,
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

fn finding_response(
    rule: &pb::Rule,
    file: &str,
    line: u32,
    message: &str,
    match_str: &str,
) -> EngineResponse {
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

    fn first_function<'a>(tree: &'a tree_sitter::Tree) -> Node<'a> {
        let root = tree.root_node();
        for i in 0..root.named_child_count() {
            let node = root.named_child(i).unwrap();
            if node.kind() == "function_declaration" || node.kind() == "method_declaration" {
                return node;
            }
        }
        panic!("no function in fixture");
    }

    /// `max_lines: N` must mean a body may be up to N lines.
    ///
    /// The body node spans the opening brace row to the closing brace row, and
    /// `end - start` counted one of those rows as code — so a function with
    /// exactly N lines in it was reported as N+1 and failed a limit it met. One
    /// function in this repository was in that position, and the debt baseline
    /// counted the false finding.
    #[test]
    fn body_lines_are_counted_between_the_braces() {
        for statements in [0usize, 1, 5, 10] {
            let body: String = (0..statements)
                .map(|i| format!("\ts{i} := {i}\n"))
                .collect();
            let src = format!("package p\nfunc f() {{\n{body}}}\n");
            let tree = parse(&src);
            let node = first_function(&tree);
            let body_node = node.child_by_field_name("body").expect("body");
            let counted = body_node
                .end_position()
                .row
                .saturating_sub(body_node.start_position().row)
                .saturating_sub(1);
            assert_eq!(
                counted, statements,
                "a body of {statements} lines must count as {statements}, got {counted}\n{src}"
            );
        }
    }

    /// A function returning one thing returns one thing, however complicated the
    /// type is.
    ///
    /// The fallback branch counted commas in the *text* of the result node, so a
    /// returned function type or a generic with several type arguments was
    /// counted once per comma: `func() func(a, b, c int, d, e string) error` came
    /// back as 5 returns rather than 1, and `max_return_values: 3` failed a
    /// function with a single return value.
    #[test]
    fn return_values_are_counted_by_type_not_by_comma() {
        let cases: Vec<(&str, &str, usize)> = vec![
            ("no result", "package p\nfunc f() {}\n", 0),
            ("single", "package p\nfunc f() int { return 0 }\n", 1),
            (
                "single pointer",
                "package p\nfunc f() *T { return nil }\n",
                1,
            ),
            (
                "single map",
                "package p\nfunc f() map[string]int { return nil }\n",
                1,
            ),
            (
                "two values",
                "package p\nfunc f() (int, error) { return 0, nil }\n",
                2,
            ),
            (
                "named values",
                "package p\nfunc f() (n int, err error) { return }\n",
                2,
            ),
            (
                "grouped named values",
                "package p\nfunc f() (a, b int, err error) { return }\n",
                3,
            ),
            // The regression: one return value whose type contains commas.
            (
                "returns a function type",
                "package p\nfunc f() func(a, b, c int, d, e string) error { return nil }\n",
                1,
            ),
            (
                "returns a generic type",
                "package p\nfunc f() Result[string, error] { var r Result[string, error]; return r }\n",
                1,
            ),
            (
                "returns a channel of func",
                "package p\nfunc f() chan func(a, b int) { return nil }\n",
                1,
            ),
            (
                "two values, one a function type",
                "package p\nfunc f() (func(a, b int), error) { return nil, nil }\n",
                2,
            ),
        ];

        for (name, src, want) in cases {
            let tree = parse(src);
            let got = count_return_values(first_function(&tree));
            assert_eq!(got, want, "{name}: {src:?}");
        }
    }
}
