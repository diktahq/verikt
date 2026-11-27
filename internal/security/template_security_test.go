package security_test

import (
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/checker/scriptguard"
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
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
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

// TestTemplates_NoMutableInstallURLs runs the line-based verikt check guards
// (mutable_install_url and silent_ci_swallow) over every Dockerfile, CI
// config, Makefile and script template, so verikt never scaffolds a project
// that fails its own check (INV-003: no unpinned dependencies). The guards
// ship at warning severity in verikt check; here any finding fails.
func TestTemplates_NoMutableInstallURLs(t *testing.T) {
	for _, lang := range []string{"go", "typescript"} {
		t.Run(lang, func(t *testing.T) {
			tmplFS := mustGetProvider(t, lang).GetTemplateFS()
			scanned := 0
			err := fs.WalkDir(tmplFS, ".", func(p string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil || d.IsDir() || !strings.HasSuffix(p, ".tmpl") {
					return walkErr
				}
				kind, ok := templateScriptKind(p)
				if !ok {
					return nil
				}
				data, err := fs.ReadFile(tmplFS, p)
				if err != nil {
					return err
				}
				scanned++
				for _, v := range scriptguard.Scan(p, kind, data, scriptguard.Options{}) {
					t.Errorf("[SECURITY] %s:%d [%s] %s", p, v.Line, v.Rule, v.Message)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walking %s templates: %v", lang, err)
			}
			if scanned == 0 {
				t.Fatalf("no %s Dockerfile/CI/Makefile templates scanned; template layout changed?", lang)
			}
		})
	}
}

// templateScriptKind classifies a template by the path it renders to: the
// part after ".../files/", without ".tmpl". GitLab and Bitbucket CI configs,
// which the project walk does not read, are scanned as generic CI config.
func templateScriptKind(tmplPath string) (scriptguard.Kind, bool) {
	out := strings.TrimSuffix(tmplPath, ".tmpl")
	if _, after, ok := strings.Cut(out, "/files/"); ok {
		out = after
	}
	switch path.Base(out) {
	case ".gitlab-ci.yml", "bitbucket-pipelines.yml":
		return scriptguard.KindCIConfig, true
	}
	return scriptguard.Classify(out, false)
}

func mustGetProvider(t *testing.T, lang string) provider.LanguageProvider {
	t.Helper()
	p, err := provider.Get(lang)
	if err != nil {
		t.Fatalf("failed to get %s provider: %v", lang, err)
	}
	return p
}
