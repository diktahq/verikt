package scaffold

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderComposition_Basic(t *testing.T) {
	memFS := fstest.MapFS{
		"templates/architectures/flat/files/main.go.tmpl": &fstest.MapFile{
			Data: []byte("package main\n\nfunc main() {}\n"),
		},
		"templates/architectures/flat/files/go.mod.tmpl": &fstest.MapFile{
			Data: []byte("module {{.ModulePath}}\n\ngo {{.GoVersion}}\n"),
		},
	}

	renderer := NewRenderer(memFS)
	out := t.TempDir()

	plan := &CompositionPlan{
		Architecture: "flat",
		Capabilities: []string{},
		Manifest: &Manifest{
			Name: "flat",
		},
		CapManifests: []CapabilityManifest{},
		Vars: map[string]interface{}{
			"ModulePath":  "github.com/test/svc",
			"GoVersion":   "1.22",
			"ServiceName": "svc",
		},
		Partials: map[string][]string{},
		ArchDir:  "templates/architectures/flat",
		CapDirs:  []string{},
	}

	result, err := renderer.RenderComposition(plan, out)
	require.NoError(t, err)
	assert.NotEmpty(t, result.FilesCreated)

	// Verify go.mod was rendered with variable
	modBytes, err := os.ReadFile(filepath.Join(out, "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(modBytes), "github.com/test/svc")
	assert.Contains(t, string(modBytes), "go 1.22")
}

func TestRenderComposition_WithCapability(t *testing.T) {
	memFS := fstest.MapFS{
		"templates/architectures/flat/files/main.go.tmpl": &fstest.MapFile{
			Data: []byte("package main\n"),
		},
		"templates/capabilities/health/files/health.go.tmpl": &fstest.MapFile{
			Data: []byte("package health\n\nfunc Check() string { return \"ok\" }\n"),
		},
	}

	renderer := NewRenderer(memFS)
	out := t.TempDir()

	plan := &CompositionPlan{
		Architecture: "flat",
		Capabilities: []string{"health"},
		Manifest:     &Manifest{Name: "flat"},
		CapManifests: []CapabilityManifest{{Name: "health"}},
		Vars: map[string]interface{}{
			"ServiceName": "svc",
		},
		Partials: map[string][]string{},
		ArchDir:  "templates/architectures/flat",
		CapDirs:  []string{"templates/capabilities/health"},
	}

	result, err := renderer.RenderComposition(plan, out)
	require.NoError(t, err)
	assert.NotEmpty(t, result.FilesCreated)

	// Verify capability file was rendered
	healthBytes, err := os.ReadFile(filepath.Join(out, "health.go"))
	require.NoError(t, err)
	assert.Contains(t, string(healthBytes), "package health")
}

func TestRenderComposition_WithFeatureConditionals(t *testing.T) {
	memFS := fstest.MapFS{
		"arch/files/main.go.tmpl": &fstest.MapFile{
			Data: []byte("package main\n"),
		},
		"arch/files/internal/root.go.tmpl": &fstest.MapFile{
			Data: []byte("// root\n"),
		},
		"arch/files/internal/fallback.go.tmpl": &fstest.MapFile{
			Data: []byte("// fallback\n"),
		},
	}

	renderer := NewRenderer(memFS)
	out := t.TempDir()

	plan := &CompositionPlan{
		Architecture: "test",
		Capabilities: []string{},
		Manifest: &Manifest{
			Name: "test",
			Conditional: map[string]ConditionalRule{
				"os_root": {
					Include: []string{"internal/root.go.tmpl"},
					Exclude: []string{"internal/fallback.go.tmpl"},
				},
			},
		},
		CapManifests: []CapabilityManifest{},
		Vars: map[string]interface{}{
			"ServiceName": "svc",
			"Features":    map[string]bool{"os_root": true},
		},
		Partials: map[string][]string{},
		ArchDir:  "arch",
		CapDirs:  []string{},
	}

	result, err := renderer.RenderComposition(plan, out)
	require.NoError(t, err)
	assert.NotEmpty(t, result.FilesCreated)

	// root.go should exist (os_root=true, include list)
	_, err = os.Stat(filepath.Join(out, "internal", "root.go"))
	assert.NoError(t, err)

	// fallback.go should NOT exist (os_root=true, exclude list)
	_, err = os.Stat(filepath.Join(out, "internal", "fallback.go"))
	assert.True(t, os.IsNotExist(err))
}

func TestRenderCapabilityFiles(t *testing.T) {
	memFS := fstest.MapFS{
		"cap/files/health.go.tmpl": &fstest.MapFile{
			Data: []byte("package health\n"),
		},
		"cap/files/config.yaml.tmpl": &fstest.MapFile{
			Data: []byte("service: {{.ServiceName}}\n"),
		},
	}

	renderer := NewRenderer(memFS)
	out := t.TempDir()

	result, conflicts, err := renderer.RenderCapabilityFiles("cap", out, map[string]interface{}{
		"ServiceName": "orders",
	})
	require.NoError(t, err)
	assert.Empty(t, conflicts)
	assert.NotEmpty(t, result.FilesCreated)

	configBytes, err := os.ReadFile(filepath.Join(out, "config.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(configBytes), "service: orders")
}

func TestComposeProject_Minimal(t *testing.T) {
	memFS := fstest.MapFS{
		"templates/architectures/flat/manifest.yaml": &fstest.MapFile{
			Data: []byte("name: flat\nvariables:\n  - name: ServiceName\n    type: string\n    required: true\n"),
		},
		"templates/architectures/flat/files/main.go.tmpl": &fstest.MapFile{
			Data: []byte("package main\n"),
		},
	}

	plan, err := ComposeProject(memFS, "flat", []string{}, map[string]interface{}{
		"ServiceName": "test-svc",
	})
	require.NoError(t, err)
	assert.Equal(t, "flat", plan.Architecture)
	assert.Empty(t, plan.Capabilities)
	assert.Equal(t, "test-svc", plan.Vars["ServiceName"])
}

func TestSuggestions_NoCaps(t *testing.T) {
	memFS := fstest.MapFS{}
	suggestions := Suggestions(memFS, []string{})
	assert.Empty(t, suggestions)
}

func TestValidateConditionalRules_NoDuplicates(t *testing.T) {
	rules := map[string]ConditionalRule{
		"os_root": {
			Include: []string{"a.go"},
			Exclude: []string{"b.go"},
		},
	}
	warnings := ValidateConditionalRules(rules)
	assert.Empty(t, warnings)
}

func TestValidateConditionalRules_WithDuplicate(t *testing.T) {
	rules := map[string]ConditionalRule{
		"os_root": {
			Include: []string{"a.go", "shared.go"},
			Exclude: []string{"shared.go"},
		},
	}
	warnings := ValidateConditionalRules(rules)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "shared.go")
}
