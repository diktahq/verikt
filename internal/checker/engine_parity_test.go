package checker

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// rustStrConst matches a Rust `const NAME: &str = "...";` declaration, including
// backslash line continuations inside the literal.
var rustStrConst = regexp.MustCompile(`(?s)const NAKED_GOROUTINE_MESSAGE: &str = "(.*?)";`)

// TestNakedGoroutineMessageMatchesEngine asserts the Go and Rust detectors report
// the same remedy for a bare `go` statement.
//
// Both implementations can produce this finding — the Rust engine on the normal
// path, the Go detector when the engine is unavailable — and the message lives in
// two files. Divergence means the advice a user receives depends on whether the
// embedded engine binary resolved, which is invisible to them.
func TestNakedGoroutineMessageMatchesEngine(t *testing.T) {
	enginePath := filepath.Join("..", "..", "engine", "crates", "engine-bin", "src", "antipatterns.rs")
	source, err := os.ReadFile(enginePath)
	if err != nil {
		t.Skipf("engine source not available at %s: %v", enginePath, err)
	}

	match := rustStrConst.FindSubmatch(source)
	if match == nil {
		t.Fatalf("NAKED_GOROUTINE_MESSAGE constant not found in %s — did the engine rename or inline it?", enginePath)
	}

	if got := unescapeRustLiteral(string(match[1])); got != nakedGoroutineMessage {
		t.Errorf("engine and Go messages differ.\nengine: %q\ngo:     %q", got, nakedGoroutineMessage)
	}
}

// unescapeRustLiteral resolves the escapes used in the engine's message literal:
// a backslash before a newline removes the newline and the next line's leading
// whitespace.
func unescapeRustLiteral(literal string) string {
	var b strings.Builder
	for i := 0; i < len(literal); i++ {
		if literal[i] != '\\' || i+1 >= len(literal) || literal[i+1] != '\n' {
			b.WriteByte(literal[i])
			continue
		}
		// Skip the backslash, the newline, and the continuation indentation.
		i++
		for i+1 < len(literal) && (literal[i+1] == ' ' || literal[i+1] == '\t') {
			i++
		}
	}
	return b.String()
}

var (
	goDetectorName   = regexp.MustCompile(`Name:\s+"([a-z0-9_]+)"`)
	rustDetectorName = regexp.MustCompile(`detector:\s+"([a-z0-9_]+)"`)
)

// TestDetectorSetsMatchEngine asserts the Go and Rust implementations detect the
// same set of anti-patterns.
//
// When the embedded engine resolves, its results REPLACE the Go ones wholesale
// (checker.CheckWithEngine), so any detector the engine lacks simply never fires
// — silently, on the default path. domain_imports_adapter and mvc_in_hexagonal
// were Go-only for exactly this reason: two architecture checks that never ran
// for most users.
func TestDetectorSetsMatchEngine(t *testing.T) {
	goSet := detectorNames(t, filepath.Join("antipatterns.go"), goDetectorName)
	enginePath := filepath.Join("..", "..", "engine", "crates", "engine-bin", "src", "antipatterns.rs")
	if _, err := os.Stat(enginePath); err != nil {
		t.Skipf("engine source not available: %v", err)
	}
	rustSet := detectorNames(t, enginePath, rustDetectorName)

	for name := range goSet {
		assert.Contains(t, rustSet, name, "detector %q exists in Go but not in the engine — it will not run on the default path", name)
	}
	for name := range rustSet {
		assert.Contains(t, goSet, name, "detector %q exists in the engine but not in Go — it will not run when the engine is unavailable", name)
	}
}

// detectorNames extracts detector identifiers from a source file.
func detectorNames(t *testing.T, path string, pattern *regexp.Regexp) map[string]bool {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	names := map[string]bool{}
	for _, m := range pattern.FindAllStringSubmatch(string(source), -1) {
		names[m[1]] = true
	}
	if len(names) == 0 {
		t.Fatalf("no detectors parsed from %s — did the literal shape change?", path)
	}
	return names
}

// The remedy must name the panic consequence, not just recommend errgroup:
// errgroup propagates a panic instead of recovering it, so the previous wording
// described a fix that does not prevent the crash.
func TestNakedGoroutineMessageNamesPanicConsequence(t *testing.T) {
	for _, want := range []string{"panic", "recover", "errgroup"} {
		if !strings.Contains(nakedGoroutineMessage, want) {
			t.Errorf("message should mention %q: %q", want, nakedGoroutineMessage)
		}
	}
}
