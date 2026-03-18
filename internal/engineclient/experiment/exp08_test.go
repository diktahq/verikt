package experiment

// EXP-08: Anti-Pattern Prevention
//
// A job runner service where anti-patterns feel natural:
// naked goroutines, global state, SQL concatenation, UUID v4 keys.
// The guide's NEVER rules should suppress all of them.
//
// Run:
//   ARCHWAY_EXPERIMENT_AGENT=1 go test -run TestEXP08 -v -timeout 600s ./internal/engineclient/experiment/

import (
	"testing"
)

// exp08TaskPrompt — "Build me a job runner" use case (anti-pattern prevention).
// Natural prompt — the task invites naked goroutines, global state, SQL concat, UUID v4.
const exp08TaskPrompt = `Build a job runner service in Go. Jobs are stored in PostgreSQL with
an ID, type, payload (JSON), and status (pending/running/done/failed).
Poll every 5 seconds, run up to 3 concurrently, expose GET /jobs and GET /jobs/:id.

Module name: github.com/example/jobrunner`

const exp08Runs = 3

// TestEXP08_AntiPatternPrevention runs 3 runs per condition and reports anti-pattern rates.
func TestEXP08_AntiPatternPrevention(t *testing.T) {
	agentGuardOrSkip(t)

	t.Logf("=== EXP-08: Anti-Pattern Prevention (%d runs/condition) ===", exp08Runs)

	type runResult struct {
		ap     int
		total  int
		passed bool
	}

	controlRuns := make([]runResult, 0, exp08Runs)
	testRuns := make([]runResult, 0, exp08Runs)

	for i := range exp08Runs {
		t.Logf("--- Run %d/%d ---", i+1, exp08Runs)
		wo := runAgentExperimentWithFixture(t,
			"EXP08-control-run"+string(rune('1'+i)),
			"job-runner-stub", exp08TaskPrompt, false)
		wi := runAgentExperimentWithFixture(t,
			"EXP08-test-run"+string(rune('1'+i)),
			"job-runner-stub", exp08TaskPrompt, true)

		controlRuns = append(controlRuns, runResult{wo.ViolationsAP, wo.ViolationsTotal, wo.Passed})
		testRuns = append(testRuns, runResult{wi.ViolationsAP, wi.ViolationsTotal, wi.Passed})

		t.Logf("  Run %d — control: ap=%d total=%d | test: ap=%d total=%d",
			i+1, wo.ViolationsAP, wo.ViolationsTotal, wi.ViolationsAP, wi.ViolationsTotal)
	}

	t.Logf("")
	t.Logf("=== EXP-08 Results ===")
	controlAP, testAP := 0, 0
	for i, r := range controlRuns {
		t.Logf("  Control run %d: ap=%d total=%d passed=%v", i+1, r.ap, r.total, r.passed)
		controlAP += r.ap
	}
	for i, r := range testRuns {
		t.Logf("  Test    run %d: ap=%d total=%d passed=%v", i+1, r.ap, r.total, r.passed)
		testAP += r.ap
	}
	t.Logf("  Total anti-patterns — control: %d | test: %d", controlAP, testAP)
	if testAP == 0 {
		t.Logf("  PASS: guide eliminated all anti-patterns across all test runs")
	} else {
		t.Logf("  INFO: guide did not eliminate all anti-patterns (test total=%d)", testAP)
	}
}

// TestEXP08_Control_Run1 through Run3 allow running individual control runs.
func TestEXP08_Control_Run1(t *testing.T) {
	agentGuardOrSkip(t)
	m := runAgentExperimentWithFixture(t, "EXP08-control-run1", "job-runner-stub", exp08TaskPrompt, false)
	t.Logf("run1 control: ap=%d total=%d passed=%v", m.ViolationsAP, m.ViolationsTotal, m.Passed)
	t.Logf("%s", m.Response)
}

func TestEXP08_Control_Run2(t *testing.T) {
	agentGuardOrSkip(t)
	m := runAgentExperimentWithFixture(t, "EXP08-control-run2", "job-runner-stub", exp08TaskPrompt, false)
	t.Logf("run2 control: ap=%d total=%d passed=%v", m.ViolationsAP, m.ViolationsTotal, m.Passed)
	t.Logf("%s", m.Response)
}

func TestEXP08_Control_Run3(t *testing.T) {
	agentGuardOrSkip(t)
	m := runAgentExperimentWithFixture(t, "EXP08-control-run3", "job-runner-stub", exp08TaskPrompt, false)
	t.Logf("run3 control: ap=%d total=%d passed=%v", m.ViolationsAP, m.ViolationsTotal, m.Passed)
	t.Logf("%s", m.Response)
}

func TestEXP08_Test_Run1(t *testing.T) {
	agentGuardOrSkip(t)
	m := runAgentExperimentWithFixture(t, "EXP08-test-run1", "job-runner-stub", exp08TaskPrompt, true)
	t.Logf("run1 test: ap=%d total=%d passed=%v", m.ViolationsAP, m.ViolationsTotal, m.Passed)
	t.Logf("%s", m.Response)
}

func TestEXP08_Test_Run2(t *testing.T) {
	agentGuardOrSkip(t)
	m := runAgentExperimentWithFixture(t, "EXP08-test-run2", "job-runner-stub", exp08TaskPrompt, true)
	t.Logf("run2 test: ap=%d total=%d passed=%v", m.ViolationsAP, m.ViolationsTotal, m.Passed)
	t.Logf("%s", m.Response)
}

func TestEXP08_Test_Run3(t *testing.T) {
	agentGuardOrSkip(t)
	m := runAgentExperimentWithFixture(t, "EXP08-test-run3", "job-runner-stub", exp08TaskPrompt, true)
	t.Logf("run3 test: ap=%d total=%d passed=%v", m.ViolationsAP, m.ViolationsTotal, m.Passed)
	t.Logf("%s", m.Response)
}
