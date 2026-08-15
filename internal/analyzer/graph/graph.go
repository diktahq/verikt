package graph

import (
	"path/filepath"
	"strings"

	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/pathglob"
	"github.com/diktahq/verikt/internal/provider"
	"golang.org/x/tools/go/packages"
)

func BuildGraph(pkgs []*packages.Package) provider.DependencyGraph {
	nodes := make(map[string]provider.PackageNode)
	edgeSet := make(map[string]provider.DependencyEdge)

	for _, pkg := range pkgs {
		if pkg == nil || pkg.PkgPath == "" {
			continue
		}
		nodes[pkg.PkgPath] = provider.PackageNode{
			Path:       pkg.PkgPath,
			Name:       pkg.Name,
			IsInternal: strings.Contains(pkg.PkgPath, "/internal/"),
			Layer:      guessLayer(pkg.PkgPath),
		}
		for importPath := range pkg.Imports {
			edge := provider.DependencyEdge{From: pkg.PkgPath, To: importPath, ImportType: "import"}
			key := edge.From + "->" + edge.To
			edgeSet[key] = edge
		}
	}

	graph := provider.DependencyGraph{
		Nodes: make([]provider.PackageNode, 0, len(nodes)),
		Edges: make([]provider.DependencyEdge, 0, len(edgeSet)),
	}

	for _, node := range nodes {
		graph.Nodes = append(graph.Nodes, node)
	}
	for _, edge := range edgeSet {
		graph.Edges = append(graph.Edges, edge)
	}
	graph.Cycles = FindCycles(graph)
	return graph
}

func FindCycles(graph provider.DependencyGraph) [][]string {
	adj := map[string][]string{}
	for _, edge := range graph.Edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
	}

	visited := map[string]bool{}
	stack := map[string]bool{}
	pathStack := []string{}
	cycles := [][]string{}

	var dfs func(node string)
	dfs = func(node string) {
		visited[node] = true
		stack[node] = true
		pathStack = append(pathStack, node)

		for _, next := range adj[node] {
			if !visited[next] {
				dfs(next)
				continue
			}
			if stack[next] {
				cycle := []string{}
				for i := len(pathStack) - 1; i >= 0; i-- {
					cycle = append([]string{pathStack[i]}, cycle...)
					if pathStack[i] == next {
						break
					}
				}
				if len(cycle) > 1 {
					cycles = append(cycles, cycle)
				}
			}
		}

		stack[node] = false
		pathStack = pathStack[:len(pathStack)-1]
	}

	for _, node := range graph.Nodes {
		if !visited[node.Path] {
			dfs(node.Path)
		}
	}

	return dedupeCycles(cycles)
}

func LayerViolations(graph provider.DependencyGraph, components []config.Component) []provider.Violation {
	allowed := map[string]map[string]bool{}
	for _, comp := range components {
		allow := make(map[string]bool, len(comp.MayDependOn))
		for _, target := range comp.MayDependOn {
			allow[target] = true
		}
		allowed[comp.Name] = allow
	}

	layerByPkg := mapLayers(graph, components)

	violations := []provider.Violation{}
	for _, edge := range graph.Edges {
		srcLayer := layerByPkg[edge.From]
		tgtLayer := layerByPkg[edge.To]
		if srcLayer == "" || tgtLayer == "" || srcLayer == tgtLayer {
			continue
		}
		// A source layer that is not a declared component name has no rules to
		// look up. That only happens for packages no component claims, which
		// checker.detectOrphanPackages already reports as an error.
		allowedLayers, ok := allowed[srcLayer]
		if !ok || allowedLayers[tgtLayer] {
			continue
		}
		violations = append(violations, provider.Violation{
			Rule:     "dependency",
			Message:  srcLayer + " must not depend on " + tgtLayer,
			Source:   edge.From,
			Target:   edge.To,
			Severity: "error",
		})
	}
	return violations
}

// mapLayers resolves each package to the layer used for dependency lookups.
//
// Declared components win: verikt.yaml is the source of truth (ADR-010), and
// guessLayer is only a fallback for packages no component claims. The other
// order silently dropped violations whenever the guessed layer name ("adapters"
// for adapter/**) differed from the declared component name ("adapter").
func mapLayers(graph provider.DependencyGraph, components []config.Component) map[string]string {
	layerByPkg := map[string]string{}

	for _, node := range graph.Nodes {
		layer := ""
		for _, comp := range components {
			if matchesAnyRule(node.Path, comp.In) {
				layer = comp.Name
				break
			}
		}

		// When components are declared, only a package a component claims takes part
		// in dependency enforcement. Falling back to the guessed layer let an
		// unclaimed package inherit a declared component's may_depend_on whenever the
		// guess happened to equal that component's name — the mirror of the bug where
		// a declared component lost its rules because the guess differed. Unclaimed
		// packages are reported separately, as orphan_package.
		if layer == "" && len(components) == 0 {
			layer = node.Layer
		}

		if layer != "" {
			layerByPkg[node.Path] = layer
		}
	}

	return layerByPkg
}

// MatchesComponent returns true if the package path matches any of the component's patterns.
func MatchesComponent(pkgPath string, comp config.Component) bool {
	return matchesAnyRule(pkgPath, comp.In)
}

// UnclaimedPackages returns the package paths that no declared component claims,
// preserving input order. These are packages verikt cannot enforce rules for:
// checker reports them as orphan_package errors and analyze surfaces them too.
//
// With no components declared there is nothing to claim against, so the result
// is empty rather than "everything".
func UnclaimedPackages(pkgPaths []string, components []config.Component) []string {
	if len(components) == 0 {
		return nil
	}

	var unclaimed []string
	for _, pkgPath := range pkgPaths {
		claimed := false
		for _, comp := range components {
			if MatchesComponent(pkgPath, comp) {
				claimed = true
				break
			}
		}
		if !claimed {
			unclaimed = append(unclaimed, pkgPath)
		}
	}
	return unclaimed
}

// ProjectLocalPackages returns the import paths of packages whose source files
// live under projectPath, skipping any that match an exclude glob. Stdlib and
// third-party dependencies are filtered out: rules only apply to project code.
//
// Exclude globs use the same matching as component `in:` patterns.
func ProjectLocalPackages(pkgs []*packages.Package, projectPath string, excludes []string) []string {
	// GoFiles are absolute, so projectPath must be too: a relative root (".",
	// the default for `verikt check`) matched nothing and silently reported zero
	// project packages. The trailing separator stops a sibling directory with a
	// shared name prefix ("/tmp/app2" for root "/tmp/app") from matching.
	root, err := filepath.Abs(projectPath)
	if err != nil {
		root = projectPath
	}
	if !strings.HasSuffix(root, string(filepath.Separator)) {
		root += string(filepath.Separator)
	}

	seen := map[string]bool{}
	var result []string
	for _, pkg := range pkgs {
		if pkg == nil || pkg.PkgPath == "" || seen[pkg.PkgPath] {
			continue
		}
		if matchesAnyRule(pkg.PkgPath, excludes) {
			continue
		}
		for _, f := range pkg.GoFiles {
			if strings.HasPrefix(f, root) {
				seen[pkg.PkgPath] = true
				result = append(result, pkg.PkgPath)
				break
			}
		}
	}
	return result
}

func matchesAnyRule(pkgPath string, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		// Patterns are project-relative ("internal/domain/**") while pkgPath is a
		// full import path ("example.com/app/internal/domain"), so the pattern is
		// matched at any depth.
		//
		// This was `strings.Contains(pkgPath, prefix)` for "/**" patterns and
		// path.Match for the rest — the fourth copy of the matcher this project
		// had, and the one missed when the other three were consolidated. It
		// carried both original defects: `in: ["**"]` matched nothing, because
		// path.Match treats "*" as not crossing "/", so a catch-all component
		// claimed no packages and every one of them was reported as an orphan.
		// And "domain/**" claimed "subdomain/x" by substring.
		if pathglob.Match(pattern, pkgPath) || pathglob.Match("**/"+pattern, pkgPath) {
			return true
		}
	}
	return false
}

func guessLayer(pkgPath string) string {
	lower := strings.ToLower(pkgPath)
	switch {
	// "/core" is kept in sync with checker.isDomainPackage, which also treats it
	// as the domain. Omitting it here made analyze report "unrecognized" for
	// projects that name their domain package core.
	case strings.Contains(lower, "/domain") || strings.Contains(lower, "/core"):
		return "domain"
	case strings.Contains(lower, "/port") || strings.Contains(lower, "/ports"):
		return "ports"
	case strings.Contains(lower, "/adapter") || strings.Contains(lower, "/adapters"):
		return "adapters"
	case strings.Contains(lower, "/application") || strings.Contains(lower, "/usecase"):
		return "application"
	case strings.Contains(lower, "/infrastructure"):
		return "infrastructure"
	default:
		return ""
	}
}

func dedupeCycles(cycles [][]string) [][]string {
	seen := map[string]bool{}
	out := make([][]string, 0, len(cycles))
	for _, cycle := range cycles {
		key := strings.Join(cycle, "->")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, cycle)
	}
	return out
}
