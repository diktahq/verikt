package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SeverityFails is the one pass/fail rule for every finding `verikt check`
// reports. Checker findings, proxy rules and decision gates used to answer
// "does this fail?" three different ways.
func TestSeverityFails(t *testing.T) {
	cases := []struct {
		severity string
		failOn   string
		want     bool
	}{
		{"error", FailOnError, true},
		{"error", FailOnWarning, true},
		{"warning", FailOnError, false},
		{"warning", "", false},
		{"warning", FailOnWarning, true},
		{"info", FailOnError, false},
		{"info", FailOnWarning, false},
		{"ignore", FailOnWarning, false},
		// A finding with no severity, or one this version does not know, fails:
		// a detector that forgets to set one must not pass silently.
		{"", FailOnError, true},
		{"critical", FailOnError, true},
	}
	for _, c := range cases {
		t.Run(c.severity+"@"+c.failOn, func(t *testing.T) {
			assert.Equal(t, c.want, SeverityFails(c.severity, c.failOn))
		})
	}
}

func TestNormalizeSeverity_EmptyAndUnknownBecomeError(t *testing.T) {
	assert.Equal(t, "error", NormalizeSeverity(""))
	assert.Equal(t, "error", NormalizeSeverity("fatal"))
	assert.Equal(t, "warning", NormalizeSeverity("warning"))
	assert.Equal(t, "info", NormalizeSeverity("info"))
	assert.Equal(t, "ignore", NormalizeSeverity("ignore"))
}

func TestValidateFailOn_AcceptsErrorWarningAndUnset(t *testing.T) {
	for _, v := range []string{"", "error", "warning"} {
		assert.NoError(t, ValidateFailOn(v), "fail_on %q", v)
	}
}

// fail_on is a severity threshold, not an override, so the severity_overrides
// vocabulary (must/should) is not accepted here, and neither is info: an info
// finding never fails.
func TestValidateFailOn_RejectsEverythingElse(t *testing.T) {
	for _, v := range []string{"info", "should", "must", "Warning"} {
		err := ValidateFailOn(v)
		require.Error(t, err, "fail_on %q", v)
		assert.Contains(t, err.Error(), "check.fail_on")
	}
}

func TestValidateVeriktYAML_RejectsInvalidFailOn(t *testing.T) {
	cfg := &VeriktConfig{Language: "go", Architecture: "flat", Check: CheckConfig{FailOn: "info"}}
	errs := ValidateVeriktYAML(cfg)

	var found bool
	for _, err := range errs {
		if strings.Contains(err.Error(), "check.fail_on") {
			found = true
		}
	}
	assert.True(t, found, "an invalid check.fail_on must be reported, got %v", errs)
}

func TestLoadVeriktYAML_ReadsFailOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verikt.yaml")
	require.NoError(t, os.WriteFile(path,
		[]byte("language: go\narchitecture: flat\ncheck:\n  fail_on: warning\n"), 0o644))

	cfg, err := LoadVeriktYAML(path)
	require.NoError(t, err)
	assert.Equal(t, FailOnWarning, cfg.Check.FailOn)
}
