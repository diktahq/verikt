package engineclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/diktahq/verikt/internal/engineclient/pb"
)

// The engine sends one CheckComplete per module; the client must combine them.
//
// engine/crates/engine-bin/src/main.rs runs grep, import_graph, antipatterns and
// metrics for every request and each emits its own summary, with a comment
// stating "The Go client merges them". It did not — the decode was a plain
// assignment, so every summary but the last was discarded.
//
// Today each call site sends rules of one engine type, and grep is emitted first
// and never early-returns, so the overwrite happens to land on the right module.
// That is an accident of ordering. A request carrying grep rules *and* any other
// type loses one module's rule_statuses entirely, and the proxy-rule path reads
// exactly that field — so every grep rule would be reported as absent.
func TestCheckMergesEveryModuleSummary(t *testing.T) {
	requireEmbeddedEngine(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/app\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "a"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "a", "a.go"),
		// The write is what makes Global mutable state rather than a lookup
		// table, and it is what makes the anti-pattern module emit a finding —
		// this test needs two modules to report, not one.
		[]byte("package a\n\nvar Global = map[string]int{}\n\nfunc Put(k string, v int) { Global[k] = v }\n\nvar _ = Sprintf\n"), 0o644))

	enginePath, err := EnginePath()
	require.NoError(t, err)
	client := New(enginePath)

	// One grep rule and one anti-pattern rule in a single request — the mixed
	// case that loses a summary.
	rules := []*pb.Rule{
		{
			Id:       "no-sprintf",
			Severity: pb.Severity_ERROR,
			Message:  "no Sprintf",
			Engine:   pb.EngineType_GREP,
			Scope:    &pb.RuleScope{Include: []string{"**/*.go"}},
			Spec:     &pb.Rule_Grep{Grep: &pb.GrepSpec{Pattern: "Sprintf"}},
		},
		{
			Id:       "anti-patterns",
			Severity: pb.Severity_WARNING,
			Message:  "anti-pattern",
			Engine:   pb.EngineType_ANTI_PATTERN,
			Spec:     &pb.Rule_AntiPattern{AntiPattern: &pb.AntiPatternSpec{}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := client.Check(ctx, dir, rules, nil)
	require.NoError(t, err)
	require.NotNil(t, result.Summary)

	statuses := map[string]bool{}
	for _, s := range result.Summary.RuleStatuses {
		statuses[s.RuleId] = true
	}

	assert.True(t, statuses["no-sprintf"],
		"the grep rule's status was discarded by a later module's summary")
	assert.True(t, statuses["anti-patterns"],
		"the anti-pattern rule's status is missing")

	// Counts must be totals across modules, not whichever module reported last.
	assert.GreaterOrEqual(t, result.Summary.FindingsTotal, uint32(2),
		"findings_total should sum every module's findings, got %d", result.Summary.FindingsTotal)
	assert.Len(t, result.Findings, int(result.Summary.FindingsTotal),
		"the summary total must match the findings actually delivered")
}
