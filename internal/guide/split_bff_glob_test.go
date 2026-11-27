package guide

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/scaffold"
	"github.com/stretchr/testify/require"
)

// TestHTTPGlobsCoverEveryArchitecturesBFFPath reads each Go architecture
// manifest and checks that a file under its mapped bff gateway directory is
// matched by the http category's frontmatter globs. Before issue #32 only the
// hexagonal path was listed, so BFF guidance never loaded in clean or flat.
func TestHTTPGlobsCoverEveryArchitecturesBFFPath(t *testing.T) {
	manifests, err := filepath.Glob(filepath.Join("..", "..", "providers", "golang", "templates", "architectures", "*", "manifest.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, manifests)

	for _, mf := range manifests {
		arch := filepath.Base(filepath.Dir(mf))
		data, err := os.ReadFile(mf)
		require.NoError(t, err)
		m, err := scaffold.ParseManifest(data)
		require.NoError(t, err)

		dir, ok := m.PathMappings["adapter/bffgateway"]
		if !ok {
			continue
		}
		file := dir + "/gateway.go"
		if !anyGlobMatches(globsForCategory("http"), file) {
			t.Errorf("%s maps the bff gateway to %s, but no http glob matches %s", arch, dir, file)
		}
	}
}

// anyGlobMatches supports the two glob shapes categoryGlobs uses: a literal
// directory prefix followed by "/**", and a single-segment "*" wildcard.
func anyGlobMatches(globs []string, file string) bool {
	for _, g := range globs {
		prefix, ok := strings.CutSuffix(g, "/**")
		if !ok {
			continue
		}
		dir := filepath.Dir(file)
		for d := dir; d != "." && d != "/"; d = filepath.Dir(d) {
			if match, _ := filepath.Match(prefix, d); match {
				return true
			}
		}
	}
	return false
}

// TestFrontendGlobsCoverEveryArchitecturesStaticAssets: static-assets writes
// its files to static/ next to the HTTP handler (go:embed cannot reach a
// parent directory, #28), so the frontend globs must match that directory in
// every architecture, not a static/ at the project root.
func TestFrontendGlobsCoverEveryArchitecturesStaticAssets(t *testing.T) {
	manifests, err := filepath.Glob(filepath.Join("..", "..", "providers", "golang", "templates", "architectures", "*", "manifest.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, manifests)

	for _, mf := range manifests {
		arch := filepath.Base(filepath.Dir(mf))
		data, err := os.ReadFile(mf)
		require.NoError(t, err)
		m, err := scaffold.ParseManifest(data)
		require.NoError(t, err)

		dir, ok := m.PathMappings["adapter/httphandler"]
		if !ok {
			continue
		}
		file := dir + "/static/css/style.css"
		if !anyGlobMatches(globsForCategory("frontend"), file) {
			t.Errorf("%s puts static assets under %s/static, but no frontend glob matches %s", arch, dir, file)
		}
	}
}

// TestCapabilityDirForStaticAssetsIsWhereTheTemplateWrites: the guide names
// the directory the static-assets template actually writes.
func TestCapabilityDirForStaticAssetsIsWhereTheTemplateWrites(t *testing.T) {
	dir := capabilityDir("static-assets")
	path := filepath.Join("..", "..", "providers", "golang", "templates", "capabilities", "static-assets", "files", filepath.FromSlash(dir))
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Errorf("capabilityDir(static-assets) = %q, but the template writes no directory there", dir)
	}
}
