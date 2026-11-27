package golang

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/provider"
	"github.com/diktahq/verikt/internal/scaffold"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withFakeGo puts a `go` executable on PATH that answers `go env GOVERSION`
// with goversion. An empty goversion leaves PATH with no `go` at all, which
// is how a machine without a Go toolchain looks to the provider.
func withFakeGo(t *testing.T, goversion string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake go toolchain is a shell script")
	}
	dir := t.TempDir()
	if goversion != "" {
		script := "#!/bin/sh\necho " + goversion + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0o755))
	}
	t.Setenv("PATH", dir)
}

func scaffoldWith(t *testing.T, options map[string]string) (string, error) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "svc")
	opts := map[string]string{"skip_hooks": "true", "capabilities": "http-api"}
	for k, v := range options {
		opts[k] = v
	}
	_, err := (&GoProvider{}).Scaffold(context.Background(), provider.ScaffoldRequest{
		ProjectName:  "svc",
		ModulePath:   "example.com/svc",
		TemplateName: "hexagonal",
		OutputDir:    out,
		Options:      opts,
	})
	return out, err
}

func readGenerated(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return string(data)
}

func TestScaffold_GoVersionFromDetectedToolchain(t *testing.T) {
	withFakeGo(t, "go1.26.3")
	out, err := scaffoldWith(t, nil)
	require.NoError(t, err)
	// major.minor only: go 1.26 is satisfied by every 1.26.x patch release, so
	// collaborators on another patch do not trigger a toolchain switch.
	assert.Contains(t, readGenerated(t, out, "go.mod"), "\ngo 1.26\n")
	assert.Contains(t, readGenerated(t, out, "Dockerfile"), "golang:1.26-alpine")
}

func TestScaffold_GoVersionFallsBackWithoutToolchain(t *testing.T) {
	withFakeGo(t, "")
	out, err := scaffoldWith(t, nil)
	require.NoError(t, err)
	assert.Contains(t, readGenerated(t, out, "go.mod"), "\ngo 1.22\n", "architecture manifest default")
}

func TestScaffold_ExplicitGoVersionWinsOverDetection(t *testing.T) {
	withFakeGo(t, "go1.26.3")
	out, err := scaffoldWith(t, map[string]string{"GoVersion": "1.24"})
	require.NoError(t, err)
	assert.Contains(t, readGenerated(t, out, "go.mod"), "\ngo 1.24\n")
}

func TestScaffold_RejectsInvalidGoVersion(t *testing.T) {
	withFakeGo(t, "go1.26.3")
	_, err := scaffoldWith(t, map[string]string{"GoVersion": "latest"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GoVersion")
}

func TestScaffold_LegacyModeUsesDetectedVersion(t *testing.T) {
	withFakeGo(t, "go1.25.1")
	out, err := scaffoldWith(t, map[string]string{"capabilities": ""})
	require.NoError(t, err)
	assert.Contains(t, readGenerated(t, out, "go.mod"), "\ngo 1.25\n")
}

func TestGoTarget_ResolvesFeaturesFromVersion(t *testing.T) {
	cases := []struct {
		name      string
		goversion string
		options   map[string]string
		version   string
		source    scaffold.VersionSource
		has       []string
		lacks     []string
	}{
		{"detected 1.26", "go1.26.3", nil, "1.26", scaffold.VersionDetected, []string{"os_root", "synctest", "range_over_func"}, nil},
		{"explicit 1.23", "go1.26.3", map[string]string{"GoVersion": "1.23"}, "1.23", scaffold.VersionExplicit, []string{"range_over_func"}, []string{"os_root"}},
		{"no toolchain", "", nil, "1.22", scaffold.VersionDefault, []string{"range_over_int", "slices_package"}, []string{"range_over_func", "os_root"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFakeGo(t, tc.goversion)
			target, err := (&GoProvider{}).goTarget(context.Background(), tc.options, "hexagonal")
			require.NoError(t, err)
			assert.Equal(t, tc.version, target.Version)
			assert.Equal(t, tc.source, target.Source)
			for _, f := range tc.has {
				assert.True(t, target.Features[f], "%s should be enabled", f)
			}
			for _, f := range tc.lacks {
				assert.False(t, target.Features[f], "%s should be disabled", f)
			}
		})
	}
}

func TestDetectVersion_FakeToolchain(t *testing.T) {
	withFakeGo(t, "go1.26.3")
	v, err := (&GoProvider{}).DetectVersion(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "1.26", v)
}

func TestDetectVersion_NoToolchain(t *testing.T) {
	withFakeGo(t, "")
	v, err := (&GoProvider{}).DetectVersion(context.Background())
	require.NoError(t, err)
	assert.Empty(t, v)
}

func TestParseGoVersion_DevelBuildIsUndetected(t *testing.T) {
	assert.Empty(t, parseGoVersion("devel go1.27-abcdef Tue Nov 3 2026"))
}

func TestExplainMissingFeatures(t *testing.T) {
	missing := &scaffold.MissingFeaturesError{Missing: []scaffold.MissingFeature{
		{Requirer: `capability "iter"`, Feature: "range_over_func", Since: "1.23"},
	}}
	target := &scaffold.Target{Version: "1.22", Source: scaffold.VersionDefault}

	msg := explainMissingFeatures(missing, target).Error()
	assert.Contains(t, msg, `capability "iter" requires feature "range_over_func", available from version 1.23`)
	assert.Contains(t, msg, "target Go version is 1.22")
	assert.Contains(t, msg, "no Go toolchain was detected")
	assert.Contains(t, msg, "--set GoVersion=1.23")

	var asMissing *scaffold.MissingFeaturesError
	assert.ErrorAs(t, explainMissingFeatures(missing, target), &asMissing, "the typed error stays in the chain")
}

func TestFeatureMatrixCitesADR007(t *testing.T) {
	data, err := (&GoProvider{}).GetFeatureMatrix()
	require.NoError(t, err)
	header := strings.SplitN(string(data), "features:", 2)[0]
	assert.Contains(t, header, "ADR-007")
	assert.NotContains(t, header, "ADR-008")
}
