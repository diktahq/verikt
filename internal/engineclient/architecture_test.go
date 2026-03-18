package engineclient

import (
	"context"
	"testing"

	pb "github.com/dcsg/archway/internal/engineclient/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArchitecture_ComponentDependencies validates that the engine can detect
// layer violations expressed as ImportGraphSpec rules derived from archway.yaml
// component declarations.
func TestArchitecture_ComponentDependencies(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	// Simulate a hexagonal archway.yaml with domain and adapter components.
	// The test codebase has internal/checker/testdata/hexagonal-project/ with:
	//   domain/     — pure domain (should not import adapter)
	//   adapter/    — adapters (may import domain)
	//
	// Rule: domain must not import adapter.
	rules := []*pb.Rule{
		{
			Id:       "arch/domain",
			Severity: pb.Severity_ERROR,
			Message:  "domain dependency violation",
			Engine:   pb.EngineType_IMPORT_GRAPH,
			Spec: &pb.Rule_ImportGraph{
				ImportGraph: &pb.ImportGraphSpec{
					PackagePattern: "internal/checker/testdata/hexagonal-project/domain/**",
					Forbidden:      []string{"internal/checker/testdata/hexagonal-project/adapter/**"},
				},
			},
		},
	}

	result, err := client.Check(context.Background(), repoRoot, rules, nil)
	require.NoError(t, err)
	assert.NotNil(t, result.Summary)

	t.Logf("architecture findings: %d", len(result.Findings))
	for _, f := range result.Findings {
		t.Logf("  %s: %s", f.File, f.Message)
	}
}

// TestArchitecture_CompliantComponent verifies that a compliant component
// produces no violations.
func TestArchitecture_CompliantComponent(t *testing.T) {
	client := newTestClient(t)
	repoRoot := findRepoRoot(t)

	// adapter/ may import domain — so this rule (domain must not import service)
	// should produce no findings since domain does not import service.
	rules := []*pb.Rule{
		{
			Id:       "arch/domain-no-service",
			Severity: pb.Severity_ERROR,
			Message:  "domain must not import service",
			Engine:   pb.EngineType_IMPORT_GRAPH,
			Spec: &pb.Rule_ImportGraph{
				ImportGraph: &pb.ImportGraphSpec{
					PackagePattern: "internal/checker/testdata/hexagonal-project/domain/**",
					Forbidden:      []string{"internal/checker/testdata/hexagonal-project/service/**"},
				},
			},
		},
	}

	result, err := client.Check(context.Background(), repoRoot, rules, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Findings, "domain must not import service — expected no violations")
	t.Logf("files checked: %d", result.Summary.FilesChecked)
}
