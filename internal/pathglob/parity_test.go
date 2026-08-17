package pathglob_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/pathglob"
	"github.com/stretchr/testify/require"
)

// ParityCase is one (pattern, path, expected) row from the shared fixture.
type ParityCase struct {
	Pattern string
	Path    string
	Match   bool
	Line    int
}

// LoadParityCases reads the fixture both language sides are checked against.
//
// It is exported through this test package's file layout so the Rust side can
// consume the same file: the fixture, not a duplicated table, is what makes the
// two implementations comparable.
func LoadParityCases(t *testing.T, path string) []ParityCase {
	t.Helper()

	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	var cases []ParityCase
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Split(scanner.Text(), "\t")
		require.Len(t, fields, 3, "line %d is malformed: %q", line, text)

		var want bool
		switch strings.TrimSpace(fields[2]) {
		case "match":
			want = true
		case "no-match":
			want = false
		default:
			t.Fatalf("line %d: expected match|no-match, got %q", line, fields[2])
		}

		cases = append(cases, ParityCase{
			Pattern: strings.TrimSpace(fields[0]),
			Path:    strings.TrimSpace(fields[1]),
			Match:   want,
			Line:    line,
		})
	}
	require.NoError(t, scanner.Err())
	require.NotEmpty(t, cases, "no cases loaded; this test guards nothing")
	return cases
}

// The Go matcher agrees with the shared fixture.
//
// The Rust side asserts the same file in engine-bin's glob_parity test. Neither
// test proves the two agree with each other directly — what they prove is that
// both agree with one written-down specification, which is the property that was
// missing. pathglob's package comment claimed to match globset; nothing checked
// it, and component patterns are matched by pathglob for orphan_package and
// missing_component but by globset for dependency and layer rules.
func TestGoMatcherAgreesWithSharedParityFixture(t *testing.T) {
	cases := LoadParityCases(t, filepath.Join("testdata", "parity-cases.txt"))

	for _, c := range cases {
		got := pathglob.Match(c.Pattern, c.Path)
		require.Equal(t, c.Match, got,
			"line %d: Match(%q, %q) = %v, fixture says %v",
			c.Line, c.Pattern, c.Path, got, c.Match)
	}
}

// The fixture must stay reachable from the Rust side, which reads it by relative
// path from the engine crate. A rename that only updates the Go test would leave
// the Rust test silently reading nothing.
func TestParityFixtureIsWhereTheEngineExpectsIt(t *testing.T) {
	const fromEngineCrate = "../../../internal/pathglob/testdata/parity-cases.txt"

	// engine/crates/engine-bin/ + the relative path the Rust test uses.
	resolved := filepath.Join("..", "..", "engine", "crates", "engine-bin", fromEngineCrate)
	_, err := os.Stat(resolved)
	require.NoError(t, err,
		"the Rust parity test reads %q relative to engine/crates/engine-bin; keep them in step", fromEngineCrate)
}
