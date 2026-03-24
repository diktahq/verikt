package engineclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/diktahq/verikt/internal/engineclient/pb"
)

// TestTypeScriptImportGraph verifies that the engine detects a forbidden import
// from domain/ into infrastructure/ in a TypeScript project.
func TestTypeScriptImportGraph(t *testing.T) {
	dir := t.TempDir()
	domainDir := filepath.Join(dir, "src", "domain")
	infraDir := filepath.Join(dir, "src", "infrastructure", "database")
	if err := os.MkdirAll(domainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(infraDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Domain file with a forbidden import from infrastructure.
	if err := os.WriteFile(filepath.Join(domainDir, "index.ts"), []byte(
		"export {}\nimport { db } from '../infrastructure/database/client.js'\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	// Target file (doesn't need to exist for import extraction, but create it anyway).
	if err := os.WriteFile(filepath.Join(infraDir, "client.ts"), []byte(
		"export const db = null\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	client := newTestClient(t)

	rules := []*pb.Rule{{
		Id:       "arch/domain",
		Severity: pb.Severity_ERROR,
		Message:  "domain must not import from infrastructure",
		Engine:   pb.EngineType_IMPORT_GRAPH,
		Scope:    &pb.RuleScope{Language: "typescript"},
		Spec: &pb.Rule_ImportGraph{
			ImportGraph: &pb.ImportGraphSpec{
				PackagePattern: "src/domain/**",
				Forbidden:      []string{"src/infrastructure/**"},
			},
		},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := client.Check(ctx, dir, rules, nil)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}

	if len(result.Findings) == 0 {
		t.Error("expected at least one finding for domain→infrastructure violation, got none")
		return
	}

	t.Logf("found %d violation(s)", len(result.Findings))
	for _, f := range result.Findings {
		t.Logf("  rule=%s file=%s msg=%s", f.RuleId, f.File, f.Message)
	}
}

// TestTypeScriptImportGraph_Clean verifies that a compliant project produces no findings.
func TestTypeScriptImportGraph_Clean(t *testing.T) {
	dir := t.TempDir()
	domainDir := filepath.Join(dir, "src", "domain")
	if err := os.MkdirAll(domainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No forbidden imports.
	if err := os.WriteFile(filepath.Join(domainDir, "index.ts"), []byte(
		"export const greeting = 'hello'\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	client := newTestClient(t)

	rules := []*pb.Rule{{
		Id:       "arch/domain",
		Severity: pb.Severity_ERROR,
		Message:  "domain must not import from infrastructure",
		Engine:   pb.EngineType_IMPORT_GRAPH,
		Scope:    &pb.RuleScope{Language: "typescript"},
		Spec: &pb.Rule_ImportGraph{
			ImportGraph: &pb.ImportGraphSpec{
				PackagePattern: "src/domain/**",
				Forbidden:      []string{"src/infrastructure/**"},
			},
		},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := client.Check(ctx, dir, rules, nil)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}

	if len(result.Findings) != 0 {
		t.Errorf("expected no findings, got %d: %+v", len(result.Findings), result.Findings)
	}
}
