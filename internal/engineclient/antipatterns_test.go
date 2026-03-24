package engineclient

import (
	"context"
	"testing"

	pb "github.com/diktahq/verikt/internal/engineclient/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func antiPatternRule(id string, detectors ...string) *pb.Rule {
	return &pb.Rule{
		Id:       id,
		Severity: pb.Severity_WARNING,
		Message:  id,
		Engine:   pb.EngineType_ANTI_PATTERN,
		Spec: &pb.Rule_AntiPattern{
			AntiPattern: &pb.AntiPatternSpec{
				Detectors: detectors,
			},
		},
	}
}

func TestAntiPattern_NakedGoroutines(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	result, err := client.Check(context.Background(), repoRoot, []*pb.Rule{
		antiPatternRule("no-naked-goroutines", "naked_goroutine"),
	}, nil)
	require.NoError(t, err)
	assert.NotNil(t, result.Summary)
	t.Logf("naked_goroutine findings: %d", len(result.Findings))
	for _, f := range result.Findings {
		t.Logf("  %s:%d: %s", f.File, f.Line, f.Message)
	}
}

func TestAntiPattern_UUIDv4AsKey(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	result, err := client.Check(context.Background(), repoRoot, []*pb.Rule{
		antiPatternRule("no-uuid-v4", "uuid_v4_as_key"),
	}, nil)
	require.NoError(t, err)
	assert.NotNil(t, result.Summary)
	t.Logf("uuid_v4_as_key findings: %d", len(result.Findings))
	for _, f := range result.Findings {
		t.Logf("  %s:%d: %s", f.File, f.Line, f.Message)
	}
}

func TestAntiPattern_GlobalMutableState(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	result, err := client.Check(context.Background(), repoRoot, []*pb.Rule{
		antiPatternRule("no-global-mutable-state", "global_mutable_state"),
	}, nil)
	require.NoError(t, err)
	assert.NotNil(t, result.Summary)
	t.Logf("global_mutable_state findings: %d", len(result.Findings))
	for _, f := range result.Findings {
		t.Logf("  %s:%d: %s", f.File, f.Line, f.Message)
	}
}

func TestAntiPattern_AllDetectors(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	// Empty detectors list = all enabled.
	result, err := client.Check(context.Background(), repoRoot, []*pb.Rule{
		antiPatternRule("all-anti-patterns"),
	}, nil)
	require.NoError(t, err)
	assert.NotNil(t, result.Summary)

	byDetector := make(map[string]int)
	for _, f := range result.Findings {
		byDetector[f.Match]++
	}

	t.Logf("total anti-pattern findings: %d", len(result.Findings))
	for det, count := range byDetector {
		t.Logf("  %s: %d", det, count)
	}
	t.Logf("files checked: %d", result.Summary.FilesChecked)
}

func TestAntiPattern_SwallowedErrors(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	result, err := client.Check(context.Background(), repoRoot, []*pb.Rule{
		{
			Id:       "no-swallowed-errors",
			Severity: pb.Severity_ERROR,
			Message:  "error must not be swallowed",
			Engine:   pb.EngineType_ANTI_PATTERN,
			Spec: &pb.Rule_AntiPattern{
				AntiPattern: &pb.AntiPatternSpec{
					Detectors: []string{"swallowed_error"},
				},
			},
		},
	}, nil)
	require.NoError(t, err)
	assert.NotNil(t, result.Summary)
	t.Logf("swallowed_error findings: %d", len(result.Findings))
	for _, f := range result.Findings {
		t.Logf("  %s:%d: %s", f.File, f.Line, f.Message)
	}
}
