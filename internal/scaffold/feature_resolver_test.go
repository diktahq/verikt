package scaffold

import (
	"context"
	"io/fs"
	"testing"

	"github.com/diktahq/verikt/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullProvider implements LanguageProvider, VersionDetector, and FeatureMatrixProvider.
type fullProvider struct {
	version    string
	versionErr error
	matrix     []byte
	matrixErr  error
}

func (p *fullProvider) Scaffold(_ context.Context, _ provider.ScaffoldRequest) (*provider.ScaffoldResponse, error) {
	return nil, provider.ErrNotImplemented
}
func (p *fullProvider) Analyze(_ context.Context, _ provider.AnalyzeRequest) (*provider.AnalyzeResponse, error) {
	return nil, provider.ErrNotImplemented
}
func (p *fullProvider) Migrate(_ context.Context, _ provider.MigrateRequest) (*provider.MigrateResponse, error) {
	return nil, provider.ErrNotImplemented
}
func (p *fullProvider) GetInfo(_ context.Context) (*provider.ProviderInfo, error) {
	return &provider.ProviderInfo{Name: "test"}, nil
}
func (p *fullProvider) GetTemplateFS() fs.FS { return nil }
func (p *fullProvider) DetectVersion(_ context.Context) (string, error) {
	return p.version, p.versionErr
}
func (p *fullProvider) GetFeatureMatrix() ([]byte, error) {
	return p.matrix, p.matrixErr
}

// bareProvider implements only LanguageProvider — no optional interfaces.
type bareProvider struct{}

func (p *bareProvider) Scaffold(_ context.Context, _ provider.ScaffoldRequest) (*provider.ScaffoldResponse, error) {
	return nil, provider.ErrNotImplemented
}
func (p *bareProvider) Analyze(_ context.Context, _ provider.AnalyzeRequest) (*provider.AnalyzeResponse, error) {
	return nil, provider.ErrNotImplemented
}
func (p *bareProvider) Migrate(_ context.Context, _ provider.MigrateRequest) (*provider.MigrateResponse, error) {
	return nil, provider.ErrNotImplemented
}
func (p *bareProvider) GetInfo(_ context.Context) (*provider.ProviderInfo, error) {
	return &provider.ProviderInfo{Name: "bare"}, nil
}
func (p *bareProvider) GetTemplateFS() fs.FS { return nil }

var testMatrix = []byte(`
features:
  - name: generics
    since: "1.18"
  - name: range_over_func
    since: "1.23"
  - name: modules
    since: "1.11"
`)

func TestFeatureResolver_Resolve_FullProvider(t *testing.T) {
	r := &FeatureResolver{}
	p := &fullProvider{version: "1.21", matrix: testMatrix}

	features, err := r.Resolve(context.Background(), p)
	require.NoError(t, err)
	assert.True(t, features["generics"], "go 1.21 supports generics (min 1.18)")
	assert.False(t, features["range_over_func"], "go 1.21 does not support range_over_func (min 1.23)")
	assert.True(t, features["modules"], "go 1.21 supports modules (min 1.11)")
}

func TestFeatureResolver_Resolve_BareProvider(t *testing.T) {
	r := &FeatureResolver{}
	p := &bareProvider{}

	features, err := r.Resolve(context.Background(), p)
	require.NoError(t, err)
	assert.Empty(t, features)
}

func TestFeatureResolver_Resolve_EmptyVersion(t *testing.T) {
	r := &FeatureResolver{}
	p := &fullProvider{version: "", matrix: testMatrix}

	features, err := r.Resolve(context.Background(), p)
	require.NoError(t, err)
	assert.Empty(t, features)
}

func TestFeatureResolver_Resolve_NilMatrix(t *testing.T) {
	r := &FeatureResolver{}
	p := &fullProvider{version: "1.21", matrix: nil}

	features, err := r.Resolve(context.Background(), p)
	require.NoError(t, err)
	assert.Empty(t, features)
}

func TestFeatureResolver_ResolveWithVersion(t *testing.T) {
	r := &FeatureResolver{}

	features, err := r.ResolveWithVersion("1.23", testMatrix)
	require.NoError(t, err)
	assert.True(t, features["generics"])
	assert.True(t, features["range_over_func"])
	assert.True(t, features["modules"])
}

func TestFeatureResolver_ResolveWithVersion_NilMatrix(t *testing.T) {
	r := &FeatureResolver{}

	features, err := r.ResolveWithVersion("1.21", nil)
	require.NoError(t, err)
	assert.Empty(t, features)
}

func TestFeatureResolver_ResolveWithVersion_OlderVersion(t *testing.T) {
	r := &FeatureResolver{}

	features, err := r.ResolveWithVersion("1.10", testMatrix)
	require.NoError(t, err)
	assert.False(t, features["generics"])
	assert.False(t, features["range_over_func"])
	assert.False(t, features["modules"], "1.10 < 1.11")
}
