package engineclient

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every detector the engine defines must fire on the coverage fixture.
//
// The Go anti-pattern tests (506 lines, 13 detectors) were deleted with the Go
// analysis path, and the replacement lives in internal/engineclient/experiment —
// a package every CI job filters out of `go list`. What CI actually ran asserted
// NoError and NotNil(Summary), then logged findings in a loop that no-ops on an
// empty slice, so it passed with zero findings. Ten detectors had no CI coverage
// of any kind: a detector could stop firing entirely and nothing would say so.
//
// The fixture is passed as the project root, which is how the engine reaches
// code under testdata/ at all (INV-004).
func TestEveryDetectorFiresOnTheCoverageFixture(t *testing.T) {
	enginePath, err := EnginePath()
	if err != nil {
		t.Skipf("embedded engine unavailable on this platform: %v", err)
	}
	client := New(enginePath)

	fixture, err := filepath.Abs(filepath.Join("testdata", "detector-coverage"))
	require.NoError(t, err)

	findings, err := client.CheckAntiPatterns(fixture, nil)
	require.NoError(t, err, "the engine must run against the coverage fixture")

	fired := map[string]bool{}
	for _, f := range findings {
		fired[f.Name] = true
	}

	for detector := range detectorSeverities {
		assert.True(t, fired[detector],
			"detector %q did not fire on the coverage fixture — either it is broken or the fixture no longer triggers it",
			detector)
	}

	if t.Failed() {
		t.Logf("%d findings on the fixture:", len(findings))
		for _, f := range findings {
			t.Logf("  [%s] %s:%d — %s", f.Name, f.File, f.Line, f.Message)
		}
	}
}
