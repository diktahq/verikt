# INV-002: Symlinked Directories Are Never Treated as Project-Local

When traversing a project directory to detect Go packages, symlinked directories must be skipped. A symlink points outside the project boundary — following it would classify external code as part of the project's architecture.

## What This Means

- `detectOrphanPackagesFS` and any future filesystem walkers must check `d.Type()&fs.ModeSymlink != 0` and return `filepath.SkipDir`
- The Go packages loader (`go/packages`) already filters by `GoFiles` paths under `projectPath`, which naturally excludes symlinked external code
- A symlinked `vendor/` or `node_modules/` must never produce orphan violations

## Why

Symlinks in a project directory typically point to external tooling, generated artefacts, or shared mounts. Traversing them would produce false orphan violations and potentially infinite loops (circular symlinks). The project boundary is the directory tree — symlinks exit that boundary.

---

*Captured by edikt:invariant — 2026-03-13*

## Scope

Every symlink, not only symlinked directories. A symlink to a regular file is equally
not project-local code.

This needed stating: the constraint said "symlinked directories", the three engine
walkers implemented exactly that, and symlinks to files were still collected and
analysed. The wording was narrower than the intent and the code matched the wording.

## Proven by

- `internal/rules/scope_test.go` — `TestExpandScope_RelativeDotRootStillSkipsHiddenDirs`
- `engine/crates/engine-bin/src/import_graph.rs` — `collect_go_files_skips_symlinked_dirs`,
  `collect_go_files_skips_symlinked_files`
- `engine/crates/engine-bin/src/typescript_imports.rs` — `collect_ts_files_skips_symlinked_dirs`

An invariant with no named test is an intention. Naming them makes scope drift greppable:
if the constraint changes and no test moves, the gap is visible.
