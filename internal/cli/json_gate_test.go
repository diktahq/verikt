package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/rules"
)

// result must agree with the exit code for every category, because it is what a
// pipeline gates on.
//
// violations[] and anti_patterns[] carry built-in detector findings only. A gate
// built from those two arrays — which is what the docs published — returned true
// while `verikt check` exited 1 for an error-severity proxy-rule violation, for a
// stale rule, and for a decision-gate failure.
func TestJSONResultAgreesWithTheExitCodeForEveryCategory(t *testing.T) {
	cases := []struct {
		name    string
		outcome checkOutcome
	}{
		{
			name: "error-severity proxy rule violation",
			outcome: checkOutcome{
				Checker: &checker.CheckResult{},
				Rules: &rules.RunResult{
					Statuses:   []rules.RuleStatus{{Rule: rules.Rule{ID: "r"}, Status: "valid"}},
					Violations: []rules.RuleViolation{{RuleID: "r", Severity: "error", File: "a.go"}},
				},
			},
		},
		{
			name: "stale proxy rule",
			outcome: checkOutcome{
				Checker: &checker.CheckResult{},
				Rules: &rules.RunResult{
					Statuses: []rules.RuleStatus{
						{Rule: rules.Rule{ID: "r"}, Filename: "r.yaml", Status: "stale", Error: "scope matches 0 files"},
					},
				},
			},
		},
		{
			name: "invalid proxy rule",
			outcome: checkOutcome{
				Checker: &checker.CheckResult{},
				Rules: &rules.RunResult{
					Statuses: []rules.RuleStatus{
						{Rule: rules.Rule{ID: "r"}, Filename: "r.yaml", Status: "invalid", Error: "bad regex"},
					},
				},
			},
		},
		{
			name: "decision gate failure",
			outcome: checkOutcome{
				Checker:   &checker.CheckResult{},
				Decisions: []checker.DecisionViolation{{Severity: "error", Topic: "database"}},
			},
		},
		{
			name: "error-severity detector finding",
			outcome: checkOutcome{
				Checker: &checker.CheckResult{
					DependencyViolations: []checker.Violation{{Rule: "dependency", Severity: "error", File: "a.go"}},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			out := captureStdout(t, func() {
				err = reportCheck(tc.outcome, nil, &checkFlags{}, "json")
			})

			require.ErrorIs(t, err, ErrCheckFailed, "this outcome must fail the build")

			var doc struct {
				Result string `json:"result"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &doc))
			assert.Equal(t, "fail", doc.Result,
				`result must say fail whenever the exit code does, or a pipeline gating on it passes`)
		})
	}
}

// A clean outcome is a pass in both places.
func TestJSONResultIsPassWhenNothingBlocks(t *testing.T) {
	var err error
	out := captureStdout(t, func() {
		err = reportCheck(checkOutcome{
			Checker: &checker.CheckResult{
				FunctionViolations: []checker.Violation{{Rule: "max_lines", Severity: "warning", File: "a.go"}},
			},
			Rules: &rules.RunResult{Statuses: []rules.RuleStatus{{Rule: rules.Rule{ID: "r"}, Status: "valid"}}},
		}, nil, &checkFlags{}, "json")
	})

	require.NoError(t, err, "warnings do not fail the build")

	var doc struct {
		Result string `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &doc))
	assert.Equal(t, "pass", doc.Result)
}

// severityGate matches a jq expression that decides pass/fail by selecting on
// severity — `jq -e` with a `select(.severity ...)` and a `length == 0`.
var severityGate = regexp.MustCompile(`jq -e[^\n]*select\(\s*\.severity[^\n]*length == 0`)

// No document may publish a gate that decides pass/fail from a subset of the
// categories.
//
// The published snippet selected error severity across `violations[]` and
// `anti_patterns[]` only, so it returned true — pass — on runs that verikt failed
// because a proxy rule fired, went stale, or a decision gate failed. It appeared
// in the CLI reference, the CI guide and the upgrade notes, in the CI guide three
// paragraphs below the sentence explaining that stale rules fail the check.
//
// Gating belongs on `.result`, which is the verdict the exit code carries.
func TestDocsDoNotPublishASeverityBasedGate(t *testing.T) {
	root := repoRootFromTest(t)

	var checked int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "target", "experiments", "dist", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".md", ".mdx", ".yml", ".yaml", ".toml":
		default:
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || strings.HasPrefix(filepath.ToSlash(rel), "docs/internal/") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		checked++

		for i, line := range strings.Split(string(data), "\n") {
			if severityGate.MatchString(line) {
				t.Errorf("%s:%d publishes a gate that decides pass/fail from a subset of categories; gate on .result instead:\n  %s",
					filepath.ToSlash(rel), i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.NotZero(t, checked, "no documents scanned; this test guards nothing")
}

// repoRootFromTest walks up to the module root.
func repoRootFromTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "go.mod not found above the test directory")
		dir = parent
	}
}
