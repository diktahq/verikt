# INV-002: Symlinked Directories Are Never Treated as Project-Local

When traversing a project directory to detect Go packages, symlinked directories must be skipped. A symlink points outside the project boundary — following it would classify external code as part of the project's architecture.

## What This Means

- `detectOrphanPackagesFS` and any future filesystem walkers must check `d.Type()&fs.ModeSymlink != 0` and return `filepath.SkipDir`
- The Go packages loader (`go/packages`) already filters by `GoFiles` paths under `projectPath`, which naturally excludes symlinked external code
- A symlinked `vendor/` or `node_modules/` must never produce orphan violations

## Why

Symlinks in a project directory typically point to external tooling, generated artefacts, or shared mounts. Traversing them would produce false orphan violations and potentially infinite loops (circular symlinks). The project boundary is the directory tree — symlinks exit that boundary.

---

*Captured by keel:invariant — 2026-03-13*
