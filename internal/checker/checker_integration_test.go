package checker

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/diktahq/verikt/internal/config"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(filename), "testdata")
}

func TestCheck_Integration_HexagonalProject(t *testing.T) {
	projectPath := filepath.Join(testdataDir(t), "hexagonal-project")

	cfg := &config.VeriktConfig{
		Language:     "go",
		Architecture: "hexagonal",
		Components: []config.Component{
			{Name: "domain", In: []string{"domain/**"}},
			{Name: "port", In: []string{"port/**"}},
			{Name: "service", In: []string{"service/**"}, MayDependOn: []string{"domain", "port"}},
			{Name: "adapter", In: []string{"adapter/**"}, MayDependOn: []string{"domain", "port", "service"}},
		},
		Rules: config.RulesConfig{
			Structure: config.StructureConfig{
				RequiredDirs:  []string{"domain/", "port/", "service/", "adapter/"},
				ForbiddenDirs: []string{"models/", "controllers/"},
			},
			Functions: config.FunctionRules{
				MaxLines:        80,
				MaxParams:       5,
				MaxReturnValues: 3,
			},
		},
	}

	result, err := Check(cfg, projectPath)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}

	// Structure should pass — all required dirs exist, no forbidden dirs.
	if len(result.StructureViolations) != 0 {
		t.Errorf("expected 0 structure violations, got %d: %v", len(result.StructureViolations), result.StructureViolations)
	}

	// Anti-patterns should be detected.
	if len(result.AntiPatternViolations) == 0 {
		t.Fatal("expected anti-pattern violations, got 0")
	}

	// Check that specific anti-patterns were found.
	found := map[string]bool{}
	for _, ap := range result.AntiPatternViolations {
		found[ap.Name] = true
	}

	expectedDetectors := []string{
		"global_mutable_state",
		"swallowed_error",
		"naked_goroutine",
		"context_background_in_handler",
		"sql_concatenation",
	}
	for _, detector := range expectedDetectors {
		if !found[detector] {
			t.Errorf("expected detector %q to fire, but it didn't. Found: %v", detector, found)
		}
	}

	// Result should not pass.
	if result.Passed() {
		t.Error("expected result to fail, but Passed() returned true")
	}

	// Compliance should be < 1.0.
	if result.Compliance() >= 1.0 {
		t.Errorf("expected compliance < 1.0, got %f", result.Compliance())
	}

	// Components should be covered.
	if result.ComponentsCovered == 0 {
		t.Error("expected at least some components covered")
	}

	t.Logf("Anti-patterns found: %d", len(result.AntiPatternViolations))
	for _, ap := range result.AntiPatternViolations {
		t.Logf("  [%s] %s:%d — %s", ap.Name, ap.File, ap.Line, ap.Message)
	}
}

// Component names that differ from the layer guessed for their directory used to
// be exempt from dependency enforcement: the guessed name ("domain" for
// domain/**) was used as the lookup key into a map built from declared names, and
// the miss was an unlogged skip. Here the domain directory is declared as "core",
// and domain/bad_dep.go imports service, which "core" does not allow.
func TestCheck_Integration_EnforcesRenamedComponent(t *testing.T) {
	projectPath := filepath.Join(testdataDir(t), "hexagonal-project")

	cfg := &config.VeriktConfig{
		Language:     "go",
		Architecture: "hexagonal",
		Components: []config.Component{
			{Name: "core", In: []string{"domain/**"}},
			{Name: "service", In: []string{"service/**"}, MayDependOn: []string{"core"}},
			{Name: "adapter", In: []string{"adapter/**"}, MayDependOn: []string{"core", "service"}},
		},
	}

	result, err := Check(cfg, projectPath)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}

	found := false
	for _, v := range result.DependencyViolations {
		if v.Rule == "dependency" && v.Message == "core must not depend on service" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the renamed component's dependency rule to be enforced, got %v", result.DependencyViolations)
	}
}

func TestCheck_Integration_CleanProject(t *testing.T) {
	// Test with a minimal config and the same project — structure only.
	projectPath := filepath.Join(testdataDir(t), "hexagonal-project")

	cfg := &config.VeriktConfig{
		Language:     "go",
		Architecture: "hexagonal",
		Rules: config.RulesConfig{
			Structure: config.StructureConfig{
				RequiredDirs: []string{"domain/", "port/"},
			},
		},
	}

	result, err := Check(cfg, projectPath)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}

	if len(result.StructureViolations) != 0 {
		t.Errorf("expected 0 structure violations, got %d", len(result.StructureViolations))
	}
}
