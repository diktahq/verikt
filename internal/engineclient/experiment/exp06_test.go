package experiment

// EXP-06: Brownfield Rescue
//
// Two sub-experiments:
//
// EXP-06a (broad prompt): "refactor this project toward a cleaner structure"
//   → Demonstrates that vague prompts on large codebases produce zero output.
//   → The lesson: scope matters more than the guide for brownfield work.
//
// EXP-06b (scoped prompt): "extract a common Exporter interface from the exporter/ package"
//   → Tests whether the guide steers the refactoring toward hexagonal patterns.
//   → Without guide: agent refactors but may not produce port/adapter separation.
//   → With guide: agent should extract a port interface and keep adapters behind it.
//
// Setup (one-time):
//   cd internal/engineclient/experiment/testdata
//   git clone --branch v1.30.0 --depth 1 https://github.com/thomaspoignant/go-feature-flag go-feature-flag
//
// Run:
//   VERIKT_EXPERIMENT_AGENT=1 go test -run TestEXP06 -v -timeout 600s ./internal/engineclient/experiment/

import (
	"os"
	"path/filepath"
	"testing"
)

// --- EXP-06a: Broad prompt (expected to fail — the lesson) ---

const exp06aBroadPrompt = `This Go project has grown organically and the code is getting hard to follow.
Refactor it toward a cleaner structure -- separate the business logic from the
HTTP API and the storage backends. Don't change the public API or behavior.

Return ONLY the modified or new file contents using this exact format with no other text:
=== <filepath> ===
[complete file content]

Include only the files you need to create or modify.`

// TestEXP06a_BroadPrompt_Contrast runs the broad prompt — expected to produce zero files.
func TestEXP06a_BroadPrompt_Contrast(t *testing.T) {
	agentGuardOrSkip(t)
	requireBrownfieldFixture(t)

	without := runAgentExperimentWithFixture(t, "EXP06a-control", "go-feature-flag", exp06aBroadPrompt, false)
	with := runAgentExperimentWithFixture(t, "EXP06a-test", "go-feature-flag", exp06aBroadPrompt, true)

	logContrast(t, without, with)

	t.Logf("")
	t.Logf("=== EXP-06a: Broad Prompt Lesson ===")
	t.Logf("  Control files generated: %d", len(without.GeneratedFiles))
	t.Logf("  Test files generated:    %d", len(with.GeneratedFiles))
	if len(without.GeneratedFiles) == 0 && len(with.GeneratedFiles) == 0 {
		t.Logf("  EXPECTED: neither condition produced code — prompt too broad for the codebase size")
	}
}

// --- EXP-06b: Scoped prompt (the real experiment) ---

const exp06bScopedPrompt = `Look at the exporter/ package in this Go project. Each sub-package (fileexporter,
s3exporter, webhookexporter, etc.) implements a different export backend but there
is no shared interface — the main package calls each exporter directly.

Refactor the exporter/ package:
1. Define a common Exporter interface that all backends implement
2. Each backend should be a separate adapter behind that interface
3. Move shared logic (if any) into a shared package or the interface file
4. Don't change the public API or behavior of any exporter

Return ONLY the modified or new file contents using this exact format with no other text:
=== <filepath> ===
[complete file content]

Include only the files you need to create or modify.`

// TestEXP06b_Control runs the scoped refactoring without guide.
func TestEXP06b_Control(t *testing.T) {
	agentGuardOrSkip(t)
	requireBrownfieldFixture(t)

	m := runAgentExperimentWithFixture(t, "EXP06b-control", "go-feature-flag", exp06bScopedPrompt, false)

	t.Logf("=== EXP-06b Control: files=%d packages=%v violations=%d passed=%v",
		len(m.GeneratedFiles), m.Packages, m.ViolationsTotal, m.Passed)
	t.Logf("=== EXP-06b Control: Response ===")
	t.Logf("%s", m.Response)
}

// TestEXP06b_Test runs the scoped refactoring with guide prepended.
func TestEXP06b_Test(t *testing.T) {
	agentGuardOrSkip(t)
	requireBrownfieldFixture(t)

	m := runAgentExperimentWithFixture(t, "EXP06b-test", "go-feature-flag", exp06bScopedPrompt, true)

	t.Logf("=== EXP-06b Test: files=%d packages=%v violations=%d passed=%v",
		len(m.GeneratedFiles), m.Packages, m.ViolationsTotal, m.Passed)
	t.Logf("=== EXP-06b Test: Response ===")
	t.Logf("%s", m.Response)
}

// TestEXP06b_Contrast runs both conditions and logs the comparison.
func TestEXP06b_Contrast(t *testing.T) {
	agentGuardOrSkip(t)
	requireBrownfieldFixture(t)

	without := runAgentExperimentWithFixture(t, "EXP06b-control", "go-feature-flag", exp06bScopedPrompt, false)
	with := runAgentExperimentWithFixture(t, "EXP06b-test", "go-feature-flag", exp06bScopedPrompt, true)

	logContrast(t, without, with)

	t.Logf("")
	t.Logf("=== EXP-06b: Scoped Prompt Results ===")
	t.Logf("  Control files: %d | Test files: %d", len(without.GeneratedFiles), len(with.GeneratedFiles))
	t.Logf("  Control packages: %v", without.Packages)
	t.Logf("  Test packages:    %v", with.Packages)
}

// TestEXP06_Full runs both 12a and 12b sequentially — the full brownfield story.
func TestEXP06_Full(t *testing.T) {
	agentGuardOrSkip(t)
	requireBrownfieldFixture(t)

	t.Run("broad_prompt", func(t *testing.T) {
		without := runAgentExperimentWithFixture(t, "EXP06a-control", "go-feature-flag", exp06aBroadPrompt, false)
		with := runAgentExperimentWithFixture(t, "EXP06a-test", "go-feature-flag", exp06aBroadPrompt, true)
		logContrast(t, without, with)
	})

	t.Run("scoped_prompt", func(t *testing.T) {
		without := runAgentExperimentWithFixture(t, "EXP06b-control", "go-feature-flag", exp06bScopedPrompt, false)
		with := runAgentExperimentWithFixture(t, "EXP06b-test", "go-feature-flag", exp06bScopedPrompt, true)
		logContrast(t, without, with)
	})
}

// --- EXP-06c: Agentic refactoring with tool access ---

// exp06cAgenticPrompt — "Someone else's codebase" use case (no mapping).
// Senior engineer refactor prompt — read first, then change.
const exp06cAgenticPrompt = `The exporter/ package has multiple backends (fileexporter, s3exporter, webhookexporter, etc.)
but no shared interface. Add a common Exporter interface in exporter/exporter.go and make
each backend implement it. Don't change any existing behaviour.

Read the code first, then make the changes.`

// exp06dMappedPrompt adds explicit codebase-to-architecture mapping.
// Hypothesis: telling the agent which directories map to which architectural layers
// will change the refactoring outcome compared to just having the abstract rules.
const exp06dMappedPrompt = `You have access to the filesystem. Read the exporter/ package in this Go project.

This project follows hexagonal architecture. Here is how the exporter directories map:

- exporter/ (port layer) — this is where the Exporter interface should live
- exporter/fileexporter/ (adapter) — file export backend, implements the port
- exporter/s3exporter/ (adapter) — S3 export backend, implements the port
- exporter/s3exporterv2/ (adapter) — S3 v2 export backend, implements the port
- exporter/gcstorageexporter/ (adapter) — GCS export backend, implements the port
- exporter/webhookexporter/ (adapter) — webhook export backend, implements the port
- exporter/kafkaexporter/ (adapter) — Kafka export backend, implements the port
- exporter/logsexporter/ (adapter) — log export backend, implements the port
- exporter/pubsubexporter/ (adapter) — PubSub export backend, implements the port
- exporter/sqsexporter/ (adapter) — SQS export backend, implements the port

Refactor the exporter/ package:
1. Define a common Exporter interface in exporter/exporter.go (the port)
2. Each backend adapter must implement that interface
3. Adapters may import from the port (exporter/) but the port must NOT import from adapters
4. Don't change the public API or behavior of any exporter

Read the existing code first, then make the changes using the Edit and Write tools.`

// TestEXP06c_Control runs the agentic refactoring without guide.
func TestEXP06c_Control(t *testing.T) {
	agentGuardOrSkip(t)
	requireBrownfieldFixture(t)

	m := runAgentExperimentWithTools(t, "EXP06c-control", "go-feature-flag", exp06cAgenticPrompt, false)

	t.Logf("=== EXP-06c Control: violations=%d dep=%d fn=%d ap=%d arch=%d passed=%v",
		m.ViolationsTotal, m.ViolationsDep, m.ViolationsFn, m.ViolationsAP, m.ViolationsArch, m.Passed)
}

// TestEXP06c_Test runs the agentic refactoring with guide prepended.
func TestEXP06c_Test(t *testing.T) {
	agentGuardOrSkip(t)
	requireBrownfieldFixture(t)

	m := runAgentExperimentWithTools(t, "EXP06c-test", "go-feature-flag", exp06cAgenticPrompt, true)

	t.Logf("=== EXP-06c Test: violations=%d dep=%d fn=%d ap=%d arch=%d passed=%v",
		m.ViolationsTotal, m.ViolationsDep, m.ViolationsFn, m.ViolationsAP, m.ViolationsArch, m.Passed)
}

// TestEXP06c_Contrast runs both conditions and logs the comparison.
func TestEXP06c_Contrast(t *testing.T) {
	agentGuardOrSkip(t)
	requireBrownfieldFixture(t)

	without := runAgentExperimentWithTools(t, "EXP06c-control", "go-feature-flag", exp06cAgenticPrompt, false)
	with := runAgentExperimentWithTools(t, "EXP06c-test", "go-feature-flag", exp06cAgenticPrompt, true)

	logContrast(t, without, with)

	t.Logf("")
	t.Logf("=== EXP-06c: Agentic Brownfield Results ===")
	t.Logf("  Control: dep=%d fn=%d ap=%d arch=%d total=%d",
		without.ViolationsDep, without.ViolationsFn, without.ViolationsAP, without.ViolationsArch, without.ViolationsTotal)
	t.Logf("  Test:    dep=%d fn=%d ap=%d arch=%d total=%d",
		with.ViolationsDep, with.ViolationsFn, with.ViolationsAP, with.ViolationsArch, with.ViolationsTotal)
	delta := without.ViolationsTotal - with.ViolationsTotal
	t.Logf("  Violations reduced: %d", delta)
}

// --- EXP-06d: Agentic refactoring with explicit codebase mapping ---

// TestEXP06d_Contrast runs the mapped prompt (12d) vs unmapped (12c) — both with guide.
// If 12d produces fewer violations, the codebase mapping is the missing piece.
func TestEXP06d_Contrast(t *testing.T) {
	agentGuardOrSkip(t)
	requireBrownfieldFixture(t)

	// 12c: guide but no mapping
	noMapping := runAgentExperimentWithTools(t, "EXP06d-nomap", "go-feature-flag", exp06cAgenticPrompt, true)
	// 12d: guide + explicit mapping
	withMapping := runAgentExperimentWithTools(t, "EXP06d-mapped", "go-feature-flag", exp06dMappedPrompt, true)

	t.Logf("")
	t.Logf("=== EXP-06d: Codebase Mapping Hypothesis ===")
	t.Logf("  No mapping (guide only):  dep=%d fn=%d ap=%d arch=%d total=%d",
		noMapping.ViolationsDep, noMapping.ViolationsFn, noMapping.ViolationsAP, noMapping.ViolationsArch, noMapping.ViolationsTotal)
	t.Logf("  With mapping (guide+map): dep=%d fn=%d ap=%d arch=%d total=%d",
		withMapping.ViolationsDep, withMapping.ViolationsFn, withMapping.ViolationsAP, withMapping.ViolationsArch, withMapping.ViolationsTotal)
	delta := noMapping.ViolationsTotal - withMapping.ViolationsTotal
	t.Logf("  Violations reduced by mapping: %d", delta)
	if delta > 0 {
		t.Logf("  VALIDATED: codebase mapping reduces violations in brownfield refactoring")
	} else {
		t.Logf("  NOT VALIDATED: mapping did not reduce violations (delta=%d)", delta)
	}
}

// requireBrownfieldFixture skips the test if the go-feature-flag fixture is not present.
func requireBrownfieldFixture(t *testing.T) {
	t.Helper()
	fixture := testdataPath(t, "go-feature-flag")
	if _, err := os.Stat(filepath.Join(fixture, "go.mod")); err != nil {
		t.Skipf("brownfield fixture not found at %s — run setup instructions in this file", fixture)
	}
}
