package golang

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCapabilityOrderDoesNotChangeBootstrap: partials render in capability
// order, so a capability typed before one it requires used its identifiers
// before they were declared (--cap health,http-api,bootstrap and
// --cap bff,http-api both failed with "undefined: router").
func TestCapabilityOrderDoesNotChangeBootstrap(t *testing.T) {
	orders := [][2]string{
		{"http-api,health,bootstrap", "health,http-api,bootstrap"},
		{"http-api,bff", "bff,http-api"},
	}
	for _, pair := range orders {
		t.Run(pair[1], func(t *testing.T) {
			want := readBootstrap(t, scaffoldArch(t, "hexagonal", pair[0]))
			got := readBootstrap(t, scaffoldArch(t, "hexagonal", pair[1]))
			if got != want {
				t.Errorf("--cap %s generated a different bootstrap.go than --cap %s:\n%s", pair[1], pair[0], got)
			}
			decl := strings.Index(got, "router := ")
			use := strings.Index(got, "router.")
			if decl < 0 || (use >= 0 && use < decl) {
				t.Errorf("bootstrap.go uses router before declaring it:\n%s", got)
			}
		})
	}
}

func readBootstrap(t *testing.T, out string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, "internal", "bootstrap", "bootstrap.go"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
