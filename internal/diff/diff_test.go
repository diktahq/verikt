package diff

import (
	"os"
	"testing"

	"github.com/dcsg/archway/internal/provider"
	"github.com/dcsg/archway/internal/scaffold"

	_ "github.com/dcsg/archway/providers/golang"
)

func composePlan(t *testing.T, arch string, caps []string) (*scaffold.CompositionPlan, provider.LanguageProvider) {
	t.Helper()
	p, err := provider.Get("go")
	if err != nil {
		t.Fatalf("get go provider: %v", err)
	}
	tFS := p.GetTemplateFS()
	vars := map[string]interface{}{
		"ServiceName": "test-svc",
		"GoModule":    "example.com/test-svc",
		"ModulePath":  "example.com/test-svc",
	}
	plan, err := scaffold.ComposeProject(tFS, arch, caps, vars)
	if err != nil {
		t.Fatalf("compose project: %v", err)
	}
	return plan, p
}

func renderToDir(t *testing.T, plan *scaffold.CompositionPlan, p provider.LanguageProvider) string {
	t.Helper()
	tFS := p.GetTemplateFS()
	tmpDir := t.TempDir()
	renderer := scaffold.NewRenderer(tFS)
	if _, err := renderer.RenderComposition(plan, tmpDir); err != nil {
		t.Fatalf("render composition: %v", err)
	}
	return tmpDir
}

func TestDiff_AllFilesPresent(t *testing.T) {
	plan, p := composePlan(t, "hexagonal", []string{"http-api"})
	tFS := p.GetTemplateFS()
	tmpDir := renderToDir(t, plan, p)

	result, err := Run(tFS, plan, tmpDir)
	if err != nil {
		t.Fatalf("diff run: %v", err)
	}

	if result.Summary.DriftScore != 0.0 {
		t.Errorf("expected drift score 0.0, got %.2f", result.Summary.DriftScore)
	}

	for _, d := range result.CapabilityDiffs {
		if d.Status != "ok" {
			t.Errorf("capability %q: expected status ok, got %q (missing: %v)", d.Name, d.Status, d.MissingFiles)
		}
	}
}

func TestDiff_MissingCapabilityFiles(t *testing.T) {
	plan, p := composePlan(t, "hexagonal", []string{"http-api"})
	tFS := p.GetTemplateFS()
	tmpDir := renderToDir(t, plan, p)

	// First run to discover present files, then delete one.
	baseline, err := Run(tFS, plan, tmpDir)
	if err != nil {
		t.Fatalf("baseline diff: %v", err)
	}

	removedAny := false
	for _, d := range baseline.CapabilityDiffs {
		if removedAny {
			break
		}
		if d.Name == plan.Architecture+" (arch)" {
			continue
		}
		for _, f := range d.PresentFiles {
			if err := os.Remove(tmpDir + "/" + f); err == nil {
				removedAny = true
				break
			}
		}
	}
	if !removedAny {
		t.Skip("no capability files to remove")
	}

	result, err := Run(tFS, plan, tmpDir)
	if err != nil {
		t.Fatalf("diff run: %v", err)
	}

	if result.Summary.DriftScore == 0.0 {
		t.Error("expected non-zero drift score after deleting files")
	}

	foundDrift := false
	for _, d := range result.CapabilityDiffs {
		if d.Status == "partial" || d.Status == "missing" {
			foundDrift = true
		}
	}
	if !foundDrift {
		t.Error("expected at least one capability with partial or missing status")
	}
}

func TestDiff_NoCapabilities(t *testing.T) {
	plan, p := composePlan(t, "hexagonal", nil)
	tFS := p.GetTemplateFS()
	tmpDir := renderToDir(t, plan, p)

	result, err := Run(tFS, plan, tmpDir)
	if err != nil {
		t.Fatalf("diff run: %v", err)
	}

	if result.Summary.DriftScore != 0.0 {
		t.Errorf("expected drift score 0.0 with no capabilities, got %.2f", result.Summary.DriftScore)
	}

	for _, d := range result.CapabilityDiffs {
		if d.Status != "ok" {
			t.Errorf("capability %q: expected status ok, got %q", d.Name, d.Status)
		}
	}
}

func TestComputeSummary(t *testing.T) {
	tests := []struct {
		name      string
		diffs     []CapabilityDiff
		wantFP    int
		wantPP    int
		wantFM    int
		wantDrift float64
	}{
		{
			name:      "all ok",
			diffs:     []CapabilityDiff{{Status: "ok", PresentFiles: []string{"a", "b"}}},
			wantFP:    1,
			wantDrift: 0.0,
		},
		{
			name: "one missing",
			diffs: []CapabilityDiff{
				{Status: "missing", MissingFiles: []string{"a", "b"}},
			},
			wantFM:    1,
			wantDrift: 1.0,
		},
		{
			name: "partial drift",
			diffs: []CapabilityDiff{
				{Status: "partial", PresentFiles: []string{"a"}, MissingFiles: []string{"b"}},
			},
			wantPP:    1,
			wantDrift: 0.5,
		},
		{
			name:      "empty diffs",
			diffs:     nil,
			wantDrift: 0.0,
		},
		{
			name: "mixed statuses",
			diffs: []CapabilityDiff{
				{Status: "ok", PresentFiles: []string{"a", "b"}},
				{Status: "partial", PresentFiles: []string{"c"}, MissingFiles: []string{"d"}},
				{Status: "missing", MissingFiles: []string{"e", "f"}},
			},
			wantFP:    1,
			wantPP:    1,
			wantFM:    1,
			wantDrift: 0.5, // 3 missing out of 6 total
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := computeSummary(tt.diffs)
			if s.FullyPresent != tt.wantFP {
				t.Errorf("FullyPresent: got %d, want %d", s.FullyPresent, tt.wantFP)
			}
			if s.PartiallyPresent != tt.wantPP {
				t.Errorf("PartiallyPresent: got %d, want %d", s.PartiallyPresent, tt.wantPP)
			}
			if s.FullyMissing != tt.wantFM {
				t.Errorf("FullyMissing: got %d, want %d", s.FullyMissing, tt.wantFM)
			}
			if s.DriftScore != tt.wantDrift {
				t.Errorf("DriftScore: got %.2f, want %.2f", s.DriftScore, tt.wantDrift)
			}
		})
	}
}
