package experiment

// EXP-07: Consistency Under Repetition
//
// Same fixture as EXP-04 (orders-service), 3 runs per condition, 6 runs total.
// Measures variance in violations_total across runs.
// The guide should produce zero variance; control is expected to vary.
//
// Mode B (embedded fixture): archway check --diff HEAD on agent changes only.
//
// Run:
//   ARCHWAY_EXPERIMENT_AGENT=1 go test -run TestEXP07 -v -timeout 600s ./internal/engineclient/experiment/

import (
	"testing"
)

// exp07TaskPrompt — "Three engineers, one standard" / consistency use case.
const exp07TaskPrompt = `Add a discount system. Discounts have a code and a percentage.
Apply the discount on order creation if a valid code is provided.`

const exp07Runs = 3

// TestEXP07_Consistency runs 3 runs per condition and reports variance.
func TestEXP07_Consistency(t *testing.T) {
	t.Logf("=== EXP-07: Consistency Under Repetition (%d runs/condition) ===", exp07Runs)

	withoutViolations := make([]int, 0, exp07Runs)
	withViolations := make([]int, 0, exp07Runs)

	for i := range exp07Runs {
		run := i + 1
		t.Logf("--- Run %d/%d ---", run, exp07Runs)

		wo := RunFeatureAddition(t, RunConfig{
			ExperimentID: "EXP-07",
			Condition:    "control",
			Run:          run,
			Label:        "EXP07-control-run" + string(rune('0'+run)),
			FixtureName:  "orders-service",
			TaskPrompt:   exp07TaskPrompt,
			WithGuide:    false,
		})
		wi := RunFeatureAddition(t, RunConfig{
			ExperimentID: "EXP-07",
			Condition:    "test",
			Run:          run,
			Label:        "EXP07-test-run" + string(rune('0'+run)),
			FixtureName:  "orders-service",
			TaskPrompt:   exp07TaskPrompt,
			WithGuide:    true,
		})

		withoutViolations = append(withoutViolations, wo.ViolationsTotal)
		withViolations = append(withViolations, wi.ViolationsTotal)

		t.Logf("  Run %d — control: violations=%d passed=%v | test: violations=%d passed=%v",
			run, wo.ViolationsTotal, wo.Passed, wi.ViolationsTotal, wi.Passed)
	}

	woVar := variance(withoutViolations)
	wiVar := variance(withViolations)

	t.Logf("")
	t.Logf("=== EXP-07 Results ===")
	t.Logf("  Control violations per run: %v", withoutViolations)
	t.Logf("  Test violations per run:    %v", withViolations)
	t.Logf("  Control variance: %.2f", woVar)
	t.Logf("  Test variance:    %.2f", wiVar)
	if wiVar == 0 {
		t.Logf("  PASS: test condition shows zero variance")
	} else {
		t.Logf("  INFO: test condition variance=%.2f (expected 0)", wiVar)
	}
	if woVar > wiVar {
		t.Logf("  Guide reduced variance by %.2f", woVar-wiVar)
	}
}
