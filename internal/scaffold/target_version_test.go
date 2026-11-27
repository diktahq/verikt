package scaffold

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goMatrixYAML mirrors a slice of providers/golang/templates/features.yaml.
var goMatrixYAML = []byte(`features:
  - name: slices_package
    since: "1.21"
  - name: range_over_int
    since: "1.22"
  - name: range_over_func
    since: "1.23"
  - name: os_root
    since: "1.24"
  - name: synctest
    since: "1.25"
`)

func TestResolveTarget_FeaturesPerVersion(t *testing.T) {
	cases := []struct {
		version string
		want    map[string]bool
	}{
		{"1.21", map[string]bool{"slices_package": true, "range_over_int": false, "range_over_func": false, "os_root": false, "synctest": false}},
		{"1.22", map[string]bool{"slices_package": true, "range_over_int": true, "range_over_func": false, "os_root": false, "synctest": false}},
		{"1.23", map[string]bool{"slices_package": true, "range_over_int": true, "range_over_func": true, "os_root": false, "synctest": false}},
		{"1.24.3", map[string]bool{"slices_package": true, "range_over_int": true, "range_over_func": true, "os_root": true, "synctest": false}},
		{"1.26", map[string]bool{"slices_package": true, "range_over_int": true, "range_over_func": true, "os_root": true, "synctest": true}},
		{"1.23rc1", map[string]bool{"slices_package": true, "range_over_int": true, "range_over_func": true, "os_root": false, "synctest": false}},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			p := &fullProvider{matrix: goMatrixYAML}
			target, err := (&FeatureResolver{}).ResolveTarget(context.Background(), p,
				VersionCandidate{Version: tc.version, Source: VersionExplicit}, "1.22")
			require.NoError(t, err)
			assert.Equal(t, tc.version, target.Version)
			assert.Equal(t, VersionExplicit, target.Source)
			assert.Equal(t, tc.want, target.Features)
		})
	}
}

func TestResolveTarget_PrecedenceAndFallback(t *testing.T) {
	ctx := context.Background()
	r := &FeatureResolver{}

	t.Run("pinned version wins over detection", func(t *testing.T) {
		p := &fullProvider{version: "1.26", matrix: goMatrixYAML}
		target, err := r.ResolveTarget(ctx, p, VersionCandidate{Version: "1.22.4", Source: VersionProject}, "1.21")
		require.NoError(t, err)
		assert.Equal(t, "1.22.4", target.Version)
		assert.Equal(t, VersionProject, target.Source)
		assert.False(t, target.Features["range_over_func"])
	})

	t.Run("detected toolchain used when nothing is pinned", func(t *testing.T) {
		p := &fullProvider{version: "1.24", matrix: goMatrixYAML}
		target, err := r.ResolveTarget(ctx, p, VersionCandidate{}, "1.22")
		require.NoError(t, err)
		assert.Equal(t, "1.24", target.Version)
		assert.Equal(t, VersionDetected, target.Source)
		assert.True(t, target.Features["os_root"])
	})

	t.Run("toolchain not installed falls back to the default", func(t *testing.T) {
		p := &fullProvider{version: "", matrix: goMatrixYAML}
		target, err := r.ResolveTarget(ctx, p, VersionCandidate{}, "1.22")
		require.NoError(t, err)
		assert.Equal(t, "1.22", target.Version)
		assert.Equal(t, VersionDefault, target.Source)
		assert.True(t, target.Features["range_over_int"])
		assert.False(t, target.Features["range_over_func"])
	})

	t.Run("detection error falls back to the default", func(t *testing.T) {
		p := &fullProvider{versionErr: errors.New("go env failed"), matrix: goMatrixYAML}
		target, err := r.ResolveTarget(ctx, p, VersionCandidate{}, "1.22")
		require.NoError(t, err)
		assert.Equal(t, "1.22", target.Version)
		assert.Equal(t, VersionDefault, target.Source)
	})

	t.Run("unparseable detected version falls back to the default", func(t *testing.T) {
		p := &fullProvider{version: "devel", matrix: goMatrixYAML}
		target, err := r.ResolveTarget(ctx, p, VersionCandidate{}, "1.22")
		require.NoError(t, err)
		assert.Equal(t, VersionDefault, target.Source)
	})

	t.Run("provider without detector uses the default", func(t *testing.T) {
		target, err := r.ResolveTarget(ctx, &bareProvider{}, VersionCandidate{}, "1.22")
		require.NoError(t, err)
		assert.Equal(t, "1.22", target.Version)
		assert.Empty(t, target.Features, "no matrix means no features")
	})

	t.Run("no version anywhere yields empty features", func(t *testing.T) {
		p := &fullProvider{matrix: goMatrixYAML}
		target, err := r.ResolveTarget(ctx, p, VersionCandidate{}, "")
		require.NoError(t, err)
		assert.Empty(t, target.Version)
		assert.NotNil(t, target.Features)
		assert.Empty(t, target.Features)
	})

	t.Run("invalid pinned version is rejected", func(t *testing.T) {
		p := &fullProvider{version: "1.26", matrix: goMatrixYAML}
		_, err := r.ResolveTarget(ctx, p, VersionCandidate{Version: "latest", Source: VersionExplicit}, "1.22")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"latest"`)
	})
}
