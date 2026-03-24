package experiment

// EXP-04: The Vague Feature Request
//
// Starting from an existing hexagonal orders service, the agent adds order cancellation.
// Without the guide, it likely places the business rule in the handler or service.
// With the guide, it places the rule in domain/, updates port/, implements in service/,
// and adds the HTTP endpoint in adapter/http/.
//
// Mode B (embedded fixture): the fixture is embedded in the system prompt so the agent
// can see the existing codebase. verikt check --diff HEAD measures only agent changes.
//
// Run:
//   VERIKT_EXPERIMENT_AGENT=1 go test -run TestEXP04 -v -timeout 300s ./internal/engineclient/experiment/

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// exp04TaskPrompt — "New feature, existing service" use case.
// One-liner ticket style — how a senior engineer would assign this task.
const exp04TaskPrompt = `Add order cancellation. Customers can cancel if the order hasn't shipped yet.`

// TestEXP04_Control adds cancellation without guide (Mode B, run 1).
func TestEXP04_Control(t *testing.T) {
	m := RunFeatureAddition(t, RunConfig{
		ExperimentID: "EXP-04",
		Condition:    "control",
		Run:          1,
		Label:        "EXP04-control",
		FixtureName:  "orders-service",
		TaskPrompt:   exp04TaskPrompt,
		WithGuide:    false,
	})

	t.Logf("=== EXP-04 Control: packages=%v violations=%d passed=%v",
		m.Packages, m.ViolationsTotal, m.Passed)
}

// TestEXP04_Test adds cancellation with guide prepended (Mode B, run 1).
func TestEXP04_Test(t *testing.T) {
	m := RunFeatureAddition(t, RunConfig{
		ExperimentID: "EXP-04",
		Condition:    "test",
		Run:          1,
		Label:        "EXP04-test",
		FixtureName:  "orders-service",
		TaskPrompt:   exp04TaskPrompt,
		WithGuide:    true,
	})

	t.Logf("=== EXP-04 Test: packages=%v violations=%d passed=%v",
		m.Packages, m.ViolationsTotal, m.Passed)

	assert.True(t, m.Passed, "verikt check --diff should pass with guide")
}

// TestEXP04_Contrast runs both conditions and logs the comparison (Mode B).
func TestEXP04_Contrast(t *testing.T) {
	without := RunFeatureAddition(t, RunConfig{
		ExperimentID: "EXP-04",
		Condition:    "control",
		Run:          1,
		Label:        "EXP04-control",
		FixtureName:  "orders-service",
		TaskPrompt:   exp04TaskPrompt,
		WithGuide:    false,
	})
	with := RunFeatureAddition(t, RunConfig{
		ExperimentID: "EXP-04",
		Condition:    "test",
		Run:          1,
		Label:        "EXP04-test",
		FixtureName:  "orders-service",
		TaskPrompt:   exp04TaskPrompt,
		WithGuide:    true,
	})

	logContrast(t, without.agentMetrics, with.agentMetrics)
}
