package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldIncludeFile_NilConditionals(t *testing.T) {
	if !shouldIncludeFile("any/file.go", nil, map[string]bool{"x": true}) {
		t.Fatal("nil conditionals should always include")
	}
}

func TestShouldIncludeFile_NilFeatures(t *testing.T) {
	cond := map[string]ConditionalRule{
		"x": {Include: []string{"a.go"}},
	}
	if !shouldIncludeFile("a.go", cond, nil) {
		t.Fatal("nil features should always include")
	}
}

func TestShouldIncludeFile_FeatureTrue_IncludeList(t *testing.T) {
	cond := map[string]ConditionalRule{
		"os_root": {Include: []string{"root.go"}},
	}
	features := map[string]bool{"os_root": true}
	if !shouldIncludeFile("root.go", cond, features) {
		t.Fatal("feature true + file in include list should be included")
	}
}

func TestShouldIncludeFile_FeatureFalse_IncludeList(t *testing.T) {
	cond := map[string]ConditionalRule{
		"os_root": {Include: []string{"root.go"}},
	}
	features := map[string]bool{"os_root": false}
	if shouldIncludeFile("root.go", cond, features) {
		t.Fatal("feature false + file in include list should be skipped")
	}
}

func TestShouldIncludeFile_FeatureTrue_ExcludeList(t *testing.T) {
	cond := map[string]ConditionalRule{
		"os_root": {Exclude: []string{"fallback.go"}},
	}
	features := map[string]bool{"os_root": true}
	if shouldIncludeFile("fallback.go", cond, features) {
		t.Fatal("feature true + file in exclude list should be skipped")
	}
}

func TestShouldIncludeFile_FeatureFalse_ExcludeList(t *testing.T) {
	cond := map[string]ConditionalRule{
		"os_root": {Exclude: []string{"fallback.go"}},
	}
	features := map[string]bool{"os_root": false}
	if !shouldIncludeFile("fallback.go", cond, features) {
		t.Fatal("feature false + file in exclude list should be included")
	}
}

func TestShouldIncludeFile_Conflicting_ExcludeWins(t *testing.T) {
	// Feature A is true and includes the file; feature B is true and excludes it.
	// Exclude wins.
	cond := map[string]ConditionalRule{
		"feat_a": {Include: []string{"shared.go"}},
		"feat_b": {Exclude: []string{"shared.go"}},
	}
	features := map[string]bool{"feat_a": true, "feat_b": true}
	if shouldIncludeFile("shared.go", cond, features) {
		t.Fatal("conflicting rules: exclude should win")
	}
}

func TestShouldIncludeFile_UnmentionedFile(t *testing.T) {
	cond := map[string]ConditionalRule{
		"os_root": {Include: []string{"root.go"}, Exclude: []string{"fallback.go"}},
	}
	features := map[string]bool{"os_root": true}
	if !shouldIncludeFile("other.go", cond, features) {
		t.Fatal("unmentioned file should always be included")
	}
}

func TestValidateConditionalRules_SameFileInBothLists(t *testing.T) {
	cond := map[string]ConditionalRule{
		"os_root": {Include: []string{"a.go"}, Exclude: []string{"a.go"}},
	}
	warnings := ValidateConditionalRules(cond)
	if len(warnings) == 0 {
		t.Fatal("expected warning for file in both include and exclude")
	}
}

func TestValidateConditionalRules_Clean(t *testing.T) {
	cond := map[string]ConditionalRule{
		"os_root": {Include: []string{"root.go"}, Exclude: []string{"fallback.go"}},
	}
	warnings := ValidateConditionalRules(cond)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestParseManifestWithConditionals(t *testing.T) {
	data := []byte(`name: test
language: go
conditional:
  os_root:
    include: ["root.go"]
    exclude: ["fallback.go"]
`)
	m, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	rule, ok := m.Conditional["os_root"]
	if !ok {
		t.Fatal("expected conditional rule for os_root")
	}
	if len(rule.Include) != 1 || rule.Include[0] != "root.go" {
		t.Fatalf("unexpected include list: %v", rule.Include)
	}
	if len(rule.Exclude) != 1 || rule.Exclude[0] != "fallback.go" {
		t.Fatalf("unexpected exclude list: %v", rule.Exclude)
	}
}

func TestRenderTemplate_ConditionalInclusion(t *testing.T) {
	renderer := NewRenderer(os.DirFS("testdata"))
	out := t.TempDir()

	// Feature os_root=true: root.go included, fallback.go excluded.
	result, err := renderer.RenderTemplate("conditional", out, map[string]interface{}{
		"ServiceName": "test-svc",
		"Features":    map[string]bool{"os_root": true},
	})
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}

	rootPath := filepath.Join(out, "internal", "safepath", "root.go")
	fallbackPath := filepath.Join(out, "internal", "safepath", "fallback.go")

	if _, err := os.Stat(rootPath); err != nil {
		t.Fatalf("expected root.go to exist: %v", err)
	}
	if _, err := os.Stat(fallbackPath); err == nil {
		t.Fatal("expected fallback.go to NOT exist")
	}

	// Verify main.go (unconditional) was created.
	found := false
	for _, f := range result.FilesCreated {
		if filepath.Base(f) == "main.go" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected main.go in result")
	}
}

func TestRenderTemplate_ConditionalExclusion(t *testing.T) {
	renderer := NewRenderer(os.DirFS("testdata"))
	out := t.TempDir()

	// Feature os_root=false: root.go excluded, fallback.go included.
	_, err := renderer.RenderTemplate("conditional", out, map[string]interface{}{
		"ServiceName": "test-svc",
		"Features":    map[string]bool{"os_root": false},
	})
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}

	rootPath := filepath.Join(out, "internal", "safepath", "root.go")
	fallbackPath := filepath.Join(out, "internal", "safepath", "fallback.go")

	if _, err := os.Stat(rootPath); err == nil {
		t.Fatal("expected root.go to NOT exist when os_root=false")
	}
	if _, err := os.Stat(fallbackPath); err != nil {
		t.Fatalf("expected fallback.go to exist: %v", err)
	}
}

func TestRenderTemplate_NoFeatures_BackwardCompat(t *testing.T) {
	renderer := NewRenderer(os.DirFS("testdata"))
	out := t.TempDir()

	// No Features in vars — all files should be included (backward compat).
	_, err := renderer.RenderTemplate("conditional", out, map[string]interface{}{
		"ServiceName": "test-svc",
	})
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}

	rootPath := filepath.Join(out, "internal", "safepath", "root.go")
	fallbackPath := filepath.Join(out, "internal", "safepath", "fallback.go")

	if _, err := os.Stat(rootPath); err != nil {
		t.Fatalf("expected root.go to exist with no features: %v", err)
	}
	if _, err := os.Stat(fallbackPath); err != nil {
		t.Fatalf("expected fallback.go to exist with no features: %v", err)
	}
}
