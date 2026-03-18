package engineclient

import (
	"context"
	"testing"

	pb "github.com/dcsg/archway/internal/engineclient/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetric_FunctionLines(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	// Any function over 5 lines — will certainly fire on this codebase.
	result, err := client.Check(context.Background(), repoRoot, []*pb.Rule{
		{
			Id:       "max-function-lines",
			Severity: pb.Severity_WARNING,
			Message:  "function too long",
			Engine:   pb.EngineType_METRIC,
			Spec: &pb.Rule_FunctionMetric{
				FunctionMetric: &pb.FunctionMetricSpec{
					MaxLines: 5,
				},
			},
		},
	}, nil)
	require.NoError(t, err)
	assert.NotNil(t, result.Summary)
	assert.NotEmpty(t, result.Findings, "expected findings for max_lines=5 on real codebase")

	t.Logf("function_lines findings: %d", len(result.Findings))
	for i, f := range result.Findings {
		if i >= 5 {
			t.Logf("  ... and %d more", len(result.Findings)-5)
			break
		}
		t.Logf("  %s:%d: %s", f.File, f.Line, f.Message)
	}
}

func TestMetric_NoViolationsWithHighLimit(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	// Max 10000 lines — nothing should fire.
	result, err := client.Check(context.Background(), repoRoot, []*pb.Rule{
		{
			Id:       "max-lines-generous",
			Severity: pb.Severity_WARNING,
			Message:  "function too long",
			Engine:   pb.EngineType_METRIC,
			Spec: &pb.Rule_FunctionMetric{
				FunctionMetric: &pb.FunctionMetricSpec{
					MaxLines:  10000,
					MaxParams: 100,
				},
			},
		},
	}, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Findings, "expected no findings with very generous limits")
	t.Logf("files checked: %d", result.Summary.FilesChecked)
}

func TestMetric_MaxParams(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	// Max 1 param — will fire everywhere.
	result, err := client.Check(context.Background(), repoRoot, []*pb.Rule{
		{
			Id:       "max-params",
			Severity: pb.Severity_WARNING,
			Message:  "too many params",
			Engine:   pb.EngineType_METRIC,
			Spec: &pb.Rule_FunctionMetric{
				FunctionMetric: &pb.FunctionMetricSpec{
					MaxParams: 1,
				},
			},
		},
	}, nil)
	require.NoError(t, err)
	assert.NotNil(t, result.Summary)
	t.Logf("function_params findings: %d", len(result.Findings))
}
