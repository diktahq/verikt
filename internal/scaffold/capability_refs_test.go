package scaffold_test

import (
	"io/fs"
	"path"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/diktahq/verikt/internal/scaffold"
	golangprovider "github.com/diktahq/verikt/providers/golang"
	tsprovider "github.com/diktahq/verikt/providers/typescript"
)

// TestCapabilityReferencesExist fails when any capability's requires, suggests
// or conflicts names a capability that has no directory in the same provider.
// A dangling requires makes `verikt new --cap <it>` fail outright (issue #16).
func TestCapabilityReferencesExist(t *testing.T) {
	providers := map[string]fs.FS{
		"golang":     (&golangprovider.GoProvider{}).GetTemplateFS(),
		"typescript": (&tsprovider.TypeScriptProvider{}).GetTemplateFS(),
	}
	for name, fsys := range providers {
		t.Run(name, func(t *testing.T) {
			manifests := loadCapabilityManifests(t, fsys)

			var dangling []string
			for capName, cm := range manifests {
				for field, refs := range map[string][]string{
					"requires":  cm.Requires,
					"suggests":  cm.Suggests,
					"conflicts": cm.Conflicts,
				} {
					for _, ref := range refs {
						if _, ok := manifests[ref]; !ok {
							dangling = append(dangling, capName+" "+field+" "+ref)
						}
					}
				}
			}
			slices.Sort(dangling)
			for _, d := range dangling {
				t.Errorf("dangling capability reference: %s (no such capability)", d)
			}
		})
	}
}

// loadCapabilityManifests parses every capability.yaml under
// templates/capabilities, keyed by directory name. Dot-directories are not
// capabilities (editor and tooling folders can appear there in a checkout).
func loadCapabilityManifests(t *testing.T, fsys fs.FS) map[string]*scaffold.CapabilityManifest {
	t.Helper()
	capsDir := path.Join("templates", "capabilities")
	entries, err := fs.ReadDir(fsys, capsDir)
	if err != nil {
		t.Fatalf("read %s: %v", capsDir, err)
	}
	manifests := map[string]*scaffold.CapabilityManifest{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		data, err := fs.ReadFile(fsys, path.Join(capsDir, e.Name(), "capability.yaml"))
		if err != nil {
			t.Errorf("capability %q: %v", e.Name(), err)
			continue
		}
		cm, err := scaffold.ParseCapabilityManifest(data)
		if err != nil {
			t.Errorf("capability %q: %v", e.Name(), err)
			continue
		}
		manifests[e.Name()] = cm
	}
	if len(manifests) == 0 {
		t.Fatal("no capabilities found")
	}
	return manifests
}

// TestComposeProject_UnknownRequirement checks that a manifest requiring a
// capability that does not exist fails with an error naming both, instead of
// a generic "file does not exist" from a later manifest read.
func TestComposeProject_UnknownRequirement(t *testing.T) {
	fsys := fstest.MapFS{
		"templates/architectures/flat/manifest.yaml": {Data: []byte("name: flat\n")},
		"templates/capabilities/s3/capability.yaml":  {Data: []byte("name: s3\nrequires: [config]\n")},
	}
	_, err := scaffold.ComposeProject(fsys, "flat", []string{"s3"}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for unknown requirement, got nil")
	}
	for _, want := range []string{`"s3"`, `"config"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}
