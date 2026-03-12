package scaffold

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeatureAvailableInTemplate(t *testing.T) {
	tmpl := `{{if .Features.os_root}}ROOT{{else}}FALLBACK{{end}}`
	vars := map[string]interface{}{
		"Features": map[string]bool{"os_root": true},
	}

	out, err := executeTemplate(tmpl, vars)
	require.NoError(t, err)
	assert.Contains(t, string(out), "ROOT")
}

func TestFeatureUnavailableInTemplate(t *testing.T) {
	tmpl := `{{if .Features.os_root}}ROOT{{else}}FALLBACK{{end}}`
	vars := map[string]interface{}{
		"Features": map[string]bool{"os_root": false},
	}

	out, err := executeTemplate(tmpl, vars)
	require.NoError(t, err)
	assert.Contains(t, string(out), "FALLBACK")
}

func TestConditionalFileInclusion_Integration(t *testing.T) {
	conditionals := map[string]ConditionalRule{
		"os_root": {
			Include: []string{"internal/safepath/root.go.tmpl"},
			Exclude: []string{"internal/safepath/fallback.go.tmpl"},
		},
	}

	t.Run("feature enabled includes root excludes fallback", func(t *testing.T) {
		features := map[string]bool{"os_root": true}
		assert.True(t, shouldIncludeFile("internal/safepath/root.go.tmpl", conditionals, features))
		assert.False(t, shouldIncludeFile("internal/safepath/fallback.go.tmpl", conditionals, features))
	})

	t.Run("feature disabled excludes root includes fallback", func(t *testing.T) {
		features := map[string]bool{"os_root": false}
		assert.False(t, shouldIncludeFile("internal/safepath/root.go.tmpl", conditionals, features))
		assert.True(t, shouldIncludeFile("internal/safepath/fallback.go.tmpl", conditionals, features))
	})

	t.Run("unmatched file included by default", func(t *testing.T) {
		features := map[string]bool{"os_root": false}
		assert.True(t, shouldIncludeFile("main.go.tmpl", conditionals, features))
	})
}

func TestRequiredFeaturesBlockScaffold_Integration(t *testing.T) {
	matrix := &FeatureMatrix{
		Features: []Feature{
			{Name: "os_root", Since: "1.24", Description: "os.OpenRoot"},
		},
	}

	t.Run("missing required feature returns error", func(t *testing.T) {
		resolved := map[string]bool{"os_root": false}
		err := CheckRequiredFeatures([]string{"os_root"}, resolved, matrix, "1.22")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "os_root")
	})

	t.Run("satisfied required feature returns nil", func(t *testing.T) {
		resolved := map[string]bool{"os_root": true}
		err := CheckRequiredFeatures([]string{"os_root"}, resolved, matrix, "1.24")
		require.NoError(t, err)
	})
}

func TestFeaturesBackwardCompatible(t *testing.T) {
	tmpl := `Hello {{.ServiceName}}`
	vars := map[string]interface{}{
		"ServiceName": "orders",
	}

	out, err := executeTemplate(tmpl, vars)
	require.NoError(t, err)
	assert.Equal(t, "Hello orders", string(out))
}

func TestFeaturesBackwardCompatibleWithGuard(t *testing.T) {
	tmpl := `{{with .Features}}{{if .os_root}}ROOT{{else}}FALLBACK{{end}}{{else}}FALLBACK{{end}}`
	vars := map[string]interface{}{
		"ServiceName": "orders",
	}

	out, err := executeTemplate(tmpl, vars)
	require.NoError(t, err)
	assert.Contains(t, string(out), "FALLBACK")
}

func TestExplicitVersionOverride_Integration(t *testing.T) {
	matrix := &FeatureMatrix{
		Features: []Feature{
			{Name: "slices_package", Since: "1.21", Description: "slices pkg"},
			{Name: "range_over_int", Since: "1.22", Description: "range int"},
			{Name: "range_over_func", Since: "1.23", Description: "range func"},
			{Name: "os_root", Since: "1.24", Description: "os.OpenRoot"},
		},
	}

	features, err := ResolveFeatures("1.26", matrix)
	require.NoError(t, err)

	for _, name := range []string{"slices_package", "range_over_int", "range_over_func", "os_root"} {
		assert.True(t, features[name], "expected %s to be true for Go 1.26", name)
	}
}

func TestMidVersionResolution_Integration(t *testing.T) {
	matrix := &FeatureMatrix{
		Features: []Feature{
			{Name: "slices_package", Since: "1.21", Description: "slices pkg"},
			{Name: "range_over_int", Since: "1.22", Description: "range int"},
			{Name: "range_over_func", Since: "1.23", Description: "range func"},
			{Name: "os_root", Since: "1.24", Description: "os.OpenRoot"},
		},
	}

	features, err := ResolveFeatures("1.23", matrix)
	require.NoError(t, err)

	assert.True(t, features["slices_package"])
	assert.True(t, features["range_over_int"])
	assert.True(t, features["range_over_func"])
	assert.False(t, features["os_root"], "1.23 < 1.24")
}

func TestFullPipeline_FeaturesToTemplate(t *testing.T) {
	matrix := &FeatureMatrix{
		Features: []Feature{
			{Name: "os_root", Since: "1.24", Description: "os.OpenRoot"},
			{Name: "log_slog", Since: "1.21", Description: "slog"},
		},
	}

	features, err := ResolveFeatures("1.22", matrix)
	require.NoError(t, err)

	vars := map[string]interface{}{
		"Features": features,
	}

	tmpl := `{{if .Features.os_root}}openroot{{end}}{{if .Features.log_slog}}slog{{end}}`
	out, err := executeTemplate(tmpl, vars)
	require.NoError(t, err)
	result := strings.TrimSpace(string(out))
	assert.Equal(t, "slog", result, "1.22 should have log_slog but not os_root")
}
