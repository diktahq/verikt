package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A false-positive report must carry the code, or it cannot become a test.
//
// A user who hits a false positive otherwise chooses between waiving it — which
// hides the defect and leaves it in place for everyone with the same shape of
// code — and rewriting correct code to avoid a pattern. One project reworded
// English error messages to stop a detector firing. The report is the third
// option, and it is only useful if it contains enough to reproduce.
func TestFalsePositiveReportCarriesTheOffendingCode(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "tables.go")
	require.NoError(t, os.WriteFile(file, []byte(
		"package adapter\n\nfunc Check(where string) string {\n\treturn where + \": delete the block\"\n}\n"), 0o644))

	report, err := buildFalsePositiveReport("sql_concatenation", file, 4, "no database in this module")
	require.NoError(t, err)

	assert.Contains(t, report, "sql_concatenation")
	assert.Contains(t, report, "no database in this module")
	assert.Contains(t, report, "delete the block", "the code that fired must be in the report")
	assert.Contains(t, report, "> ", "the offending line must be marked")
	assert.Contains(t, report, "INV-005")
}

// A missing file is reported, not fatal — the finding is still worth sending.
func TestFalsePositiveReportSurvivesAMissingFile(t *testing.T) {
	report, err := buildFalsePositiveReport("god_package", "does/not/exist.go", 0, "")
	require.NoError(t, err)
	assert.Contains(t, report, "Could not read")
	assert.Contains(t, report, "god_package")
}

// Without a reason, the report asks for one rather than shipping a blank.
func TestFalsePositiveReportPromptsForTheReason(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.go")
	require.NoError(t, os.WriteFile(file, []byte("package x\n"), 0o644))

	report, err := buildFalsePositiveReport("god_package", file, 1, "")
	require.NoError(t, err)
	assert.True(t, strings.Contains(report, "describe what makes the risk unreachable"),
		"an empty reason should prompt, not ship blank: %s", report)
}
