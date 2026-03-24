package engineclient

import (
	"context"
	"testing"

	pb "github.com/diktahq/verikt/internal/engineclient/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportGraph_ForbiddenImport(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	// cli must not import checker (only route through checker via check.go, not directly)
	rules := []*pb.Rule{
		{
			Id:       "no-direct-checker-import",
			Severity: pb.Severity_ERROR,
			Message:  "cli layer must not import checker directly",
			Engine:   pb.EngineType_IMPORT_GRAPH,
			Scope:    &pb.RuleScope{Include: []string{"**/*.go"}},
			Spec: &pb.Rule_ImportGraph{
				ImportGraph: &pb.ImportGraphSpec{
					PackagePattern: "internal/cli*",
					Forbidden:      []string{"internal/checker*"},
				},
			},
		},
	}

	result, err := client.Check(context.Background(), repoRoot, rules, nil)
	require.NoError(t, err)
	assert.NotNil(t, result.Summary)

	// cli/check.go imports checker — this should fire
	t.Logf("import_graph findings: %d", len(result.Findings))
	for _, f := range result.Findings {
		t.Logf("  %s: %s", f.File, f.Message)
	}
}

func TestImportGraph_NoViolationOnCompliantPackage(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	// rules package should not import cli — this should pass
	rules := []*pb.Rule{
		{
			Id:       "no-rules-imports-cli",
			Severity: pb.Severity_ERROR,
			Message:  "rules layer must not import cli",
			Engine:   pb.EngineType_IMPORT_GRAPH,
			Scope:    &pb.RuleScope{Include: []string{"**/*.go"}},
			Spec: &pb.Rule_ImportGraph{
				ImportGraph: &pb.ImportGraphSpec{
					PackagePattern: "internal/rules/**",
					Forbidden:      []string{"internal/cli*"},
				},
			},
		},
	}

	result, err := client.Check(context.Background(), repoRoot, rules, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Findings)
	t.Logf("files checked: %d", result.Summary.FilesChecked)
}

func TestImportGraph_MixedGrepAndImportGraph(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	rules := []*pb.Rule{
		{
			Id:       "no-direct-checker-import",
			Severity: pb.Severity_WARNING,
			Message:  "cli layer must not import checker directly",
			Engine:   pb.EngineType_IMPORT_GRAPH,
			Scope:    &pb.RuleScope{Include: []string{"**/*.go"}},
			Spec: &pb.Rule_ImportGraph{
				ImportGraph: &pb.ImportGraphSpec{
					PackagePattern: "internal/cli*",
					Forbidden:      []string{"internal/checker*"},
				},
			},
		},
		{
			Id:       "no-os-exit",
			Severity: pb.Severity_WARNING,
			Message:  "Do not call os.Exit directly",
			Engine:   pb.EngineType_GREP,
			Scope:    &pb.RuleScope{Include: []string{"**/*.go"}, Exclude: []string{"*_test.go"}},
			Spec: &pb.Rule_Grep{
				Grep: &pb.GrepSpec{Pattern: `os\.Exit\(`},
			},
		},
	}

	result, err := client.Check(context.Background(), repoRoot, rules, nil)
	require.NoError(t, err)
	assert.NotNil(t, result.Summary)
	t.Logf("total findings: %d (import_graph + grep)", len(result.Findings))
}
