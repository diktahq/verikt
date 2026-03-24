package experiment

// EXP-10: Governance Checkpoint Effect
//
// Isolates the governance checkpoint as the only variable.
// Both conditions receive the full verikt guide.
// Control: guide WITHOUT governance checkpoint section.
// Test: guide WITH governance checkpoint section (as generated).
//
// Fixtures: orders-service (Mode B) and greenfield (Mode A).
// 3 runs per condition per fixture.
// Measures: violation count delta, variance, per-category breakdown.
//
// Run:
//
//	VERIKT_EXPERIMENT_AGENT=1 go test -run TestEXP10 -v -timeout 600s ./internal/engineclient/experiment/

import (
	"fmt"
	"strings"
	"testing"
)

const exp10Runs = 3

// exp10TaskPromptFeatureAddition — same as EXP-07.
const exp10TaskPromptFeatureAddition = `Add a discount system. Discounts have a code and a percentage.
Apply the discount on order creation if a valid code is provided.`

// exp10TaskPromptGreenfield — same as EXP-03.
const exp10TaskPromptGreenfield = `Build a notification service in Go. Needs to support email and SMS.
Add an HTTP API to trigger notifications.

Module name: github.com/example/notifications`

// stripGovernanceCheckpoint removes the "## Governance Checkpoint" section from a guide string.
// It finds the section header, then finds the next "## " heading and removes everything in between.
// If the section is at the end of the string (no following ##), it strips to end of string.
// If the section does not exist, the guide is returned unchanged.
func stripGovernanceCheckpoint(guide string) string {
	const sectionHeader = "## Governance Checkpoint\n"

	start := strings.Index(guide, sectionHeader)
	if start == -1 {
		return guide
	}

	// Find the next "## " heading after the section start.
	searchFrom := start + len(sectionHeader)
	nextSection := strings.Index(guide[searchFrom:], "\n## ")
	if nextSection == -1 {
		// Section extends to end of string.
		return guide[:start]
	}

	// nextSection is relative to searchFrom; +1 to include the leading newline before "## ".
	end := searchFrom + nextSection + 1
	return guide[:start] + guide[end:]
}

// TestEXP10_FeatureAddition runs 3 runs per condition on the orders-service fixture.
// Both conditions receive the guide; control has the governance checkpoint stripped.
func TestEXP10_FeatureAddition(t *testing.T) {
	agentGuardOrSkip(t)

	bin := veriktBin(t)

	// Pre-generate the guide once from the fixture to ensure both conditions use the
	// same guide content — differing only in the presence of the checkpoint section.
	fixtureSrcDir := testdataPath(t, "orders-service")
	guideSetupDir := t.TempDir()
	copyDir(t, fixtureSrcDir, guideSetupDir)
	fullGuide := generateGuide(t, bin, guideSetupDir)
	strippedGuide := stripGovernanceCheckpoint(fullGuide)

	hasCheckpoint := strings.Contains(fullGuide, "## Governance Checkpoint")
	t.Logf("=== EXP-10 FeatureAddition: guide has checkpoint=%v, stripped differs=%v ===",
		hasCheckpoint, fullGuide != strippedGuide)

	t.Logf("=== EXP-10: Governance Checkpoint Effect — Feature Addition (%d runs/condition) ===", exp10Runs)

	controlViolations := make([]int, 0, exp10Runs)
	testViolations := make([]int, 0, exp10Runs)

	for i := range exp10Runs {
		run := i + 1
		t.Logf("--- Run %d/%d ---", run, exp10Runs)

		ctrl := RunFeatureAddition(t, RunConfig{
			ExperimentID:  "EXP-10",
			Condition:     "without-checkpoint",
			Run:           run,
			Label:         fmt.Sprintf("EXP10-fa-control-run%d", run),
			FixtureName:   "orders-service",
			TaskPrompt:    exp10TaskPromptFeatureAddition,
			WithGuide:     true,
			GuideOverride: &strippedGuide,
		})

		test := RunFeatureAddition(t, RunConfig{
			ExperimentID:  "EXP-10",
			Condition:     "with-checkpoint",
			Run:           run,
			Label:         fmt.Sprintf("EXP10-fa-test-run%d", run),
			FixtureName:   "orders-service",
			TaskPrompt:    exp10TaskPromptFeatureAddition,
			WithGuide:     true,
			GuideOverride: &fullGuide,
		})

		controlViolations = append(controlViolations, ctrl.ViolationsTotal)
		testViolations = append(testViolations, test.ViolationsTotal)

		t.Logf("  Run %d — without-checkpoint: violations=%d passed=%v | with-checkpoint: violations=%d passed=%v",
			run, ctrl.ViolationsTotal, ctrl.Passed, test.ViolationsTotal, test.Passed)
	}

	reportEXP10Results(t, "FeatureAddition", controlViolations, testViolations)
}

// TestEXP10_Greenfield runs 3 runs per condition on a greenfield build.
// Uses the same verikt.yaml and task prompt as EXP-03.
func TestEXP10_Greenfield(t *testing.T) {
	agentGuardOrSkip(t)

	bin := veriktBin(t)

	// Pre-generate the guide once from the EXP-03 verikt.yaml.
	fullGuide := generateGuideFromYAML(t, bin, exp03VeriktYAML)
	strippedGuide := stripGovernanceCheckpoint(fullGuide)

	hasCheckpoint := strings.Contains(fullGuide, "## Governance Checkpoint")
	t.Logf("=== EXP-10 Greenfield: guide has checkpoint=%v, stripped differs=%v ===",
		hasCheckpoint, fullGuide != strippedGuide)

	t.Logf("=== EXP-10: Governance Checkpoint Effect — Greenfield (%d runs/condition) ===", exp10Runs)

	controlViolations := make([]int, 0, exp10Runs)
	testViolations := make([]int, 0, exp10Runs)

	for i := range exp10Runs {
		run := i + 1
		t.Logf("--- Run %d/%d ---", run, exp10Runs)

		ctrl := RunGreenfield(t, RunConfig{
			ExperimentID:  "EXP-10",
			Condition:     "without-checkpoint",
			Run:           run,
			Label:         fmt.Sprintf("EXP10-gf-control-run%d", run),
			VeriktYAML:    exp03VeriktYAML,
			TaskPrompt:    exp10TaskPromptGreenfield,
			WithGuide:     true,
			GuideOverride: &strippedGuide,
		})

		test := RunGreenfield(t, RunConfig{
			ExperimentID:  "EXP-10",
			Condition:     "with-checkpoint",
			Run:           run,
			Label:         fmt.Sprintf("EXP10-gf-test-run%d", run),
			VeriktYAML:    exp03VeriktYAML,
			TaskPrompt:    exp10TaskPromptGreenfield,
			WithGuide:     true,
			GuideOverride: &fullGuide,
		})

		controlViolations = append(controlViolations, ctrl.ViolationsTotal)
		testViolations = append(testViolations, test.ViolationsTotal)

		t.Logf("  Run %d — without-checkpoint: violations=%d passed=%v | with-checkpoint: violations=%d passed=%v",
			run, ctrl.ViolationsTotal, ctrl.Passed, test.ViolationsTotal, test.Passed)
	}

	reportEXP10Results(t, "Greenfield", controlViolations, testViolations)
}

// TestEXP10_All runs both sub-experiments as subtests.
func TestEXP10_All(t *testing.T) {
	t.Run("FeatureAddition", TestEXP10_FeatureAddition)
	t.Run("Greenfield", TestEXP10_Greenfield)
}

// reportEXP10Results logs the summary statistics for one EXP-10 fixture condition pair.
func reportEXP10Results(t *testing.T, fixture string, controlViolations, testViolations []int) {
	t.Helper()

	ctrlVar := variance(controlViolations)
	testVar := variance(testViolations)
	ctrlMean := mean(controlViolations)
	testMean := mean(testViolations)
	delta := ctrlMean - testMean

	t.Logf("")
	t.Logf("=== EXP-10 Results (%s) ===", fixture)
	t.Logf("  Without-checkpoint violations per run: %v  (mean=%.2f variance=%.2f)", controlViolations, ctrlMean, ctrlVar)
	t.Logf("  With-checkpoint violations per run:    %v  (mean=%.2f variance=%.2f)", testViolations, testMean, testVar)
	t.Logf("  Mean delta (without - with): %.2f", delta)
	if delta > 0 {
		t.Logf("  IMPROVEMENT: checkpoint reduced violations by %.2f on average", delta)
	} else if delta < 0 {
		t.Logf("  REGRESSION: checkpoint increased violations by %.2f on average", -delta)
	} else {
		t.Logf("  NO CHANGE: checkpoint had no effect on violation count")
	}
	if testVar < ctrlVar {
		t.Logf("  Checkpoint reduced variance by %.2f", ctrlVar-testVar)
	}
}

// mean returns the arithmetic mean of a slice of ints.
func mean(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0
	for _, v := range vals {
		sum += v
	}
	return float64(sum) / float64(len(vals))
}
