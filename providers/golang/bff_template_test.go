package golang

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBFFGatewayPackageTestsPassUnderRace renders the bff gateway package and
// runs the tests it ships with `go test -race`. The package uses only the
// standard library when observability is not selected, so it is copied into a
// throwaway module that needs no downloads.
//
// Regression for issue #32: FanOut appended unknown-service results without the
// mutex its goroutines held, which the race detector reports.
func TestBFFGatewayPackageTestsPassUnderRace(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the go toolchain")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}

	svc := scaffoldArch(t, "hexagonal", "http-api,bff")
	mod := t.TempDir()
	if err := os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module example.com/bffcheck\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkgDir := filepath.Join(mod, "bffgateway")
	copyGoFiles(t, filepath.Join(svc, "adapter", "bffgateway"), pkgDir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goBin, "test", "-race", "-count=1", "./...")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test -race on the rendered bff package failed: %v\n%s", err, out)
	}
}

func copyGoFiles(t *testing.T, from, to string) {
	t.Helper()
	entries, err := os.ReadDir(from)
	if err != nil {
		t.Fatalf("read rendered bff package: %v", err)
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		t.Fatal(err)
	}
	copied := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(from, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(to, e.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
		copied++
	}
	if copied == 0 {
		t.Fatalf("no Go files rendered under %s", from)
	}
}
