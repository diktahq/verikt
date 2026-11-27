package cli

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/diktahq/verikt/internal/provider"
)

// featureLang is a provider registered only for these tests. It has a Go-style
// feature matrix, a capability that needs range-over-func iterators (Go 1.23+)
// and one that renders {{.GoVersion}}, and it never detects a toolchain, so the
// results do not depend on the Go installed on the test machine.
const featureLang = "featuretest"

type featureTestProvider struct{ tfs fs.FS }

func (p *featureTestProvider) Scaffold(context.Context, provider.ScaffoldRequest) (*provider.ScaffoldResponse, error) {
	return nil, provider.ErrNotImplemented
}
func (p *featureTestProvider) Analyze(context.Context, provider.AnalyzeRequest) (*provider.AnalyzeResponse, error) {
	return nil, provider.ErrNotImplemented
}
func (p *featureTestProvider) Migrate(context.Context, provider.MigrateRequest) (*provider.MigrateResponse, error) {
	return nil, provider.ErrNotImplemented
}
func (p *featureTestProvider) GetInfo(context.Context) (*provider.ProviderInfo, error) {
	return &provider.ProviderInfo{Name: featureLang}, nil
}
func (p *featureTestProvider) GetTemplateFS() fs.FS                          { return p.tfs }
func (p *featureTestProvider) DetectVersion(context.Context) (string, error) { return "", nil }
func (p *featureTestProvider) GetFeatureMatrix() ([]byte, error) {
	return fs.ReadFile(p.tfs, "templates/features.yaml")
}

func init() {
	provider.Register(featureLang, &featureTestProvider{tfs: fstest.MapFS{
		"templates/features.yaml": {Data: []byte(`features:
  - name: range_over_int
    since: "1.22"
  - name: range_over_func
    since: "1.23"
`)},
		"templates/architectures/hexagonal/manifest.yaml": {Data: []byte(`name: hexagonal
description: "Hexagonal"
variables:
  - name: ServiceName
    type: string
    required: true
  - name: ModulePath
    type: string
    required: true
  - name: GoVersion
    type: string
    default: "1.22"
`)},
		"templates/capabilities/iter/capability.yaml": {Data: []byte(`name: iter
description: "Needs iterators"
requires_features: [range_over_func]
`)},
		"templates/capabilities/iter/files/iter/iter.go.tmpl": {Data: []byte("package iter\n")},
		"templates/capabilities/ci/capability.yaml":           {Data: []byte("name: ci\ndescription: \"CI\"\n")},
		"templates/capabilities/ci/files/ci.yml.tmpl":         {Data: []byte("image: golang:{{.GoVersion}}\n")},
	}})
}

// featureProject writes a project for featureLang whose go.mod declares
// goDirective, and changes into it.
func featureProject(t *testing.T, goDirective string) string {
	t.Helper()
	dir := t.TempDir()
	yaml := "language: " + featureLang + "\narchitecture: hexagonal\n"
	if err := os.WriteFile(filepath.Join(dir, "verikt.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	gomod := "module example.com/svc\n\ngo " + goDirective + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	return dir
}

func addCapability(t *testing.T, capability string) error {
	t.Helper()
	var err error
	captureStdout(t, func() { _, err = executeCommand(t, "add", capability) })
	return err
}

func TestAdd_RequiresFeature_RejectedWhenGoModLacksIt(t *testing.T) {
	dir := featureProject(t, "1.22")
	err := addCapability(t, "iter")
	if err == nil {
		t.Fatal("adding a capability that needs Go 1.23 to a go 1.22 project should fail")
	}
	for _, want := range []string{`capability "iter"`, `"range_over_func"`, "1.23", "go.mod", "1.22"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
	if _, statErr := os.Stat(filepath.Join(dir, "iter", "iter.go")); statErr == nil {
		t.Error("no files should be written when a required feature is missing")
	}
}

func TestAdd_RequiresFeature_AcceptedWhenGoModHasIt(t *testing.T) {
	dir := featureProject(t, "1.23.4")
	if err := addCapability(t, "iter"); err != nil {
		t.Fatalf("go 1.23.4 provides range_over_func: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "iter", "iter.go")); err != nil {
		t.Errorf("iter.go should be rendered: %v", err)
	}
}

func TestAdd_GoVersionComesFromGoMod(t *testing.T) {
	dir := featureProject(t, "1.25.2")
	if err := addCapability(t, "ci"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "image: golang:1.25.2\n" {
		t.Errorf("GoVersion should come from go.mod, not the manifest default; got %q", got)
	}
}
