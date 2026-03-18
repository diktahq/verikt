package experiment

// EXP-05: Prompt Quality Compensation
//
// 2×2 design: {lazy, thorough} × {no-guide, guide} = conditions A, B, C, D.
// Flagship claim: B ≈ C — a lazy prompt with the guide produces output comparable
// to a thorough prompt without the guide.
//
// Run:
//   ARCHWAY_EXPERIMENT_AGENT=1 go test -run TestEXP05 -v -timeout 600s ./internal/engineclient/experiment/

import (
	"testing"
)

const exp05ArchwayYAML = `language: go
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

// exp05LazyPrompt — "Does the prompt even matter?" use case, lazy condition.
// This is how a junior engineer or a quick Slack message describes the task.
const exp05LazyPrompt = `Build an inventory service. Products have a name, SKU, and quantity.
Need CRUD and an HTTP API.

Module name: github.com/example/inventory`

// exp05ThoroughPrompt — "Does the prompt even matter?" use case, thorough condition.
// This is how a senior engineer writes a ticket: full requirements, explicit constraints.
const exp05ThoroughPrompt = `Build an inventory service in Go.

Products: name (required), SKU (unique, required), quantity (non-negative).
Operations: create, get by SKU, update quantity, delete.
HTTP API with proper 400/404 responses.
PostgreSQL for persistence — keep the storage layer behind an interface.
Domain errors (not found, validation) must be distinct from infrastructure errors.
No business logic in HTTP handlers.

Module name: github.com/example/inventory`

// TestEXP05_AllConditions runs all 4 conditions (A, B, C, D) and logs the comparison.
func TestEXP05_AllConditions(t *testing.T) {
	agentGuardOrSkip(t)

	bin := archwayBin(t)
	guide := generateGuideFromYAML(t, bin, exp05ArchwayYAML)

	// A: lazy + no guide
	a := runAgentExperiment(t, "EXP05-A-lazy-noguide", exp05LazyPrompt, exp05ArchwayYAML)
	// B: lazy + guide
	b := runAgentExperiment(t, "EXP05-B-lazy-guide", guide+"\n\n---\n\n"+exp05LazyPrompt, exp05ArchwayYAML)
	// C: thorough + no guide
	c := runAgentExperiment(t, "EXP05-C-thorough-noguide", exp05ThoroughPrompt, exp05ArchwayYAML)
	// D: thorough + guide
	d := runAgentExperiment(t, "EXP05-D-thorough-guide", guide+"\n\n---\n\n"+exp05ThoroughPrompt, exp05ArchwayYAML)

	t.Logf("")
	t.Logf("=== EXP-05: Prompt Quality Compensation ===")
	t.Logf("")
	t.Logf("  %-30s  %-8s  %-8s  %-10s  %-10s", "Condition", "Passed", "Hexagonal", "Violations", "Cost")
	t.Logf("  %-30s  %-8s  %-8s  %-10s  %-10s", "---", "---", "---", "---", "---")
	for _, m := range []agentMetrics{a, b, c, d} {
		t.Logf("  %-30s  %-8v  %-8v  %-10d  $%-9.4f",
			m.Label, m.Passed, isHexagonalShape(m.Packages), m.ViolationsTotal, m.CostUSD)
	}
	t.Logf("")

	// Flagship comparison: B vs C.
	diff := b.ViolationsTotal - c.ViolationsTotal
	if diff < 0 {
		diff = -diff
	}
	t.Logf("  B vs C violation diff: %d (claim: ≤1)", diff)
	t.Logf("  B packages: %v", b.Packages)
	t.Logf("  C packages: %v", c.Packages)
	t.Logf("  B≈C (|violations_B - violations_C| ≤ 1): %v", diff <= 1)

	t.Logf("")
	t.Logf("=== EXP-05 Responses ===")
	for _, label := range []string{"A", "B", "C", "D"} {
		m := map[string]agentMetrics{"A": a, "B": b, "C": c, "D": d}[label]
		t.Logf("--- Condition %s: %s ---", label, m.Label)
		t.Logf("%s", m.Response)
		t.Logf("")
	}
}

// TestEXP05_ConditionA is condition A in isolation: lazy prompt, no guide.
func TestEXP05_ConditionA(t *testing.T) {
	agentGuardOrSkip(t)
	m := runAgentExperiment(t, "EXP05-A-lazy-noguide", exp05LazyPrompt, exp05ArchwayYAML)
	t.Logf("A: packages=%v hexagonal=%v violations=%d", m.Packages, isHexagonalShape(m.Packages), m.ViolationsTotal)
	t.Logf("%s", m.Response)
}

// TestEXP05_ConditionB is condition B in isolation: lazy prompt, with guide.
func TestEXP05_ConditionB(t *testing.T) {
	agentGuardOrSkip(t)
	bin := archwayBin(t)
	guide := generateGuideFromYAML(t, bin, exp05ArchwayYAML)
	m := runAgentExperiment(t, "EXP05-B-lazy-guide", guide+"\n\n---\n\n"+exp05LazyPrompt, exp05ArchwayYAML)
	t.Logf("B: packages=%v hexagonal=%v violations=%d", m.Packages, isHexagonalShape(m.Packages), m.ViolationsTotal)
	t.Logf("%s", m.Response)
}

// TestEXP05_ConditionC is condition C in isolation: thorough prompt, no guide.
func TestEXP05_ConditionC(t *testing.T) {
	agentGuardOrSkip(t)
	m := runAgentExperiment(t, "EXP05-C-thorough-noguide", exp05ThoroughPrompt, exp05ArchwayYAML)
	t.Logf("C: packages=%v hexagonal=%v violations=%d", m.Packages, isHexagonalShape(m.Packages), m.ViolationsTotal)
	t.Logf("%s", m.Response)
}

// TestEXP05_ConditionD is condition D in isolation: thorough prompt, with guide.
func TestEXP05_ConditionD(t *testing.T) {
	agentGuardOrSkip(t)
	bin := archwayBin(t)
	guide := generateGuideFromYAML(t, bin, exp05ArchwayYAML)
	m := runAgentExperiment(t, "EXP05-D-thorough-guide", guide+"\n\n---\n\n"+exp05ThoroughPrompt, exp05ArchwayYAML)
	t.Logf("D: packages=%v hexagonal=%v violations=%d", m.Packages, isHexagonalShape(m.Packages), m.ViolationsTotal)
	t.Logf("%s", m.Response)
}

// TestEXP05_BvsC compares conditions B and C directly — the flagship claim.
func TestEXP05_BvsC(t *testing.T) {
	agentGuardOrSkip(t)

	bin := archwayBin(t)
	guide := generateGuideFromYAML(t, bin, exp05ArchwayYAML)

	b := runAgentExperiment(t, "EXP05-B-lazy-guide", guide+"\n\n---\n\n"+exp05LazyPrompt, exp05ArchwayYAML)
	c := runAgentExperiment(t, "EXP05-C-thorough-noguide", exp05ThoroughPrompt, exp05ArchwayYAML)

	t.Logf("")
	t.Logf("=== EXP-05: B vs C (flagship comparison) ===")
	t.Logf("  B (lazy+guide):      violations=%d hexagonal=%v passed=%v pkgs=%v",
		b.ViolationsTotal, isHexagonalShape(b.Packages), b.Passed, b.Packages)
	t.Logf("  C (thorough+noguide): violations=%d hexagonal=%v passed=%v pkgs=%v",
		c.ViolationsTotal, isHexagonalShape(c.Packages), c.Passed, c.Packages)

	diff := b.ViolationsTotal - c.ViolationsTotal
	if diff < 0 {
		diff = -diff
	}
	t.Logf("  |violations_B - violations_C| = %d", diff)
	t.Logf("  B≈C claim holds (diff ≤ 1): %v", diff <= 1)
	t.Logf("  Both multi-package: %v", len(b.Packages) >= 3 && len(c.Packages) >= 3)
	t.Logf("  B cost: $%.4f | C cost: $%.4f", b.CostUSD, c.CostUSD)
}
