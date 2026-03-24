use std::path::{Component, Path, PathBuf};
use tree_sitter::Parser;

/// Extract TypeScript/TSX module import specifiers from source.
///
/// Relative imports (`./foo`, `../bar`) are resolved against `file_path`'s
/// directory and returned as paths relative to `project_root`.
/// Non-relative imports (npm packages, path aliases) are returned as-is.
pub fn extract_ts_imports(file_path: &Path, source: &str, project_root: &Path) -> Vec<String> {
    let is_tsx = file_path.extension().map(|e| e == "tsx").unwrap_or(false);

    let mut parser = Parser::new();
    let lang = if is_tsx {
        tree_sitter_typescript::LANGUAGE_TSX.into()
    } else {
        tree_sitter_typescript::LANGUAGE_TYPESCRIPT.into()
    };
    parser
        .set_language(&lang)
        .expect("tree-sitter-typescript language load failed");

    let tree = match parser.parse(source, None) {
        Some(t) => t,
        None => return vec![],
    };

    let mut specifiers = Vec::new();
    collect_import_specifiers(tree.root_node(), source, &mut specifiers);

    let file_dir = file_path.parent().unwrap_or(Path::new(""));
    specifiers
        .into_iter()
        .map(|s| resolve_specifier(&s, file_dir, project_root))
        .collect()
}

/// Recursively collect import/require source strings from the AST.
fn collect_import_specifiers(node: tree_sitter::Node, source: &str, out: &mut Vec<String>) {
    match node.kind() {
        // import X from '...' | import '...' | export X from '...'
        "import_statement" | "export_statement" => {
            for i in 0..node.child_count() {
                if let Some(child) = node.child(i) {
                    if child.kind() == "string" {
                        push_string_value(child, source, out);
                        return;
                    }
                }
            }
        }
        // require('...') — CommonJS
        "call_expression" => {
            if let Some(fn_node) = node.child_by_field_name("function") {
                if &source[fn_node.byte_range()] == "require" {
                    if let Some(args) = node.child_by_field_name("arguments") {
                        for i in 0..args.child_count() {
                            let Some(arg) = args.child(i) else { continue };
                            if arg.kind() == "string" {
                                push_string_value(arg, source, out);
                                break;
                            }
                        }
                    }
                }
            }
        }
        _ => {}
    }
    for i in 0..node.child_count() {
        if let Some(child) = node.child(i) {
            collect_import_specifiers(child, source, out);
        }
    }
}

fn push_string_value(node: tree_sitter::Node, source: &str, out: &mut Vec<String>) {
    let raw = &source[node.byte_range()];
    // Strip surrounding quotes (single, double, or template literal).
    let inner = raw.trim_matches('"').trim_matches('\'').trim_matches('`');
    if !inner.is_empty() {
        out.push(inner.to_string());
    }
}

/// Resolve a TypeScript import specifier to a normalised path.
/// Relative specifiers are resolved against `file_dir` and made relative to
/// `project_root`. Non-relative specifiers are returned unchanged.
fn resolve_specifier(specifier: &str, file_dir: &Path, project_root: &Path) -> String {
    if !specifier.starts_with('.') {
        return specifier.to_string();
    }

    let resolved = normalize_path(&file_dir.join(specifier));
    let without_ext = strip_ts_extension(&resolved);

    without_ext
        .strip_prefix(project_root)
        .map(|p| p.to_string_lossy().replace('\\', "/"))
        .unwrap_or_else(|_| without_ext.to_string_lossy().replace('\\', "/"))
}

fn strip_ts_extension(path: &Path) -> PathBuf {
    match path.extension().and_then(|e| e.to_str()) {
        Some("ts" | "tsx" | "js" | "jsx" | "mts" | "cts") => path.with_extension(""),
        _ => path.to_path_buf(),
    }
}

fn normalize_path(path: &Path) -> PathBuf {
    let mut parts: Vec<Component> = Vec::new();
    for component in path.components() {
        match component {
            Component::CurDir => {}
            Component::ParentDir => match parts.last() {
                Some(Component::Normal(_)) => {
                    parts.pop();
                }
                _ => parts.push(component),
            },
            other => parts.push(other),
        }
    }
    parts.iter().collect()
}

/// Return true if a resolved TypeScript import is project-internal.
/// Relative imports are resolved to project-relative paths (e.g. `src/domain/index`).
/// NPM packages are either bare names (`express`) or scoped (`@prisma/client`).
/// A path is internal when it contains a `/` and is not a scoped npm package.
pub fn is_internal_ts_import(to_pkg: &str) -> bool {
    to_pkg.contains('/') && !to_pkg.starts_with('@')
}

/// Walk all `.ts` and `.tsx` files under the project, respecting target_files filter.
/// Skips `node_modules`, `dist`, `build`, and hidden directories.
pub fn collect_ts_files(project: &Path, target_files: &[String]) -> Vec<PathBuf> {
    if !target_files.is_empty() {
        return target_files
            .iter()
            .filter(|f| f.ends_with(".ts") || f.ends_with(".tsx"))
            .map(|f| project.join(f))
            .collect();
    }

    let mut files = Vec::new();
    collect_ts_files_recursive(project, &mut files);
    files
}

fn collect_ts_files_recursive(dir: &Path, files: &mut Vec<PathBuf>) {
    let entries = match std::fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };
    for entry in entries.flatten() {
        let path = entry.path();
        let name = entry.file_name();
        let name_str = name.to_string_lossy();
        if path.is_dir() {
            if matches!(
                name_str.as_ref(),
                "node_modules" | "dist" | "build" | "target" | ".git"
            ) || name_str.starts_with('.')
            {
                continue;
            }
            collect_ts_files_recursive(&path, files);
        } else if name_str.ends_with(".ts") || name_str.ends_with(".tsx") {
            files.push(path);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn no_root() -> PathBuf {
        PathBuf::from("/project")
    }

    fn parse(file: &str, src: &str) -> Vec<String> {
        let root = no_root();
        let path = root.join(file);
        let mut imports = extract_ts_imports(&path, src, &root);
        imports.sort();
        imports
    }

    #[test]
    fn default_import() {
        let imports = parse(
            "src/transport/http/server.ts",
            "import express from 'express';\n",
        );
        assert_eq!(imports, vec!["express"]);
    }

    #[test]
    fn named_import() {
        let imports = parse(
            "src/transport/http/server.ts",
            "import { Router } from 'express';\n",
        );
        assert_eq!(imports, vec!["express"]);
    }

    #[test]
    fn relative_import_resolved() {
        let imports = parse(
            "src/transport/http/server.ts",
            "import { Logger } from '../../infrastructure/logger';\n",
        );
        assert_eq!(imports, vec!["src/infrastructure/logger"]);
    }

    #[test]
    fn side_effect_import() {
        let imports = parse("src/index.ts", "import 'reflect-metadata';\n");
        assert_eq!(imports, vec!["reflect-metadata"]);
    }

    #[test]
    fn reexport_from() {
        let imports = parse(
            "src/domain/index.ts",
            "export { User } from './user';\n",
        );
        assert_eq!(imports, vec!["src/domain/user"]);
    }

    #[test]
    fn require_call() {
        let imports = parse("src/index.ts", "const fs = require('fs');\n");
        assert_eq!(imports, vec!["fs"]);
    }

    #[test]
    fn scoped_package_not_internal() {
        assert!(!is_internal_ts_import("@prisma/client"));
        assert!(!is_internal_ts_import("@types/node"));
    }

    #[test]
    fn project_relative_path_is_internal() {
        assert!(is_internal_ts_import("src/domain/user"));
        assert!(is_internal_ts_import("src/infrastructure/db"));
    }

    #[test]
    fn bare_package_not_internal() {
        assert!(!is_internal_ts_import("express"));
        assert!(!is_internal_ts_import("pino"));
    }

    #[test]
    fn tsx_file_parsed() {
        let imports = parse(
            "src/components/Button.tsx",
            "import React from 'react';\nimport { theme } from '../theme';\n",
        );
        assert_eq!(imports, vec!["react", "src/theme"]);
    }
}
