package golang

import (
	"os"
	"strings"
	"testing"
)

// The guide's middleware table describes the router the http-api template
// generates. It must not list middleware the router no longer uses (RealIP was
// removed because it trusts client-supplied forwarding headers), and it must
// list them in the router's order: Recoverer is outermost.
func TestProjectMatrixMiddlewareTableMatchesRouter(t *testing.T) {
	docPath, err := generateProjectMatrix(t.TempDir(), "hexagonal", []string{"http-api"}, nil)
	if err != nil {
		t.Fatalf("generateProjectMatrix: %v", err)
	}
	raw, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read project matrix: %v", err)
	}
	out := string(raw)

	if strings.Contains(out, "RealIP") {
		t.Errorf("middleware table lists RealIP, which the generated router no longer uses:\n%s", out)
	}
	if !strings.Contains(out, "| 1 | Recoverer |") {
		t.Errorf("middleware table does not start with Recoverer, the router's outermost middleware:\n%s", out)
	}
}
