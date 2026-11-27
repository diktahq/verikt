package scaffold

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addedCapsFS is a minimal provider template tree: a clean architecture that
// maps the hexagonal adapter/httphandler and service directories, http-api
// (already installed in the tests below) and health, which writes into both.
func addedCapsFS() fstest.MapFS {
	return fstest.MapFS{
		"templates/architectures/clean/manifest.yaml": &fstest.MapFile{Data: []byte(
			"name: clean\npath_mappings:\n  adapter/httphandler: internal/interface/handler\n  service: internal/usecase\n")},
		"templates/architectures/flat/manifest.yaml": &fstest.MapFile{Data: []byte(
			"name: flat\npath_mappings:\n  adapter/httphandler: handler\n  service: \"\"\n")},
		"templates/capabilities/http-api/capability.yaml": &fstest.MapFile{Data: []byte("name: http-api\n")},
		"templates/capabilities/http-api/files/adapter/httphandler/router.go.tmpl": &fstest.MapFile{
			Data: []byte("package {{.ArchPaths.HTTPHandlerPkg}}\n")},
		"templates/capabilities/health/capability.yaml": &fstest.MapFile{Data: []byte("name: health\nrequires: [http-api]\n")},
		"templates/capabilities/health/files/adapter/httphandler/health.go.tmpl": &fstest.MapFile{
			Data: []byte("package {{.ArchPaths.HTTPHandlerPkg}}\n")},
		"templates/capabilities/health/files/service/probe.go.tmpl": &fstest.MapFile{
			Data: []byte("package {{.ArchPaths.ServicePkg}}\n")},
	}
}

func composeAdded(t *testing.T, tfs fstest.MapFS, arch string) *CompositionPlan {
	t.Helper()
	plan, err := ComposeProject(tfs, arch, []string{"http-api", "health"}, map[string]interface{}{})
	require.NoError(t, err)
	return plan
}

func TestRenderAddedCapabilities_WritesToMappedPathsWithArchPaths(t *testing.T) {
	tfs := addedCapsFS()
	out := t.TempDir()

	result, skipped, err := NewRenderer(tfs).RenderAddedCapabilities(composeAdded(t, tfs, "clean"), out, []string{"health"})
	require.NoError(t, err)
	assert.Empty(t, skipped)
	assert.ElementsMatch(t, []string{
		filepath.Join(out, "internal", "interface", "handler", "health.go"),
		filepath.Join(out, "internal", "usecase", "probe.go"),
	}, result.FilesCreated)

	health, err := os.ReadFile(filepath.Join(out, "internal", "interface", "handler", "health.go"))
	require.NoError(t, err)
	assert.Equal(t, "package handler\n", string(health), "ArchPaths must reach templates rendered by add")
}

func TestRenderAddedCapabilities_RendersOnlyNamedCapabilities(t *testing.T) {
	tfs := addedCapsFS()
	out := t.TempDir()

	_, _, err := NewRenderer(tfs).RenderAddedCapabilities(composeAdded(t, tfs, "clean"), out, []string{"health"})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(out, "internal", "interface", "handler", "router.go"))
}

func TestRenderAddedCapabilities_CreatesNoUnmappedDirectories(t *testing.T) {
	tfs := addedCapsFS()
	out := t.TempDir()

	_, _, err := NewRenderer(tfs).RenderAddedCapabilities(composeAdded(t, tfs, "flat"), out, []string{"health"})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(out, "handler", "health.go"))
	assert.FileExists(t, filepath.Join(out, "probe.go"), "flat maps service/ to the project root")
	assert.NoDirExists(t, filepath.Join(out, "adapter"))
	assert.NoDirExists(t, filepath.Join(out, "service"))
}

func TestRenderAddedCapabilities_SkipsExistingFiles(t *testing.T) {
	tfs := addedCapsFS()
	out := t.TempDir()
	existing := filepath.Join(out, "internal", "interface", "handler", "health.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(existing), 0o755))
	require.NoError(t, os.WriteFile(existing, []byte("keep\n"), 0o644))

	result, skipped, err := NewRenderer(tfs).RenderAddedCapabilities(composeAdded(t, tfs, "clean"), out, []string{"health"})
	require.NoError(t, err)
	assert.Equal(t, []string{existing}, skipped)
	assert.Equal(t, []string{filepath.Join(out, "internal", "usecase", "probe.go")}, result.FilesCreated)

	got, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.Equal(t, "keep\n", string(got))
}

func TestRenderAddedCapabilities_RejectsCapabilityOutsidePlan(t *testing.T) {
	tfs := addedCapsFS()
	out := t.TempDir()

	_, _, err := NewRenderer(tfs).RenderAddedCapabilities(composeAdded(t, tfs, "clean"), out, []string{"redis"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"redis"`)

	entries, err := os.ReadDir(out)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// The dry-run preview must agree with the real render, mapped paths included,
// and write nothing.
func TestPreviewAddedCapabilities_MatchesRenderAndWritesNothing(t *testing.T) {
	tfs := addedCapsFS()
	out := t.TempDir()
	existing := filepath.Join(out, "internal", "usecase", "probe.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(existing), 0o755))
	require.NoError(t, os.WriteFile(existing, []byte("keep\n"), 0o644))

	renderer := NewRenderer(tfs)
	created, skipped, err := renderer.PreviewAddedCapabilities(composeAdded(t, tfs, "clean"), out, []string{"health"})
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(out, "internal", "interface", "handler", "health.go")}, created)
	assert.Equal(t, []string{existing}, skipped)
	assert.NoDirExists(t, filepath.Join(out, "internal", "interface"), "dry run must not create directories")

	result, renderSkipped, err := renderer.RenderAddedCapabilities(composeAdded(t, tfs, "clean"), out, []string{"health"})
	require.NoError(t, err)
	assert.ElementsMatch(t, created, result.FilesCreated, "preview must match render")
	assert.ElementsMatch(t, skipped, renderSkipped, "preview must match render")
}
