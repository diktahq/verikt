package analyzer

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAnalyzerAnalyzeHexagonal(t *testing.T) {
	a := New(filepath.Join("testdata", "hexagonal"))
	if err := a.LoadPackages(""); err != nil {
		t.Fatalf("LoadPackages() error = %v", err)
	}
	result, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Language != "go" {
		t.Fatalf("Language = %q, want go", result.Language)
	}
	if result.PackageCount == 0 {
		t.Fatal("expected packages")
	}
	if result.Architecture.Confidence <= 0 {
		t.Fatalf("expected architecture confidence > 0, got %v", result.Architecture.Confidence)
	}
}

// Packages no component claims have no dependency rules to enforce. check
// reports them as orphan_package errors; analyze used to say nothing at all, so
// the two disagreed about what was actually being enforced.
func TestAnalyzerReportsUnclaimedPackages(t *testing.T) {
	a := New(filepath.Join("testdata", "orphan"))
	if err := a.LoadPackages(""); err != nil {
		t.Fatalf("LoadPackages() error = %v", err)
	}
	result, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	var orphans []string
	for _, v := range result.Violations {
		if v.Rule == "orphan_package" {
			orphans = append(orphans, v.Source)
			if v.Severity != "error" {
				t.Errorf("orphan_package severity = %q, want error (matching check)", v.Severity)
			}
		}
	}

	want := []string{"example.com/orphan/helpers"}
	if len(orphans) != 1 || orphans[0] != want[0] {
		t.Errorf("orphan packages = %v, want %v", orphans, want)
	}
}

// A project whose components claim every package reports no orphans.
func TestAnalyzerNoUnclaimedPackagesWhenFullyMapped(t *testing.T) {
	a := New(filepath.Join("testdata", "hexagonal"))
	if err := a.LoadPackages(""); err != nil {
		t.Fatalf("LoadPackages() error = %v", err)
	}
	result, err := a.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	// The fixture's own verikt.yaml claims every one of its packages, so a fully
	// mapped project must produce no orphan findings.
	for _, v := range result.Violations {
		if v.Rule == "orphan_package" {
			t.Errorf("unexpected orphan_package with no verikt.yaml: %+v", v)
		}
	}
}

func TestAnalyzerInvalidPath(t *testing.T) {
	a := New(filepath.Join("testdata", "missing"))
	if err := a.LoadPackages(""); err == nil {
		t.Fatal("expected error")
	}
}
