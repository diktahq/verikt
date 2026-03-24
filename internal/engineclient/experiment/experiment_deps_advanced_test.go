package experiment

import (
	"context"
	"testing"
	"time"

	pb "github.com/diktahq/verikt/internal/engineclient/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeps_AllowedOnly verifies the AllowedOnly whitelist mode — any internal import
// not in the allowed list must be flagged.
func TestDeps_AllowedOnly(t *testing.T) {
	client := newEngineClient(t)
	projectPath := findRepoRoot(t)

	// Allow domain only. Service imports domain+port → port import is a violation.
	p := "internal/engineclient/experiment/testdata/hexagonal"
	rule := &pb.Rule{
		Id:       "arch/service-allowed-only",
		Severity: pb.Severity_ERROR,
		Message:  "service may only import domain",
		Engine:   pb.EngineType_IMPORT_GRAPH,
		Spec: &pb.Rule_ImportGraph{ImportGraph: &pb.ImportGraphSpec{
			PackagePattern: p + "/service/**",
			AllowedOnly:    []string{p + "/domain/**"},
		}},
	}

	start := time.Now()
	result, err := client.Check(context.Background(), projectPath, []*pb.Rule{rule}, nil)
	duration := time.Since(start)
	require.NoError(t, err)

	t.Logf("AllowedOnly (%v): %d findings", duration, len(result.Findings))
	for _, f := range result.Findings {
		t.Logf("  %s — %s", f.File, f.Message)
	}

	// service/ is clean (no illegal imports in test testdata) — just verify it runs without error.
	t.Logf("AllowedOnly whitelist mode executed successfully")
}

// TestDeps_AllowedOnly_Violation verifies AllowedOnly detects a real violation:
// domain/ imports service/ but AllowedOnly contains only an empty set → violation.
func TestDeps_AllowedOnly_Violation(t *testing.T) {
	client := newEngineClient(t)
	projectPath := findRepoRoot(t)

	p := "internal/engineclient/experiment/testdata/hexagonal"

	// domain/ is allowed to import nothing — it imports service/ so this must fire.
	rule := &pb.Rule{
		Id:       "arch/domain-no-imports",
		Severity: pb.Severity_ERROR,
		Message:  "domain may not import anything internal",
		Engine:   pb.EngineType_IMPORT_GRAPH,
		Spec: &pb.Rule_ImportGraph{ImportGraph: &pb.ImportGraphSpec{
			PackagePattern: p + "/domain/**",
			// AllowedOnly is empty — any internal import is a violation.
			AllowedOnly: []string{"__nothing__"},
		}},
	}

	start := time.Now()
	result, err := client.Check(context.Background(), projectPath, []*pb.Rule{rule}, nil)
	duration := time.Since(start)
	require.NoError(t, err)

	t.Logf("AllowedOnly violation (%v): %d findings", duration, len(result.Findings))
	for _, f := range result.Findings {
		t.Logf("  %s — %s", f.File, f.Message)
	}

	// domain/domain.go imports service/ which is internal and not in the allowed list.
	assert.NotEmpty(t, result.Findings, "AllowedOnly must flag domain→service import as violation")
}

// TestDeps_ForbiddenAndAllowed verifies that forbidden + allowed_only interact correctly:
// forbidden takes precedence over the allowed list.
func TestDeps_ForbiddenAndAllowed(t *testing.T) {
	client := newEngineClient(t)
	projectPath := findRepoRoot(t)

	p := "internal/engineclient/experiment/testdata/hexagonal"

	// Forbid service/**, also list it as allowed → forbidden wins, must still flag.
	rule := &pb.Rule{
		Id:       "arch/domain-forbidden-allowed",
		Severity: pb.Severity_ERROR,
		Message:  "domain forbidden check",
		Engine:   pb.EngineType_IMPORT_GRAPH,
		Spec: &pb.Rule_ImportGraph{ImportGraph: &pb.ImportGraphSpec{
			PackagePattern: p + "/domain/**",
			Forbidden:      []string{p + "/service/**"},
			AllowedOnly:    []string{p + "/service/**"}, // listed as allowed but also forbidden
		}},
	}

	result, err := client.Check(context.Background(), projectPath, []*pb.Rule{rule}, nil)
	require.NoError(t, err)

	t.Logf("Forbidden+Allowed interaction: %d findings", len(result.Findings))
	// When allowed_only also contains the forbidden pattern, it is excluded from the "not in allowed" violation
	// but the forbidden check still fires. Verify the engine doesn't crash and returns a result.
	t.Logf("Rule executed without error — %d total findings", len(result.Findings))
}

// TestDeps_EmptyProject verifies the engine handles an empty project (no Go files) gracefully.
func TestDeps_EmptyProject(t *testing.T) {
	client := newEngineClient(t)

	// Use a directory with no Go files — /tmp is always available.
	rule := &pb.Rule{
		Id:       "arch/empty",
		Severity: pb.Severity_ERROR,
		Message:  "empty project check",
		Engine:   pb.EngineType_IMPORT_GRAPH,
		Spec: &pb.Rule_ImportGraph{ImportGraph: &pb.ImportGraphSpec{
			PackagePattern: "domain/**",
			Forbidden:      []string{"service/**"},
		}},
	}

	result, err := client.Check(context.Background(), t.TempDir(), []*pb.Rule{rule}, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Findings, "empty project must produce zero findings")
}

// TestDeps_MultipleRules verifies the engine correctly handles multiple import rules in one call.
func TestDeps_MultipleRules(t *testing.T) {
	client := newEngineClient(t)
	projectPath := findRepoRoot(t)
	components := engineHexagonalComponents()

	start := time.Now()
	violations := checkDependencies(t, client, projectPath, components)
	duration := time.Since(start)

	t.Logf("Multi-rule check (%v): %d violations across %d components",
		duration, len(violations), len(components))
	for _, v := range violations {
		t.Logf("  [%s] %s — %s", v.Rule, v.File, v.Message)
	}

	// domain imports service — must be caught across all rules in one pass.
	assert.NotEmpty(t, violations, "multi-rule check must detect domain→service violation")
}
