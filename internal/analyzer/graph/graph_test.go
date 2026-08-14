package graph

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

func loadPkgs(t *testing.T, dir string) []*packages.Package {
	t.Helper()
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedImports | packages.NeedFiles | packages.NeedModule, Dir: dir}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("packages.Load() error = %v", err)
	}
	return pkgs
}

func TestBuildGraph(t *testing.T) {
	pkgs := loadPkgs(t, filepath.Join("..", "testdata", "hexagonal"))
	g := BuildGraph(pkgs)
	if len(g.Nodes) == 0 {
		t.Fatal("expected nodes")
	}
	if len(g.Edges) == 0 {
		t.Fatal("expected edges")
	}
}

func TestLayerViolations(t *testing.T) {
	pkgs := loadPkgs(t, filepath.Join("..", "testdata", "hexagonal"))
	g := BuildGraph(pkgs)
	components := []config.Component{
		{Name: "domain", In: []string{"domain/**"}, MayDependOn: []string{}},
		{Name: "ports", In: []string{"port/**"}, MayDependOn: []string{"domain"}},
		{Name: "adapters", In: []string{"adapter/**"}, MayDependOn: []string{"ports", "domain"}},
	}
	// The hexagonal fixture is compliant — no cross-layer imports to report.
	assert.Empty(t, LayerViolations(g, components))
}

// UnclaimedPackages is the single definition of "no component claims this
// package". checker wraps it as an orphan_package error and analyze reports it
// too — before, only checker knew about the condition.
func TestUnclaimedPackages(t *testing.T) {
	components := []config.Component{
		{Name: "domain", In: []string{"domain/**"}},
		{Name: "adapters", In: []string{"adapter/**"}},
	}
	paths := []string{
		"example.com/app/domain/order",
		"example.com/app/adapter/db",
		"example.com/app/internal/config",
		"example.com/app/cmd/server",
	}

	unclaimed := UnclaimedPackages(paths, components)

	assert.Equal(t, []string{"example.com/app/internal/config", "example.com/app/cmd/server"}, unclaimed)
}

// With no components declared there is nothing to be unclaimed against.
func TestUnclaimedPackages_NoComponents(t *testing.T) {
	assert.Empty(t, UnclaimedPackages([]string{"example.com/app/foo"}, nil))
}

// Only packages whose sources live under the project are considered — stdlib and
// third-party dependencies must never be reported as unclaimed.
func TestProjectLocalPackages_ExcludesDependenciesAndGlobs(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("..", "testdata", "hexagonal"))
	require.NoError(t, err)
	pkgs := loadPkgs(t, dir)

	local := ProjectLocalPackages(pkgs, dir, nil)

	require.NotEmpty(t, local)
	for _, p := range local {
		assert.Contains(t, p, "example.com/hexagonal", "third-party or stdlib package leaked in: %s", p)
	}
	assert.Contains(t, local, "example.com/hexagonal/internal/domain")

	// Exclude globs use the same matching as component `in:` patterns.
	excluded := ProjectLocalPackages(pkgs, dir, []string{"internal/domain/**"})
	assert.NotContains(t, excluded, "example.com/hexagonal/internal/domain")
	assert.Contains(t, excluded, "example.com/hexagonal/internal/port")
}

// GoFiles are absolute paths. A relative projectRoot — "." is the default for
// `verikt check` — matched no files, so orphan_package detection silently found
// nothing on the Go path.
func TestProjectLocalPackages_RelativeProjectRoot(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("..", "testdata", "hexagonal"))
	require.NoError(t, err)
	pkgs := loadPkgs(t, dir)

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(dir))

	assert.ElementsMatch(t, ProjectLocalPackages(pkgs, dir, nil), ProjectLocalPackages(pkgs, ".", nil))
}

// A sibling directory sharing a name prefix ("/tmp/app2" vs "/tmp/app") must not
// be treated as inside the project.
func TestProjectLocalPackages_SiblingPrefixNotIncluded(t *testing.T) {
	pkgs := []*packages.Package{
		{PkgPath: "example.com/app/inside", GoFiles: []string{filepath.Join("/tmp", "app", "inside", "x.go")}},
		{PkgPath: "example.com/app2/outside", GoFiles: []string{filepath.Join("/tmp", "app2", "outside", "y.go")}},
	}

	local := ProjectLocalPackages(pkgs, filepath.Join("/tmp", "app"), nil)

	assert.Equal(t, []string{"example.com/app/inside"}, local)
}

// guessLayer must agree with checker.isDomainPackage, which counts /core as a
// domain package. While it did not, a repo using internal/core as its domain
// scored no domain signal and `verikt analyze` reported "unrecognized".
func TestGuessLayer(t *testing.T) {
	tests := []struct {
		pkgPath string
		want    string
	}{
		{pkgPath: "example.com/app/internal/core", want: "domain"},
		{pkgPath: "example.com/app/internal/core/engine", want: "domain"},
		{pkgPath: "example.com/app/internal/domain/order", want: "domain"},
		{pkgPath: "example.com/app/internal/port/http", want: "ports"},
		{pkgPath: "example.com/app/internal/ports", want: "ports"},
		{pkgPath: "example.com/app/internal/adapter/db", want: "adapters"},
		{pkgPath: "example.com/app/internal/application", want: "application"},
		{pkgPath: "example.com/app/internal/usecase/create", want: "application"},
		{pkgPath: "example.com/app/internal/infrastructure/db", want: "infrastructure"},
		{pkgPath: "example.com/app/cmd/service", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.pkgPath, func(t *testing.T) {
			assert.Equal(t, tt.want, guessLayer(tt.pkgPath))
		})
	}
}

// layerGraph builds a synthetic graph with the Layer field populated exactly as
// BuildGraph would, so mapping precedence can be tested without loading packages.
func layerGraph(from, to string) provider.DependencyGraph {
	return provider.DependencyGraph{
		Nodes: []provider.PackageNode{
			{Path: from, Layer: guessLayer(from)},
			{Path: to, Layer: guessLayer(to)},
		},
		Edges: []provider.DependencyEdge{{From: from, To: to, ImportType: "import"}},
	}
}

// A declared component name that differs from what guessLayer infers for its
// directory must still have its may_depend_on enforced. Using the guessed layer
// as the lookup key silently exempted such components from all dependency
// enforcement (verikt.yaml is the source of truth — ADR-010).
func TestLayerViolations_DeclaredComponentNameWinsOverGuessedLayer(t *testing.T) {
	const (
		adapterPkg = "example.com/app/adapter/httphandler"
		servicePkg = "example.com/app/service/order"
	)

	tests := []struct {
		name          string
		componentName string
	}{
		{name: "singular name differs from guessed \"adapters\"", componentName: "adapter"},
		{name: "plural name matches guessed layer", componentName: "adapters"},
		{name: "misspelled name matches no guessed layer", componentName: "adaptr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			components := []config.Component{
				{Name: "domain", In: []string{"domain/**"}},
				{Name: "service", In: []string{"service/**"}, MayDependOn: []string{"domain"}},
				{Name: tt.componentName, In: []string{"adapter/**"}, MayDependOn: []string{"domain"}},
			}

			violations := LayerViolations(layerGraph(adapterPkg, servicePkg), components)

			require.Len(t, violations, 1, "adapter imports service, which is not in may_depend_on")
			assert.Equal(t, "dependency", violations[0].Rule)
			assert.Equal(t, adapterPkg, violations[0].Source)
			assert.Equal(t, servicePkg, violations[0].Target)
			assert.Equal(t, tt.componentName+" must not depend on service", violations[0].Message)
		})
	}
}

// An unclaimed package must not inherit a declared component's rules just because
// the guessed layer name happens to match that component's name.
//
// This is the mirror of the precedence bug above: there, a declared component lost
// its rules because the guess differed from its name; here, a package in no component
// gains rules because the guess equals one. Both come from mixing an inferred name
// with declared names in the same lookup.
func TestLayerViolations_InferredLayerDoesNotBorrowComponentRules(t *testing.T) {
	// A component is *named* "domain" but claims a differently-named directory, so
	// nothing it declares covers internal/domain/legacy. guessLayer still infers
	// "domain" for that package, which is the collision under test. (Component
	// patterns match by substring, so the claimed directory must not contain
	// "domain".)
	components := []config.Component{
		{Name: "domain", In: []string{"core-model/**"}},
		{Name: "adapters", In: []string{"adapter/**"}, MayDependOn: []string{"domain"}},
	}
	unclaimed := "example.com/app/internal/domain/legacy"
	if guessLayer(unclaimed) != "domain" {
		t.Fatalf("precondition: guessLayer(%q) = %q, want domain", unclaimed, guessLayer(unclaimed))
	}

	// The unclaimed package imports an adapter. If it had borrowed the domain
	// component's rules this would be reported as "domain must not depend on
	// adapters" — a rule that was never declared for it.
	violations := LayerViolations(layerGraph(unclaimed, "example.com/app/adapter/db"), components)

	assert.Empty(t, violations, "an unclaimed package must not be enforced as a component")
}

// With no components declared at all, guessLayer remains the only signal — that is
// the analyze path, and it must keep working.
func TestLayerViolations_InferredLayersStillUsedWithoutComponents(t *testing.T) {
	layers := mapLayers(layerGraph("example.com/app/adapter/db", "example.com/app/domain/order"), nil)
	assert.Equal(t, "adapters", layers["example.com/app/adapter/db"])
	assert.Equal(t, "domain", layers["example.com/app/domain/order"])
}

// A package no component claims has no rules to enforce. It is not reported
// here — checker.detectOrphanPackages already reports it as an error-severity
// orphan_package, so emitting a second finding would double-report one cause.
func TestLayerViolations_IgnoresPackagesNoComponentClaims(t *testing.T) {
	components := []config.Component{
		{Name: "domain", In: []string{"domain/**"}},
		{Name: "service", In: []string{"service/**"}, MayDependOn: []string{"domain"}},
	}

	violations := LayerViolations(layerGraph("example.com/app/adapter/httphandler", "example.com/app/service/order"), components)

	assert.Empty(t, violations)
}

// With no components declared at all there are no rules to enforce.
func TestLayerViolations_NoComponentsDeclared(t *testing.T) {
	assert.Empty(t, LayerViolations(layerGraph("example.com/app/adapter/httphandler", "example.com/app/domain/order"), nil))
}
