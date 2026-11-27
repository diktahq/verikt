package golang

import (
	"bytes"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedGoIsGofmtClean: generated Go files must be gofmt-clean as
// written, before any post-scaffold hook runs. Hooks are skippable, and a
// failed `go mod tidy` stops them before `gofmt -w`. Partial imports used to
// land unsorted inside the module import group of internal/bootstrap.
func TestGeneratedGoIsGofmtClean(t *testing.T) {
	sets := map[string]string{
		"minimal":       "http-api",
		"platform-only": "platform",
		"bootstrap":     "health,http-api,bootstrap",
		"bff":           "bff,http-api",
		"all":           allCapabilities(t),
	}
	for _, arch := range []string{"hexagonal", "clean", "layered", "flat"} {
		for name, caps := range sets {
			t.Run(arch+"/"+name, func(t *testing.T) {
				out := scaffoldArch(t, arch, caps)
				for _, rel := range unformattedGoFiles(t, out) {
					t.Errorf("%s is not gofmt-clean", rel)
				}
			})
		}
	}
}

// allCapabilities lists every Go capability, comma-separated.
func allCapabilities(t *testing.T) string {
	t.Helper()
	entries, err := fs.ReadDir(templatesFS, "templates/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return strings.Join(names, ",")
}

// unformattedGoFiles returns the .go files under root that gofmt would change.
func unformattedGoFiles(t *testing.T, root string) []string {
	t.Helper()
	var bad []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		formatted, fmtErr := format.Source(src)
		switch {
		case fmtErr != nil:
			bad = append(bad, rel+" (does not parse: "+fmtErr.Error()+")")
		case !bytes.Equal(src, formatted):
			bad = append(bad, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return bad
}
