package experiment

// EXP-09: Capability Discovery
//
// The agent adds a payment call to the orders service, then lists what else it needs.
// Without the guide: generic suggestions (retry, logging, error handling).
// With the guide: verikt capability vocabulary (circuit-breaker, retry+idempotency, timeout).
//
// The primary measurement is qualitative (suggestion content), not quantitative (violations).
//
// Mode B (embedded fixture): verikt check --diff HEAD on agent changes only.
//
// Run:
//   VERIKT_EXPERIMENT_AGENT=1 go test -run TestEXP09 -v -timeout 300s ./internal/engineclient/experiment/

import (
	"strings"
	"testing"
)

// exp09TaskPrompt — "Capability discovery" use case.
// Ask the agent to implement then reflect on what's missing.
const exp09TaskPrompt = `Add payment processing to the CreateOrder flow. When an order is created,
charge the customer via POST https://api.payments.example.com/charge with the order total and customer ID.

After implementing this, list what other capabilities or infrastructure this service
should have, given what we just built. Be specific about why each one matters.

After the code files, add a section:
=== RECOMMENDATIONS ===
[your capability recommendations]`

// exp09CapabilityTerms are verikt vocabulary terms we look for in the recommendations.
var exp09CapabilityTerms = []string{
	"circuit-breaker", "circuit breaker",
	"retry", "backoff",
	"idempoten",
	"timeout",
}

// countCapabilityHits returns how many verikt vocabulary terms appear in the text.
func countCapabilityHits(text string) (int, []string) {
	lower := strings.ToLower(text)
	var found []string
	for _, term := range exp09CapabilityTerms {
		if strings.Contains(lower, term) {
			found = append(found, term)
		}
	}
	return len(found), found
}

// extractRecommendations returns the text after "=== RECOMMENDATIONS ===" in the response.
func extractRecommendations(response string) string {
	parts := strings.SplitN(response, "=== RECOMMENDATIONS ===", 2)
	if len(parts) < 2 {
		parts = strings.SplitN(response, "RECOMMENDATIONS", 2)
		if len(parts) < 2 {
			return response
		}
		return parts[1]
	}
	return strings.TrimSpace(parts[1])
}

// TestEXP09_Control adds payment call and lists capabilities without guide (Mode B, fixture in user prompt).
func TestEXP09_Control(t *testing.T) {
	m := RunFeatureAddition(t, RunConfig{
		ExperimentID:        "EXP-09",
		Condition:           "control",
		Run:                 1,
		Label:               "EXP09-control",
		FixtureName:         "orders-service",
		TaskPrompt:          exp09TaskPrompt,
		WithGuide:           false,
		FixtureInUserPrompt: true,
	})

	recs := extractRecommendations(m.Response)
	hits, found := countCapabilityHits(recs)

	t.Logf("=== EXP-09 Control: violations=%d passed=%v", m.ViolationsTotal, m.Passed)
	t.Logf("=== EXP-09 Control: capability vocabulary hits=%d terms=%v", hits, found)
	t.Logf("=== EXP-09 Control: Recommendations ===\n%s", recs)
}

// TestEXP09_Test adds payment call and lists capabilities with guide prepended (Mode B, fixture in user prompt).
func TestEXP09_Test(t *testing.T) {
	m := RunFeatureAddition(t, RunConfig{
		ExperimentID:        "EXP-09",
		Condition:           "test",
		Run:                 1,
		Label:               "EXP09-test",
		FixtureName:         "orders-service",
		TaskPrompt:          exp09TaskPrompt,
		WithGuide:           true,
		FixtureInUserPrompt: true,
	})

	recs := extractRecommendations(m.Response)
	hits, found := countCapabilityHits(recs)

	t.Logf("=== EXP-09 Test: violations=%d passed=%v", m.ViolationsTotal, m.Passed)
	t.Logf("=== EXP-09 Test: capability vocabulary hits=%d terms=%v", hits, found)
	t.Logf("=== EXP-09 Test: Recommendations ===\n%s", recs)
}

// TestEXP09_Contrast runs both conditions and compares capability vocabulary usage.
// Uses FixtureInUserPrompt so the system prompt does not restrict content after file blocks.
func TestEXP09_Contrast(t *testing.T) {
	without := RunFeatureAddition(t, RunConfig{
		ExperimentID:        "EXP-09",
		Condition:           "control",
		Run:                 1,
		Label:               "EXP09-control",
		FixtureName:         "orders-service",
		TaskPrompt:          exp09TaskPrompt,
		WithGuide:           false,
		FixtureInUserPrompt: true,
	})
	with := RunFeatureAddition(t, RunConfig{
		ExperimentID:        "EXP-09",
		Condition:           "test",
		Run:                 1,
		Label:               "EXP09-test",
		FixtureName:         "orders-service",
		TaskPrompt:          exp09TaskPrompt,
		WithGuide:           true,
		FixtureInUserPrompt: true,
	})

	woRecs := extractRecommendations(without.Response)
	wiRecs := extractRecommendations(with.Response)
	woHits, woFound := countCapabilityHits(woRecs)
	wiHits, wiFound := countCapabilityHits(wiRecs)

	t.Logf("")
	t.Logf("=== EXP-09: Capability Discovery ===")
	t.Logf("  Control — vocabulary hits: %d — terms: %v", woHits, woFound)
	t.Logf("  Test    — vocabulary hits: %d — terms: %v", wiHits, wiFound)
	t.Logf("  Control code violations: %d | Test code violations: %d",
		without.ViolationsTotal, with.ViolationsTotal)
	t.Logf("=== Control Recommendations ===\n%s", woRecs)
	t.Logf("=== Test Recommendations ===\n%s", wiRecs)
}
