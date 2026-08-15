package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// downloadURL matches a release asset URL in the documentation, capturing the
// tag from the path and the file name.
var downloadURL = regexp.MustCompile(`releases/download/v([0-9]+\.[0-9]+\.[0-9]+[^/\s]*)/([^\s)"'|]+)`)

// Every documented download URL must name a file the release actually produces.
//
// GoReleaser's archive name_template puts the version *in the file name*, so an
// asset is `verikt_0.2.0_linux_amd64.tar.gz`. The install instructions asked for
// `verikt_linux_amd64.tar.gz` and had 404'd since v0.1.0 — nobody had ever
// successfully followed them.
//
// The version appears twice in such a URL, once in the path and once in the file
// name. Updating the release only touched the path, which moved the link from one
// broken state to another. Checking that the file name is derived from the same
// template GoReleaser uses catches both mistakes, and needs no network.
func TestDocumentedDownloadURLsMatchTheReleaseArchiveNames(t *testing.T) {
	root := repoRootFromTest(t)
	projectName, nameTemplate := goreleaserArchiveTemplate(t, root)

	var checked int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "target", "experiments", "dist", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".md", ".mdx", ".yml", ".yaml":
		default:
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || strings.HasPrefix(filepath.ToSlash(rel), "docs/internal/") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range downloadURL.FindAllStringSubmatch(line, -1) {
				checked++
				version, fileName := m[1], m[2]
				want := expectedArchivePattern(projectName, nameTemplate, version)
				if !want.MatchString(fileName) {
					t.Errorf("%s:%d links to %q, which no release asset is named.\n  archives.name_template renders to %s\n  line: %s",
						filepath.ToSlash(rel), i+1, fileName, want, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.NotZero(t, checked, "no download URLs found; this test guards nothing")
}

// goreleaserArchiveTemplate reads the project name and archive name_template.
func goreleaserArchiveTemplate(t *testing.T, root string) (string, string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, ".goreleaser.yaml"))
	require.NoError(t, err, "the archive naming is defined here")

	var cfg struct {
		ProjectName string `yaml:"project_name"`
		Archives    []struct {
			NameTemplate string `yaml:"name_template"`
		} `yaml:"archives"`
	}
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	require.NotEmpty(t, cfg.Archives, "no archives configured")
	require.NotEmpty(t, cfg.Archives[0].NameTemplate, "no archive name_template configured")

	return cfg.ProjectName, cfg.Archives[0].NameTemplate
}

// expectedArchivePattern renders the name_template into a regexp, leaving the
// platform fields open and pinning the project name and version.
func expectedArchivePattern(projectName, nameTemplate, version string) *regexp.Regexp {
	rendered := nameTemplate
	for _, field := range []struct{ token, value string }{
		{"{{ .ProjectName }}", regexp.QuoteMeta(projectName)},
		{"{{ .Version }}", regexp.QuoteMeta(version)},
		{"{{ .Os }}", `[a-z0-9]+`},
		{"{{ .Arch }}", `[a-z0-9]+`},
	} {
		rendered = strings.ReplaceAll(rendered, field.token, field.value)
	}
	// Archives are tar.gz, with zip overridden for Windows.
	return regexp.MustCompile(fmt.Sprintf(`^%s\.(tar\.gz|zip)$`, rendered))
}
