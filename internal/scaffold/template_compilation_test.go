package scaffold

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTemplateCompilation scaffolds each architecture into a temp dir and runs `go build ./...`.
// It only runs when VERIKT_TEST_GO_VERSION is set (used in CI template-matrix job).
func TestTemplateCompilation(t *testing.T) {
	goVersion := os.Getenv("VERIKT_TEST_GO_VERSION")
	if goVersion == "" {
		t.Skip("VERIKT_TEST_GO_VERSION not set, skipping template compilation test")
	}

	displayVersion := strings.TrimSuffix(goVersion, ".x")

	architectures := []string{"flat", "hexagonal", "layered", "clean"}

	providerFS := os.DirFS(filepath.Join("..", "..", "providers", "golang"))
	templateSubFS := os.DirFS(filepath.Join("..", "..", "providers", "golang", "templates"))
	bareRenderer := NewRenderer(templateSubFS)
	compositionRenderer := NewRenderer(providerFS)

	// Architecture-only tests (no capabilities).
	for _, arch := range architectures {
		t.Run(arch+"_bare_go"+displayVersion, func(t *testing.T) {
			outDir := t.TempDir()

			vars := map[string]interface{}{
				"ServiceName": "testsvc",
				"ModulePath":  "github.com/test/testsvc",
				"GoVersion":   displayVersion,
			}

			archDir := filepath.Join("architectures", arch)
			manifestPath := filepath.Join(archDir, "manifest.yaml")
			manifestData, err := os.ReadFile(filepath.Join("..", "..", "providers", "golang", "templates", manifestPath))
			if err != nil {
				t.Fatalf("read manifest: %v", err)
			}

			manifest, err := ParseManifest(manifestData)
			if err != nil {
				t.Fatalf("parse manifest: %v", err)
			}

			for key, value := range manifest.Defaults() {
				if _, exists := vars[key]; !exists {
					vars[key] = value
				}
			}

			_, err = bareRenderer.RenderTemplate(archDir, outDir, vars)
			if err != nil {
				t.Fatalf("RenderTemplate(%s) error = %v", arch, err)
			}

			goBuild(t, outDir, arch, displayVersion)
		})
	}

	// Architecture + capability composition tests.
	// These verify that capability templates compose correctly with architectures
	// and produce compilable Go code across Go versions.
	type combo struct {
		arch string
		caps []string
	}

	combos := []combo{
		// Hexagonal — core service patterns
		{"hexagonal", []string{"platform", "bootstrap", "http-api", "health"}},
		{"hexagonal", []string{"platform", "bootstrap", "grpc", "health"}},
		{"hexagonal", []string{"platform", "bootstrap", "http-api", "health", "cors", "validation"}},
		{"hexagonal", []string{"platform", "bootstrap", "http-api", "postgres", "uuid", "migrations", "health"}},
		{"hexagonal", []string{"platform", "bootstrap", "http-api", "postgres", "auth-jwt", "cors", "rate-limiting", "validation"}},
		{"hexagonal", []string{"platform", "bootstrap", "http-api", "http-client", "circuit-breaker", "retry", "timeout"}},
		{"hexagonal", []string{"platform", "bootstrap", "kafka-consumer", "mysql", "health"}},
		{"hexagonal", []string{"platform", "bootstrap", "http-api", "postgres", "event-bus", "outbox"}},
		{"hexagonal", []string{"platform", "bootstrap", "grpc", "redis", "health"}},

		// Layered
		{"layered", []string{"platform", "bootstrap", "http-api", "health"}},
		{"layered", []string{"platform", "bootstrap", "http-api", "postgres", "health"}},

		// Clean
		{"clean", []string{"platform", "bootstrap", "http-api", "health"}},
		{"clean", []string{"platform", "bootstrap", "http-api", "postgres", "health"}},

		// Flat
		{"flat", []string{"http-api", "health"}},
	}

	for _, c := range combos {
		name := fmt.Sprintf("%s_%s_go%s", c.arch, strings.Join(c.caps, "+"), displayVersion)
		t.Run(name, func(t *testing.T) {
			outDir := t.TempDir()

			vars := map[string]interface{}{
				"ServiceName": "testsvc",
				"ModulePath":  "github.com/test/testsvc",
				"GoVersion":   displayVersion,
			}

			plan, err := ComposeProject(providerFS, c.arch, c.caps, vars)
			if err != nil {
				t.Fatalf("ComposeProject(%s, %v) error = %v", c.arch, c.caps, err)
			}

			_, err = compositionRenderer.RenderComposition(plan, outDir)
			if err != nil {
				t.Fatalf("RenderComposition(%s, %v) error = %v", c.arch, c.caps, err)
			}

			goBuild(t, outDir, c.arch, displayVersion)
		})
	}
}

func goBuild(t *testing.T, dir, arch, goVersion string) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Skipf("no go.mod in scaffolded %s output, skipping build", arch)
	}

	// Resolve external dependencies before building.
	env := append(os.Environ(), "GONOSUMCHECK=*", "GONOSUMDB=*")

	tidy := exec.CommandContext(context.Background(), "go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = env
	tidy.CombinedOutput() //nolint:errcheck

	cmd := exec.CommandContext(context.Background(), "go", "build", "./...")
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed for %s (Go %s):\n%s", arch, goVersion, string(output))
	}
}
