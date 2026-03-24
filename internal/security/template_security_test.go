package security_test

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/provider"

	// Register providers so provider.Get works in tests.
	_ "github.com/diktahq/verikt/providers/golang"
	_ "github.com/diktahq/verikt/providers/typescript"
)

// denyPatterns are regex patterns that must never appear in template output.
// This test catches security regressions across all providers and templates.
var denyPatterns = []struct {
	pattern *regexp.Regexp
	reason  string
	// fileGlob limits which files this pattern applies to (empty = all .tmpl files).
	fileGlob string
}{
	{
		pattern: regexp.MustCompile(`change-me-in-production|change.me.in.prod`),
		reason:  "hardcoded fallback secret — service starts with forgeable tokens if env var unset",
	},
	{
		pattern: regexp.MustCompile(`image:\s*\S+:latest`),
		reason:  "unpinned Docker image tag — builds are not reproducible",
	},
	{
		pattern:  regexp.MustCompile(`AllowedOrigins:\s*\[\]string\{"\*"\}`),
		reason:   "CORS wildcard origin in Go templates",
		fileGlob: "*cors*",
	},
	{
		pattern:  regexp.MustCompile(`\?\?\s*\['\*'\]`),
		reason:   "CORS wildcard origin fallback in TypeScript templates",
		fileGlob: "*cors*",
	},
	{
		pattern:  regexp.MustCompile(`fmt\.Sprintf\([^)]*grant_type=`),
		reason:   "OAuth2 token exchange must use url.Values, not fmt.Sprintf — prevents parameter injection",
		fileGlob: "*oauth2*",
	},
	{
		pattern:  regexp.MustCompile(`reflection:\s*true`),
		reason:   "gRPC reflection enabled by default — exposes service schema in production",
		fileGlob: "*config*example*",
	},
}

func TestTemplates_NoInsecurePatterns(t *testing.T) {
	providers := []struct {
		name string
		p    provider.LanguageProvider
	}{
		{"golang", mustGetProvider(t, "go")},
		{"typescript", mustGetProvider(t, "typescript")},
	}

	for _, prov := range providers {
		t.Run(prov.name, func(t *testing.T) {
			tmplFS := prov.p.GetTemplateFS()
			err := fs.WalkDir(tmplFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil || d.IsDir() {
					return nil
				}
				if !strings.HasSuffix(path, ".tmpl") && !strings.HasSuffix(path, ".yml.tmpl") && !strings.HasSuffix(path, ".yaml.tmpl") {
					return nil
				}

				data, readErr := fs.ReadFile(tmplFS, path)
				if readErr != nil {
					t.Errorf("failed to read %s: %v", path, readErr)
					return nil
				}
				content := string(data)

				for _, dp := range denyPatterns {
					if dp.fileGlob != "" {
						matched, _ := filepath.Match(dp.fileGlob, filepath.Base(path))
						if !matched {
							continue
						}
					}
					if dp.pattern.MatchString(content) {
						t.Errorf("[SECURITY] %s/%s: %s\n  pattern: %s", prov.name, path, dp.reason, dp.pattern.String())
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walking %s templates: %v", prov.name, err)
			}
		})
	}
}

func mustGetProvider(t *testing.T, lang string) provider.LanguageProvider {
	t.Helper()
	p, err := provider.Get(lang)
	if err != nil {
		t.Fatalf("failed to get %s provider: %v", lang, err)
	}
	return p
}
