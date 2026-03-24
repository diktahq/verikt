package experiment

// EXP-03: The Blank Slate
//
// Hypothesis: without the guide the agent produces flat/minimal structure.
// With the guide it produces hexagonal structure with domain/port/service/adapter layers.
//
// Run:
//   VERIKT_EXPERIMENT_AGENT=1 go test -run TestEXP03 -v -timeout 300s ./internal/engineclient/experiment/

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const exp03VeriktYAML = `language: go
architecture: hexagonal

components:
  - name: domain
    in: [domain/**]
  - name: ports
    in: [port/**]
    may_depend_on: [domain]
  - name: service
    in: [service/**]
    may_depend_on: [domain, ports]
  - name: adapters
    in: [adapter/**]
    may_depend_on: [domain, ports, service]

rules:
  functions:
    max_lines: 50
    max_params: 4
    max_return_values: 2

check:
  exclude:
    - cmd/**
`

// exp03TaskPrompt — "Build me a job runner" use case.
// Natural engineer prompt, no format instructions (those are in the system prompt).
const exp03TaskPrompt = `Build a notification service in Go. Needs to support email and SMS.
Add an HTTP API to trigger notifications.

Module name: github.com/example/notifications`

// TestEXP03_Control runs the task with no guide — baseline structural output.
func TestEXP03_Control(t *testing.T) {
	agentGuardOrSkip(t)

	m := runAgentExperiment(t, "EXP03-control", exp03TaskPrompt, exp03VeriktYAML)

	t.Logf("=== EXP-03 Control: packages=%v hexagonal=%v violations=%d passed=%v",
		m.Packages, isHexagonalShape(m.Packages), m.ViolationsTotal, m.Passed)
	t.Logf("=== EXP-03 Control: Response ===")
	t.Logf("%s", m.Response)
}

// TestEXP03_Test runs the task with the guide prepended.
func TestEXP03_Test(t *testing.T) {
	agentGuardOrSkip(t)

	bin := veriktBin(t)
	guide := generateGuideFromYAML(t, bin, exp03VeriktYAML)
	promptWithGuide := guide + "\n\n---\n\n" + exp03TaskPrompt

	m := runAgentExperiment(t, "EXP03-test", promptWithGuide, exp03VeriktYAML)

	t.Logf("=== EXP-03 Test: packages=%v hexagonal=%v violations=%d passed=%v",
		m.Packages, isHexagonalShape(m.Packages), m.ViolationsTotal, m.Passed)

	assert.True(t, m.Passed, "verikt check should pass with guide")
	assert.True(t, isHexagonalShape(m.Packages), "should produce hexagonal structure")

	t.Logf("=== EXP-03 Test: Response ===")
	t.Logf("%s", m.Response)
}

// TestEXP03_Contrast runs both conditions and logs the side-by-side comparison.
func TestEXP03_Contrast(t *testing.T) {
	agentGuardOrSkip(t)

	bin := veriktBin(t)
	guide := generateGuideFromYAML(t, bin, exp03VeriktYAML)

	without := runAgentExperiment(t, "EXP03-control", exp03TaskPrompt, exp03VeriktYAML)
	with := runAgentExperiment(t, "EXP03-test", guide+"\n\n---\n\n"+exp03TaskPrompt, exp03VeriktYAML)

	logContrast(t, without, with)
}
