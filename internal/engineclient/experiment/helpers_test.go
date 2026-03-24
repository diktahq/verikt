package experiment

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/engineclient"
	pb "github.com/diktahq/verikt/internal/engineclient/pb"
)

// newEngineClient returns a client backed by the embedded engine binary,
// or skips the test if the binary is unavailable.
func newEngineClient(t *testing.T) *engineclient.Client {
	t.Helper()
	path, err := engineclient.EnginePath()
	if err != nil {
		t.Skipf("engine binary not available: %v", err)
	}
	return engineclient.New(path)
}

// checkerTestdataDir returns the path to internal/checker/testdata.
func checkerTestdataDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	// helpers_test.go is at internal/engineclient/experiment/
	// testdata is at internal/checker/testdata/
	return filepath.Join(filepath.Dir(filename), "..", "..", "checker", "testdata")
}

// checkFunctionMetrics calls the engine for function metric violations.
func checkFunctionMetrics(t *testing.T, client *engineclient.Client, projectPath string, rules config.FunctionRules) []checker.Violation {
	t.Helper()
	if rules.MaxLines == 0 && rules.MaxParams == 0 && rules.MaxReturnValues == 0 {
		return nil
	}
	rule := &pb.Rule{
		Id:       "function-metrics",
		Severity: pb.Severity_WARNING,
		Message:  "function metric violation",
		Engine:   pb.EngineType_METRIC,
		Spec: &pb.Rule_FunctionMetric{
			FunctionMetric: &pb.FunctionMetricSpec{
				MaxLines:   int32(rules.MaxLines),
				MaxParams:  int32(rules.MaxParams),
				MaxReturns: int32(rules.MaxReturnValues),
			},
		},
	}
	result, err := client.Check(context.Background(), projectPath, []*pb.Rule{rule}, nil)
	if err != nil {
		t.Fatalf("engine CheckFunctionMetrics: %v", err)
	}
	out := make([]checker.Violation, 0, len(result.Findings))
	for _, f := range result.Findings {
		out = append(out, checker.Violation{
			Category: "function",
			File:     f.File,
			Line:     int(f.Line),
			Message:  f.Message,
			Rule:     f.Match,
			Severity: "warning",
		})
	}
	return out
}

// checkDependencies calls the engine import graph for component dependency violations.
func checkDependencies(t *testing.T, client *engineclient.Client, projectPath string, components []config.Component) []checker.Violation {
	t.Helper()
	rules := componentsToImportRules(components)
	if len(rules) == 0 {
		return nil
	}
	result, err := client.Check(context.Background(), projectPath, rules, nil)
	if err != nil {
		t.Fatalf("engine CheckDependencies: %v", err)
	}
	out := make([]checker.Violation, 0, len(result.Findings))
	for _, f := range result.Findings {
		out = append(out, checker.Violation{
			Category: "dependency",
			File:     f.File,
			Line:     int(f.Line),
			Message:  f.Message,
			Rule:     f.RuleId,
			Severity: "error",
		})
	}
	return out
}

// componentsToImportRules converts verikt components to ImportGraphSpec rules.
func componentsToImportRules(components []config.Component) []*pb.Rule {
	var rules []*pb.Rule
	for _, comp := range components {
		allowedNames := map[string]bool{}
		for _, dep := range comp.MayDependOn {
			allowedNames[dep] = true
		}
		var forbidden []string
		for _, other := range components {
			if other.Name == comp.Name || allowedNames[other.Name] {
				continue
			}
			for _, p := range other.In {
				// Add both the glob pattern and the bare directory so that
				// single-level imports (e.g. "service") match "service/**".
				forbidden = append(forbidden, p)
				if dir := strings.TrimSuffix(p, "/**"); dir != p {
					forbidden = append(forbidden, dir)
				}
			}
		}
		for _, pkgPattern := range comp.In {
			rules = append(rules, &pb.Rule{
				Id:       "arch/" + comp.Name,
				Severity: pb.Severity_ERROR,
				Message:  comp.Name + " has a forbidden dependency",
				Engine:   pb.EngineType_IMPORT_GRAPH,
				Spec: &pb.Rule_ImportGraph{ImportGraph: &pb.ImportGraphSpec{
					PackagePattern: pkgPattern,
					Forbidden:      forbidden,
				}},
			})
		}
	}
	return rules
}

// --- Adapters bridging engineclient.Client to checker interfaces ---

type apAdapter struct{ c *engineclient.Client }

func (a *apAdapter) CheckAntiPatterns(projectPath string, detectors []string) ([]checker.AntiPattern, error) {
	findings, err := a.c.CheckAntiPatterns(projectPath, detectors)
	if err != nil {
		return nil, err
	}
	out := make([]checker.AntiPattern, len(findings))
	for i, f := range findings {
		out[i] = checker.AntiPattern{
			Name:     f.Name,
			File:     f.File,
			Line:     f.Line,
			Message:  f.Message,
			Severity: f.Severity,
		}
	}
	return out, nil
}

type depAdapter struct{ c *engineclient.Client }

func (d *depAdapter) CheckDependencies(projectPath string, components []config.Component) ([]checker.Violation, error) {
	rules := componentsToImportRules(components)
	if len(rules) == 0 {
		return nil, nil
	}
	result, err := d.c.Check(context.Background(), projectPath, rules, nil)
	if err != nil {
		return nil, err
	}
	out := make([]checker.Violation, 0, len(result.Findings))
	for _, f := range result.Findings {
		out = append(out, checker.Violation{
			Category: "dependency",
			File:     f.File,
			Line:     int(f.Line),
			Message:  f.Message,
			Rule:     f.RuleId,
			Severity: "error",
		})
	}
	return out, nil
}

type metricAdapter struct{ c *engineclient.Client }

func (m *metricAdapter) CheckFunctionMetrics(projectPath string, rules config.FunctionRules) ([]checker.Violation, error) {
	if rules.MaxLines == 0 && rules.MaxParams == 0 && rules.MaxReturnValues == 0 {
		return nil, nil
	}
	rule := &pb.Rule{
		Id:       "function-metrics",
		Severity: pb.Severity_WARNING,
		Message:  "function metric violation",
		Engine:   pb.EngineType_METRIC,
		Spec: &pb.Rule_FunctionMetric{
			FunctionMetric: &pb.FunctionMetricSpec{
				MaxLines:   int32(rules.MaxLines),
				MaxParams:  int32(rules.MaxParams),
				MaxReturns: int32(rules.MaxReturnValues),
			},
		},
	}
	result, err := m.c.Check(context.Background(), projectPath, []*pb.Rule{rule}, nil)
	if err != nil {
		return nil, err
	}
	out := make([]checker.Violation, 0, len(result.Findings))
	for _, f := range result.Findings {
		out = append(out, checker.Violation{
			Category: "function",
			File:     f.File,
			Line:     int(f.Line),
			Message:  f.Message,
			Rule:     f.Match,
			Severity: "warning",
		})
	}
	return out, nil
}
