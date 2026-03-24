package golang

import (
	"context"
	"testing"

	"github.com/diktahq/verikt/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGoProviderImplementsVersionDetector(t *testing.T) {
	var p provider.LanguageProvider = &GoProvider{}
	vd, ok := p.(provider.VersionDetector)
	assert.True(t, ok, "GoProvider should implement VersionDetector")
	assert.NotNil(t, vd)
}

func TestGoProviderImplementsFeatureMatrixProvider(t *testing.T) {
	var p provider.LanguageProvider = &GoProvider{}
	fmp, ok := p.(provider.FeatureMatrixProvider)
	assert.True(t, ok, "GoProvider should implement FeatureMatrixProvider")
	assert.NotNil(t, fmp)
}

func TestNonImplementorDoesNotPanic(t *testing.T) {
	// A plain LanguageProvider that does NOT implement VersionDetector.
	var p provider.LanguageProvider = &GoProvider{}
	// Simulate checking a provider that might not implement the interface.
	_, ok := (interface{}(p)).(provider.VersionDetector)
	assert.True(t, ok, "GoProvider does implement it, but type assertion should not panic either way")

	// Use a nil interface value to verify no panic on failed assertion.
	var nilProvider provider.LanguageProvider
	_, ok = (interface{}(nilProvider)).(provider.VersionDetector)
	assert.False(t, ok)
}

func TestParseGoVersion(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "full version", raw: "go1.26.1", want: "1.26"},
		{name: "minor only", raw: "go1.22", want: "1.22"},
		{name: "empty string", raw: "", want: ""},
		{name: "no go prefix", raw: "1.21.3", want: "1.21"},
		{name: "rc version", raw: "go1.23rc1", want: "1.23rc1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGoVersion(tt.raw)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectVersion(t *testing.T) {
	p := &GoProvider{}
	version, err := p.DetectVersion(context.Background())
	require.NoError(t, err)
	// In CI/dev, go should be available.
	assert.NotEmpty(t, version, "expected a Go version to be detected")
	assert.Contains(t, version, ".", "version should contain major.minor separator")
}

func TestGetFeatureMatrix(t *testing.T) {
	p := &GoProvider{}
	data, err := p.GetFeatureMatrix()
	require.NoError(t, err)
	require.NotNil(t, data)

	// Verify it's valid YAML.
	var parsed map[string]interface{}
	err = yaml.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Contains(t, parsed, "features")
}
