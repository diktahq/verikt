package golang

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/provider"
)

func scaffoldArch(t *testing.T, arch, caps string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "svc")
	opts := map[string]string{"skip_hooks": "true"}
	if caps != "" {
		opts["capabilities"] = caps
	}
	_, err := (&GoProvider{}).Scaffold(context.Background(), provider.ScaffoldRequest{
		ProjectName:  "svc",
		ModulePath:   "example.com/svc",
		TemplateName: arch,
		OutputDir:    out,
		Options:      opts,
	})
	if err != nil {
		t.Fatalf("scaffold %s (caps %q): %v", arch, caps, err)
	}
	return out
}

// TestScaffoldWritesManifestComponents: the components a project is checked
// against are the ones its architecture manifest declares (issue #18).
func TestScaffoldWritesManifestComponents(t *testing.T) {
	for _, arch := range []string{"hexagonal", "clean", "layered"} {
		t.Run(arch, func(t *testing.T) {
			manifest, err := loadManifest(path.Join("templates", "architectures", arch))
			if err != nil {
				t.Fatal(err)
			}
			if len(manifest.Components) == 0 {
				t.Fatalf("%s manifest declares no components", arch)
			}
			out := scaffoldArch(t, arch, "http-api")
			cfg, err := config.LoadVeriktYAML(filepath.Join(out, "verikt.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cfg.Components, manifest.Components) {
				t.Errorf("verikt.yaml components differ from the manifest:\n got  %+v\n want %+v", cfg.Components, manifest.Components)
			}
		})
	}
}

// TestDefaultConfigMatchesManifest keeps config.DefaultVeriktConfig, used by
// `verikt init` and `verikt analyze --init`, in step with the manifests that
// `verikt new` scaffolds from.
func TestDefaultConfigMatchesManifest(t *testing.T) {
	for _, arch := range []string{"hexagonal", "clean", "layered", "flat"} {
		t.Run(arch, func(t *testing.T) {
			manifest, err := loadManifest(path.Join("templates", "architectures", arch))
			if err != nil {
				t.Fatal(err)
			}
			got := config.DefaultVeriktConfig("go", arch).Components
			if len(got) == 0 && len(manifest.Components) == 0 {
				return
			}
			if !reflect.DeepEqual(got, manifest.Components) {
				t.Errorf("DefaultVeriktConfig(go, %s) components differ from the manifest:\n got  %+v\n want %+v", arch, got, manifest.Components)
			}
		})
	}
}

// TestFlatScaffoldWithServiceCapabilities: flat maps service/ to the project
// root, so capabilities that write under service/ must land there.
func TestFlatScaffoldWithServiceCapabilities(t *testing.T) {
	out := scaffoldArch(t, "flat", "http-api,testing,saga")

	if _, err := os.Stat(filepath.Join(out, "saga", "step.go")); err != nil {
		t.Errorf("saga step not rendered under saga/: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, "example_test.go"))
	if err != nil {
		t.Fatalf("testing example not rendered at the root: %v", err)
	}
	// The root of a flat project is package main; an external test package
	// there must be main_test or the directory does not compile.
	if !strings.Contains(string(data), "package main_test") {
		t.Errorf("root example_test.go has the wrong package clause:\n%s", firstLine(string(data)))
	}
}

// TestServiceTestPackageMatchesDirectory: the testing capability's example
// test is an external test package of whatever package service/ maps to.
func TestServiceTestPackageMatchesDirectory(t *testing.T) {
	cases := map[string]string{
		"hexagonal": "service/example_test.go",
		"clean":     "internal/usecase/example_test.go",
		"layered":   "internal/service/example_test.go",
	}
	for arch, rel := range cases {
		t.Run(arch, func(t *testing.T) {
			out := scaffoldArch(t, arch, "http-api,testing")
			data, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatal(err)
			}
			want := "package " + path.Base(path.Dir(rel)) + "_test"
			if got := firstLine(string(data)); got != want {
				t.Errorf("%s package clause = %q, want %q", rel, got, want)
			}
		})
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
