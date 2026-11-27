package scaffold

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requiresFeaturesFS is a template tree with a feature matrix and one
// capability that needs range-over-func iterators (Go 1.23+).
func requiresFeaturesFS() fs.FS {
	tfs := testCapabilityFS().(fstest.MapFS)
	tfs["templates/features.yaml"] = &fstest.MapFile{Data: goMatrixYAML}
	tfs["templates/capabilities/iter/capability.yaml"] = &fstest.MapFile{Data: []byte(`name: iter
description: "Needs iterators"
requires_features:
  - range_over_func
`)}
	tfs["templates/capabilities/iter/files/iter.go.tmpl"] = &fstest.MapFile{Data: []byte("package iter\n")}
	return tfs
}

func featuresFor(t *testing.T, version string) map[string]bool {
	t.Helper()
	m, err := ParseFeatureMatrix(goMatrixYAML)
	require.NoError(t, err)
	f, err := ResolveFeatures(version, m)
	require.NoError(t, err)
	return f
}

func baseVars(features map[string]bool) map[string]interface{} {
	vars := map[string]interface{}{"ServiceName": "svc", "ModulePath": "example.com/svc"}
	if features != nil {
		vars["Features"] = features
	}
	return vars
}

func TestComposeProject_RequiresFeatures_VersionHasFeature(t *testing.T) {
	plan, err := ComposeProject(requiresFeaturesFS(), "hexagonal", []string{"iter"}, baseVars(featuresFor(t, "1.23")))
	require.NoError(t, err)
	assert.Contains(t, plan.Capabilities, "iter")
}

func TestComposeProject_RequiresFeatures_VersionLacksFeature(t *testing.T) {
	_, err := ComposeProject(requiresFeaturesFS(), "hexagonal", []string{"iter"}, baseVars(featuresFor(t, "1.22")))
	require.Error(t, err)

	var missing *MissingFeaturesError
	require.True(t, errors.As(err, &missing), "want *MissingFeaturesError, got %T: %v", err, err)
	require.Len(t, missing.Missing, 1)
	assert.Equal(t, MissingFeature{Requirer: `capability "iter"`, Feature: "range_over_func", Since: "1.23"}, missing.Missing[0])
	assert.Equal(t, "1.23", missing.MinimumVersion())

	msg := err.Error()
	assert.Contains(t, msg, `capability "iter"`)
	assert.Contains(t, msg, `"range_over_func"`)
	assert.Contains(t, msg, "1.23")
}

func TestComposeProject_RequiresFeatures_NoFeaturesResolved(t *testing.T) {
	// No version could be resolved, so nothing can be shown to satisfy the
	// requirement: fail closed rather than render code that may not compile.
	_, err := ComposeProject(requiresFeaturesFS(), "hexagonal", []string{"iter"}, baseVars(nil))
	var missing *MissingFeaturesError
	require.True(t, errors.As(err, &missing), "want *MissingFeaturesError, got %T: %v", err, err)
}

func TestComposeProject_RequiresFeatures_UnknownFeature(t *testing.T) {
	tfs := requiresFeaturesFS().(fstest.MapFS)
	tfs["templates/capabilities/iter/capability.yaml"] = &fstest.MapFile{Data: []byte(`name: iter
description: "Needs something the matrix does not define"
requires_features: [time_travel]
`)}
	_, err := ComposeProject(tfs, "hexagonal", []string{"iter"}, baseVars(featuresFor(t, "1.26")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"time_travel"`)
	assert.Contains(t, err.Error(), "feature matrix")
}

func TestComposeProject_RequiresFeatures_Architecture(t *testing.T) {
	tfs := requiresFeaturesFS().(fstest.MapFS)
	tfs["templates/architectures/hexagonal/manifest.yaml"] = &fstest.MapFile{Data: []byte(`name: hexagonal
description: "Hexagonal architecture"
requires_features: [os_root]
variables:
  - name: ServiceName
    type: string
    required: true
  - name: ModulePath
    type: string
    required: true
`)}
	_, err := ComposeProject(tfs, "hexagonal", []string{"http-api"}, baseVars(featuresFor(t, "1.23")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `architecture "hexagonal"`)
	assert.Contains(t, err.Error(), "1.24")

	_, err = ComposeProject(tfs, "hexagonal", []string{"http-api"}, baseVars(featuresFor(t, "1.24")))
	require.NoError(t, err)
}

func TestComposeProject_NoRequiredFeatures_IgnoresMissingFeatures(t *testing.T) {
	// Capabilities without requires_features compose whether or not features
	// were resolved.
	_, err := ComposeProject(requiresFeaturesFS(), "hexagonal", []string{"http-api"}, baseVars(nil))
	require.NoError(t, err)
}
