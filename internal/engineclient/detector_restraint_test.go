package engineclient

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No detector may fire on code a careful engineer would write.
//
// This is the counterpart to TestEveryDetectorFiresOnTheCoverageFixture.
// Detection was guarded and restraint was not, so a detector could grow louder
// forever and no test objected — which is how global_mutable_state came to be
// wrong 21 times out of 22 on verikt's own source, reporting lookup tables that
// Go cannot express as constants and prescribing dependency injection for them.
//
// A false positive costs more than a miss. One teaches a reader to skim the
// section; a few teach them to ignore the tool. So a finding here fails the
// build, and adding a case is the first thing to do when a detector reports
// something nobody would act on.
func TestNoDetectorFiresOnCleanCode(t *testing.T) {
	enginePath, err := EnginePath()
	if err != nil {
		t.Skipf("embedded engine unavailable on this platform: %v", err)
	}
	client := New(enginePath)

	fixture, err := filepath.Abs(filepath.Join("testdata", "detector-restraint"))
	require.NoError(t, err)

	findings, err := client.CheckAntiPatterns(fixture, nil)
	require.NoError(t, err, "the engine must run against the restraint fixture")

	if len(findings) == 0 {
		return
	}

	reported := make([]string, 0, len(findings))
	for _, f := range findings {
		reported = append(reported, f.Name+"\t"+f.File+"\t"+f.Message)
	}
	sort.Strings(reported)

	t.Errorf("%d false positive(s) on code that should produce none:", len(findings))
	for _, line := range reported {
		t.Errorf("  %s", line)
	}
	t.Error("Either the detector is wrong, or the fixture genuinely contains the " +
		"problem and should be corrected. Waiving it in verikt.yaml is not the fix — " +
		"every user with the same shape of code sees the same false positive.")
}

// The restraint fixture is only meaningful if the engine actually reads it.
// An empty or unparsed fixture would pass the test above trivially.
func TestRestraintFixtureIsActuallyAnalysed(t *testing.T) {
	enginePath, err := EnginePath()
	if err != nil {
		t.Skipf("embedded engine unavailable on this platform: %v", err)
	}

	fixture, err := filepath.Abs(filepath.Join("testdata", "detector-restraint"))
	require.NoError(t, err)

	// The coverage fixture proves the detectors work; running them over the
	// restraint fixture with a deliberately broken file proves this fixture is
	// in scope rather than silently skipped.
	findings, err := New(enginePath).CheckAntiPatterns(
		filepath.Join(fixture, "..", "detector-coverage"), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, findings,
		"the sibling coverage fixture must still produce findings; if it does not, "+
			"the engine is not reading these testdata directories at all and the "+
			"restraint test above proves nothing")
}
