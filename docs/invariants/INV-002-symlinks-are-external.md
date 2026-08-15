# INV-002: Symlinks Are Never Treated as Project-Local

When traversing a project directory, every symlink must be skipped — files as well as directories. A symlink points outside the project boundary — following it would classify external code as part of the project's architecture.

## What This Means

- `detectOrphanPackagesFS`, `ExpandScope` and any future filesystem walkers must check `d.Type()&fs.ModeSymlink != 0` and skip the entry, before any `d.IsDir()` filter
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

- `internal/rules/scope_symlink_test.go` — `TestExpandScopeSkipsSymlinkedFiles`,
  `TestExpandScopeSkipsSymlinkedDirectories`
- `engine/crates/engine-bin/src/import_graph.rs` — `collect_go_files_skips_symlinked_dirs`,
  `collect_go_files_skips_symlinked_files`
- `engine/crates/engine-bin/src/typescript_imports.rs` — `collect_ts_files_skips_symlinked_dirs`

An invariant with no named test is an intention. Naming them makes scope drift greppable:
if the constraint changes and no test moves, the gap is visible.

That is exactly what happened here, and the list itself hid it. When the scope was
widened from directories to every symlink, only the engine walkers were changed;
`internal/rules/scope.go` still tested `d.IsDir() && d.Type()&fs.ModeSymlink != 0`,
so a symlink to a file outside the project was pulled into proxy-rule scope and
grepped. The Go entry named `TestExpandScope_RelativeDotRootStillSkipsHiddenDirs`,
whose fixture is `.git/`, `.hidden/` and `vendor/` — no symlink anywhere in it. A
named test that does not exercise the constraint is worse than no entry: it reads
as coverage.

Worth knowing for anyone touching the Go side: `filepath.WalkDir` does not follow
symlinks and reports them from `Lstat`, so `d.IsDir()` is false even for a link to
a directory. The old guard could therefore never fire — linked directories were
skipped only because the walk never descended into them. Test the type alone.
