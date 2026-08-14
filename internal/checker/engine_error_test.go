package checker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/config"
)

var errEngineExploded = errors.New("engine crashed")

type failingDependencyClient struct{}

func (failingDependencyClient) CheckDependencies(string, []config.Component) ([]Violation, error) {
	return nil, errEngineExploded
}

type stubAntiPatternClient struct{ findings []AntiPattern }

func (c stubAntiPatternClient) CheckAntiPatterns(string, []string) ([]AntiPattern, error) {
	return c.findings, nil
}

type stubDependencyClient struct{ findings []Violation }

func (c stubDependencyClient) CheckDependencies(string, []config.Component) ([]Violation, error) {
	return c.findings, nil
}

type stubMetricClient struct{ findings []Violation }

func (c stubMetricClient) CheckFunctionMetrics(string, config.FunctionRules) ([]Violation, error) {
	return c.findings, nil
}

func antiPatternClientReturning(findings []AntiPattern) AntiPatternClient {
	return stubAntiPatternClient{findings: findings}
}

func dependencyClientReturning(findings []Violation) DependencyClient {
	return stubDependencyClient{findings: findings}
}

func metricClientReturning(findings []Violation) MetricClient {
	return stubMetricClient{findings: findings}
}

var (
	emptyAntiPatternClient = stubAntiPatternClient{}
	emptyMetricClient      = stubMetricClient{}
)

func typescriptProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "domain"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "src", "domain", "user.ts"),
		[]byte("export const a = 1;\n"), 0o644))
	return dir
}

// An engine that could not run has not reported "no violations" — it has
// reported nothing. The Go path returns the error; the TypeScript path discarded
// it and returned a passing result, so on the one language where the engine is
// not mandatory a crashed, missing or timed-out engine read as a clean check.
//
// This is the failure mode ErrEngineRequired exists to prevent, in the branch
// that had no test.
func TestCheckTypeScriptReportsEngineFailure(t *testing.T) {
	cfg := &config.VeriktConfig{
		Language:     "typescript",
		Architecture: "hexagonal",
		Components: []config.Component{
			{Name: "domain", In: []string{"src/domain"}},
		},
	}

	result, err := CheckWithEngine(cfg, typescriptProject(t),
		emptyAntiPatternClient, failingDependencyClient{}, emptyMetricClient)

	require.Error(t, err, "a dependency check that could not run must not be reported as a pass")
	assert.ErrorIs(t, err, errEngineExploded)
	assert.NotNil(t, result, "partial results are still returned for reporting")
}

// Excludes must survive the partial-result path too: a caller that renders
// partial results would otherwise see findings the configuration says to ignore,
// and the ratchet counts them.
func TestCheckWithEngineOnlyAppliesExcludesToPartialResults(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "gen"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "gen", "generated.go"), []byte("package gen\n"), 0o644))

	cfg := &config.VeriktConfig{
		Language:     "go",
		Architecture: "hexagonal",
		Components:   []config.Component{{Name: "gen", In: []string{"gen"}}},
		Check:        config.CheckConfig{Exclude: []string{"gen/**"}},
	}

	result, err := CheckWithEngine(cfg, dir,
		emptyAntiPatternClient, failingDependencyClient{}, emptyMetricClient)

	require.Error(t, err)
	require.NotNil(t, result)
	for _, v := range result.StructureViolations {
		assert.NotContains(t, v.File, "gen/", "excluded path leaked into partial results")
	}
}
