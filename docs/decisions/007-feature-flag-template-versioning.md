# ADR-007: Feature-Flag Template Engine for Cross-Language Version Compatibility

**Status:** accepted
**Date:** 2026-03-11

---

## Context

verikt scaffolds production code from templates. The generated code uses language-specific APIs and patterns that change across language versions. For example:

- Go 1.24 introduced `os.Root` for kernel-level path traversal protection
- Go 1.21 introduced the `slices` stdlib package
- Go 1.22 introduced range-over-int
- Python 3.10 introduced structural pattern matching
- PHP 8.1 introduced enums
- TypeScript 5.0 introduced the `satisfies` operator

Our templates currently hardcode patterns for a single language version. If a user targets Go 1.22, templates using `os.Root` (Go 1.24+) won't compile. This problem exists for every language verikt will support (ADR-006 polyglot architecture).

This is not a new problem — Helm, Spring Initializr, Cookiecutter, cargo-generate, and Protobuf Editions all solve variants of it.

## Decision

Templates check **feature flags**, never version numbers. The scaffold engine resolves features from the target language version at scaffold time.

### Three layers

**1. Provider declares a Feature Matrix**

Each language provider ships a `features.yaml` mapping features to the version that introduced them:

```yaml
# providers/golang/features.yaml
features:
  - name: slices_package
    description: "stdlib slices package (slices.SortFunc, slices.Contains, etc.)"
    since: "1.21"
  - name: range_over_int
    description: "for i := range n syntax"
    since: "1.22"
  - name: os_root
    description: "os.Root for kernel-level path traversal protection"
    since: "1.24"
```

**2. Scaffold engine resolves features at render time**

At scaffold time:
1. Auto-detect language version (`go env GOVERSION`, `node --version`, `php -v`, etc.)
2. Load the provider's feature matrix
3. Resolve: every feature where `detected_version >= since` becomes `true`
4. Inject resolved features into template variables as `Features.<name>`

This logic lives in the **core scaffold engine**, not in any provider. Providers declare facts; the engine resolves them.

**3. Templates use feature flags**

```go
{{if .Features.os_root}}
root, err := os.OpenRoot(dir)
defer root.Close()
{{else}}
absPath := filepath.Join(dir, rel)
if !strings.HasPrefix(absPath, dir) {
    return fmt.Errorf("path traversal detected")
}
{{end}}
```

### Conditional file inclusion

Entire files can be gated on features in the manifest:

```yaml
conditional:
  os_root:
    include: ["internal/safepath/root.go.tmpl"]
    exclude: ["internal/safepath/fallback.go.tmpl"]
```

### Required features

Templates and architectures can declare minimum feature requirements:

```yaml
# architectures/hexagonal/manifest.yaml
requires_features: [slices_package]
```

If the detected version lacks a required feature, verikt gives a clear error with upgrade instructions rather than generating broken code.

### Provider interface extension

The existing `LanguageProvider` interface is NOT modified. Instead, two optional interfaces
are added. Providers implement them to opt in. Callers use type assertions:

```go
// Existing — unchanged
type LanguageProvider interface {
    Scaffold(ctx context.Context, req ScaffoldRequest) (*ScaffoldResponse, error)
    Analyze(ctx context.Context, req AnalyzeRequest) (*AnalyzeResponse, error)
    Migrate(ctx context.Context, req MigrateRequest) (*MigrateResponse, error)
    GetInfo(ctx context.Context) (*ProviderInfo, error)
    GetTemplateFS() fs.FS
}

// NEW — optional interfaces (idiomatic Go: io.Reader vs io.ReadCloser pattern)
type VersionDetector interface {
    DetectVersion(ctx context.Context) (string, error)
}

type FeatureMatrixProvider interface {
    GetFeatureMatrix() ([]byte, error)
}

// Usage at call site:
if vd, ok := provider.(VersionDetector); ok {
    version, err = vd.DetectVersion(ctx)
}

type Feature struct {
    Name        string
    Description string
    Since       string // semver — minimum version that introduced this feature
}
```

### Version detection

Each provider implements `DetectVersion()` using the language's standard tooling:

| Language | Detection command | Parse example |
|---|---|---|
| Go | `go env GOVERSION` | `go1.26.1` → `1.26` |
| TypeScript | `tsc --version` or `package.json` engines | `5.2.2` → `5.2` |
| Python | `python3 --version` | `3.12.1` → `3.12` |
| PHP | `php -v` | `8.3.1` → `8.3` |
| Ruby | `ruby -v` | `3.3.0` → `3.3` |
| Rust | `rustc --version` + edition in `Cargo.toml` | `1.75.0` + `2021` |

If detection fails (tool not installed), fall back to the manifest's default version.

## Rationale

### Why feature flags, not version checks in templates

- Template authors think in capabilities ("does this version have `os.Root`?"), not version numbers ("is this >= 1.24?")
- Feature flags are language-agnostic — the same `{{if .Features.pattern_matching}}` pattern works for Go, Python, Ruby
- Feature names are self-documenting; version numbers require lookup
- A feature matrix is a single source of truth, vs. scattered version checks in N templates

### Why not separate template directories per version

- Maintenance nightmare — every change must be applied to N copies
- Scales terribly with the version matrix: 5 Go versions × 7 architectures × 63 capabilities = combinatorial explosion
- Most version differences are small (a function name, an import) — not worth duplicating entire files

### Why not minimum floor only

- Works for a single language (what Rails does), but verikt is polyglot
- PHP 7→8 and Python 2→3 have massive API surface differences — users legitimately need to target older versions
- Enterprise environments often can't upgrade freely (compliance, legacy integration)
- A minimum floor forces users to choose between verikt and their language version

## Alternatives Considered

### Separate template directories per version
- **Pros:** Simple mental model, no conditional logic in templates
- **Cons:** Combinatorial explosion, massive duplication, impossible to maintain across languages

### Version number checks in templates (`{{if ge .GoVersion "1.24"}}`)
- **Pros:** Simple to implement, no extra infrastructure
- **Cons:** Language-specific, template authors must know version history, version strings vary across languages (semver vs date vs edition)

### Minimum version floor only (Rails model)
- **Pros:** Simplest possible approach, zero maintenance overhead
- **Cons:** Excludes users on older versions, doesn't work for polyglot, enterprise-hostile

### Programmatic contributors (Spring Initializr model)
- **Pros:** Maximum flexibility, real code instead of template conditionals
- **Cons:** Massively over-engineered for verikt's scale, requires Go code for every version difference, breaks the "templates are the source of truth" principle

## Consequences

### Positive
- Templates stay readable — feature flags are self-documenting
- One template set per architecture, not one per version × architecture
- Adding a new language requires declaring a feature matrix, not building version-branching infrastructure
- Users on older language versions get working code, not compilation errors
- The scaffold engine is language-agnostic — providers just declare facts

### Negative / Trade-offs
- Templates with many conditional blocks become harder to read (mitigated by conditional file inclusion for structural differences)
- Feature matrices must be maintained as languages evolve (but it's a YAML file, not code)
- Detection can fail if the language toolchain isn't installed (mitigated by manifest defaults)
- Initial implementation cost for the feature resolution engine

---

*Captured by edikt:adr — 2026-03-11*
