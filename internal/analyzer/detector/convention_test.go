package detector

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

// packages.Load runs with Tests disabled, so no _test.go file ever reaches the
// detector: it reported "test files 0/N" on every project, and never saw the
// t.Run calls that identify a table-driven suite.
func TestDetectTestingSeesTestFiles(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedImports | packages.NeedSyntax | packages.NeedFiles, Dir: filepath.Join("..", "testdata", "hexagonal")}
	pkgs, err := packages.Load(cfg, "./...")
	require.NoError(t, err)

	result := DetectConventions(pkgs)

	// The fixture has 6 .go files, one of which is cmd/service/main_test.go.
	assert.Equal(t, 1, result.Testing.TestFiles)
	assert.Equal(t, 6, result.Testing.TotalGoFiles)
	assert.Equal(t, "table-driven", result.Testing.Pattern, "main_test.go uses t.Run")
}

// Subtest detection must look only at test files. Counting every ".Run(" call in
// production code labelled a project with zero tests as table-driven at 0.85
// confidence — a confident answer derived from a method that has nothing to do
// with testing.
func TestDetectTestingIgnoresRunCallsOutsideTests(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedImports | packages.NeedSyntax | packages.NeedFiles, Dir: filepath.Join("..", "testdata", "no-tests")}
	pkgs, err := packages.Load(cfg, "./...")
	require.NoError(t, err)

	result := DetectConventions(pkgs)

	assert.Equal(t, 0, result.Testing.TestFiles)
	assert.Equal(t, "minimal", result.Testing.Pattern, "s.Run() in main.go is not a subtest")
	assert.Empty(t, result.Testing.Evidence)
}

// A BDD library imported by production code is not evidence of a BDD test suite.
// The signal was taken from pkg.Imports and merged with the test-file signal, so any
// project with a test file plus a ginkgo import in a helper was labelled bdd.
func TestDetectTestingIgnoresBDDImportsOutsideTests(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedImports | packages.NeedSyntax | packages.NeedFiles, Dir: filepath.Join("..", "testdata", "hexagonal")}
	pkgs, err := packages.Load(cfg, "./...")
	require.NoError(t, err)

	// The fixture deliberately has a production file importing
	// internal/ginkgorunner — a path the BDD substring match hits — alongside a
	// test file that uses t.Run and no BDD library. So the pattern must be
	// table-driven, and would flip to bdd the moment a production import could
	// contribute to the signal again. Without that import the test passed
	// whichever implementation was in place.
	result := DetectConventions(pkgs)

	assert.Equal(t, "table-driven", result.Testing.Pattern)
	assert.NotContains(t, result.Testing.Evidence, "found BDD test libraries")
}

func TestDetectConventions(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedImports | packages.NeedSyntax | packages.NeedFiles, Dir: filepath.Join("..", "testdata", "hexagonal")}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	result := DetectConventions(pkgs)
	if result.Logging.Pattern == "" {
		t.Fatal("expected logging pattern")
	}
	if result.Testing.TotalGoFiles == 0 {
		t.Fatal("expected testing file stats")
	}
}
