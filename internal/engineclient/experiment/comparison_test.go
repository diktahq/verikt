package experiment

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diktahq/verikt/internal/engineclient"
	pb "github.com/diktahq/verikt/internal/engineclient/pb"
	"github.com/diktahq/verikt/internal/rules"
)

// TestCompareGoVsRustGrep runs the same grep rules against verikt's own codebase
// through both the Go engine and the Rust engine, and compares results.
func TestCompareGoVsRustGrep(t *testing.T) {
	repoRoot := findRepoRoot(t)
	client := newTestClient(t)

	goRules := testRules()

	for _, goRule := range goRules {
		t.Run(goRule.ID, func(t *testing.T) {
			goStart := time.Now()
			goViolations, err := rules.RunGrep(goRule, repoRoot, nil)
			if err != nil {
				t.Fatalf("Go engine error: %v", err)
			}
			goDuration := time.Since(goStart)

			pbRule := convertRule(goRule)

			rustStart := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			result, err := client.Check(ctx, repoRoot, []*pb.Rule{pbRule}, nil)
			if err != nil {
				t.Fatalf("Rust engine error: %v", err)
			}
			rustDuration := time.Since(rustStart)

			t.Logf("Rule: %s", goRule.ID)
			t.Logf("  Go:   %d violations in %v", len(goViolations), goDuration)
			t.Logf("  Rust: %d violations in %v (engine: %.1fms)",
				len(result.Findings), rustDuration, result.Summary.DurationMs)

			if len(goViolations) > 0 || len(result.Findings) > 0 {
				t.Logf("  Go findings:")
				for _, v := range goViolations {
					t.Logf("    %s:%d %s", v.File, v.Line, truncateStr(v.Match, 80))
				}
				t.Logf("  Rust findings:")
				for _, f := range result.Findings {
					t.Logf("    %s:%d %s", f.File, f.Line, truncateStr(f.Match, 80))
				}
			}
		})
	}
}

// TestBenchmarkGoVsRust runs all rules together and reports aggregate timing.
func TestBenchmarkGoVsRust(t *testing.T) {
	repoRoot := findRepoRoot(t)
	client := newTestClient(t)

	goRules := testRules()
	pbRules := make([]*pb.Rule, 0, len(goRules))
	for _, r := range goRules {
		pbRules = append(pbRules, convertRule(r))
	}

	goStart := time.Now()
	totalGoViolations := 0
	for _, rule := range goRules {
		violations, err := rules.RunGrep(rule, repoRoot, nil)
		if err != nil {
			t.Fatalf("Go engine error on %s: %v", rule.ID, err)
		}
		totalGoViolations += len(violations)
	}
	goDuration := time.Since(goStart)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rustStart := time.Now()
	result, err := client.Check(ctx, repoRoot, pbRules, nil)
	if err != nil {
		t.Fatalf("Rust engine error: %v", err)
	}
	rustDuration := time.Since(rustStart)

	t.Logf("=== Benchmark: %d rules against verikt codebase ===", len(goRules))
	t.Logf("Go engine:   %d violations in %v", totalGoViolations, goDuration)
	t.Logf("Rust engine: %d violations in %v (engine internal: %.1fms)",
		len(result.Findings), rustDuration, result.Summary.DurationMs)
	t.Logf("Files checked (Rust): %d", result.Summary.FilesChecked)
	if rustDuration.Milliseconds() > 0 {
		t.Logf("Speedup: %.1fx", float64(goDuration.Milliseconds())/float64(rustDuration.Milliseconds()))
	}

	for _, s := range result.Summary.RuleStatuses {
		status := "VALID"
		switch s.Status {
		case pb.RuleStatus_STALE:
			status = "STALE"
		case pb.RuleStatus_INVALID:
			status = "INVALID"
		}
		t.Logf("  Rule %s: %s", s.RuleId, status)
	}
}

func newTestClient(t *testing.T) *engineclient.Client {
	t.Helper()
	path, err := engineclient.EnginePath()
	if err != nil {
		t.Skipf("engine binary not available: %v", err)
	}
	return engineclient.New(path)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

func testRules() []rules.Rule {
	return []rules.Rule{
		{
			ID:          "cap-sql-parameterized",
			Engine:      "grep",
			Description: "SQL queries must use parameterized queries, not string concatenation",
			Severity:    "error",
			Pattern:     `(fmt\.Sprintf|"\+.*SELECT|"\+.*INSERT|"\+.*UPDATE|"\+.*DELETE)`,
			Scope:       []string{"**/*.go"},
			Exclude:     []string{"*_test.go", "vendor/**"},
		},
		{
			ID:          "cap-handler-context",
			Engine:      "grep",
			Description: "HTTP handlers should use request context, not context.Background()",
			Severity:    "warning",
			Pattern:     `context\.Background\(\)`,
			Scope:       []string{"**/*.go"},
			Exclude:     []string{"*_test.go"},
		},
		{
			ID:             "cap-tracing-context",
			Engine:         "grep",
			Description:    "Functions making external calls should pass context for tracing",
			Severity:       "warning",
			Pattern:        `(http\.Get|http\.Post|sql\.Query|sql\.Exec)`,
			MustNotContain: "ctx",
			Scope:          []string{"**/*.go"},
			Exclude:        []string{"*_test.go", "vendor/**"},
		},
		{
			ID:          "no-os-exit",
			Engine:      "grep",
			Description: "Do not call os.Exit directly; use structured shutdown",
			Severity:    "warning",
			Pattern:     `os\.Exit\(`,
			Scope:       []string{"**/*.go"},
			Exclude:     []string{"*_test.go", "cmd/**/*.go"},
		},
		{
			ID:          "no-global-mutable-state",
			Engine:      "grep",
			Description: "Global mutable state is forbidden",
			Severity:    "error",
			Pattern:     `var\s+\w+\s*=\s*(map\[|&|\[\]\w)`,
			Scope:       []string{"**/*.go"},
			Exclude:     []string{"*_test.go", "vendor/**"},
		},
		{
			ID:          "no-fmt-print-in-lib",
			Engine:      "grep",
			Description: "Library code should not use fmt.Print; use structured logging",
			Severity:    "warning",
			Pattern:     `fmt\.Print(ln|f)?\(`,
			Scope:       []string{"internal/**/*.go"},
			Exclude:     []string{"*_test.go", "internal/cli/**"},
		},
	}
}

func convertRule(r rules.Rule) *pb.Rule {
	severity := pb.Severity_ERROR
	switch r.Severity {
	case "warning":
		severity = pb.Severity_WARNING
	case "info":
		severity = pb.Severity_INFO
	}

	return &pb.Rule{
		Id:       r.ID,
		Severity: severity,
		Message:  r.Description,
		Engine:   pb.EngineType_GREP,
		Scope: &pb.RuleScope{
			Include: r.Scope,
			Exclude: r.Exclude,
		},
		Spec: &pb.Rule_Grep{Grep: &pb.GrepSpec{
			Pattern:        r.Pattern,
			MustContain:    r.MustContain,
			MustNotContain: r.MustNotContain,
		}},
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
