package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/provider"
)

func sampleResult() *provider.AnalyzeResponse {
	return &provider.AnalyzeResponse{
		Language:      "go",
		PackageCount:  5,
		FileCount:     12,
		FunctionCount: 20,
		Architecture: provider.ArchitectureResult{
			Pattern:    "hexagonal",
			Confidence: 0.89,
			Evidence:   []string{"found domain"},
		},
		Framework: provider.FrameworkResult{Name: "chi", Confidence: 0.95},
		Conventions: provider.ConventionResults{
			ErrorHandling: provider.ConventionFinding{Pattern: "wrapped"},
			Logging:       provider.ConventionFinding{Pattern: "slog/structured"},
			Config:        provider.ConventionFinding{Pattern: "koanf"},
			Testing:       provider.TestingFinding{Pattern: "table-driven", TestFiles: 3, TotalGoFiles: 12},
		},
	}
}

// Not every violation is about an edge between two packages: orphan_package
// names a single package. Printing "(pkg -> )" with an empty arrow target
// implies a dependency that does not exist.
func TestTerminalFormatterOmitsEmptyViolationTarget(t *testing.T) {
	result := sampleResult()
	result.Violations = []provider.Violation{
		{Rule: "orphan_package", Message: "package matches no declared component", Source: "example.com/app/helpers", Severity: "error"},
		{Rule: "dependency", Message: "domain must not depend on adapters", Source: "example.com/app/domain", Target: "example.com/app/adapter", Severity: "error"},
	}

	terminal, err := NewFormatter("terminal", true)
	if err != nil {
		t.Fatalf("NewFormatter terminal: %v", err)
	}
	out, err := terminal.Format(result)
	if err != nil {
		t.Fatalf("terminal format: %v", err)
	}

	if strings.Contains(out, "-> )") {
		t.Errorf("output renders an empty arrow target:\n%s", out)
	}
	if !strings.Contains(out, "(example.com/app/helpers)") {
		t.Errorf("expected the source alone for a single-package violation:\n%s", out)
	}
	if !strings.Contains(out, "(example.com/app/domain -> example.com/app/adapter)") {
		t.Errorf("expected both endpoints for a dependency violation:\n%s", out)
	}
}

func TestFormatters(t *testing.T) {
	result := sampleResult()

	terminal, err := NewFormatter("terminal", true)
	if err != nil {
		t.Fatalf("NewFormatter terminal: %v", err)
	}
	out, err := terminal.Format(result)
	if err != nil {
		t.Fatalf("terminal format: %v", err)
	}
	if !strings.Contains(out, "Project Summary") {
		t.Fatalf("terminal output missing section: %s", out)
	}

	jsonFmt, err := NewFormatter("json", true)
	if err != nil {
		t.Fatalf("NewFormatter json: %v", err)
	}
	jsonOut, err := jsonFmt.Format(result)
	if err != nil {
		t.Fatalf("json format: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	mdFmt, err := NewFormatter("markdown", true)
	if err != nil {
		t.Fatalf("NewFormatter markdown: %v", err)
	}
	mdOut, err := mdFmt.Format(result)
	if err != nil {
		t.Fatalf("markdown format: %v", err)
	}
	if !strings.Contains(mdOut, "## Architecture") {
		t.Fatalf("markdown output missing architecture section: %s", mdOut)
	}
}
