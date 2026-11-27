package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/diktahq/verikt/internal/engineclient/pb"
)

const ruleWithoutSeverity = `
id: no-raw-sql
engine: grep
description: "No raw SQL"
pattern: 'db\.Exec\('
scope:
  - "**/*.go"
`

// A rule with no severity is documented to default to error. ValidateRule set
// that default on its own by-value copy, so the rule LoadRules returned still
// had an empty severity — and the engine path turned empty into info, which
// never fails the check.
func TestLoadRulesDefaultsAMissingSeverityToError(t *testing.T) {
	dir := setupTestProject(t, map[string]string{"a.go": "package a\n"})
	rulesDir := setupRulesDir(t, dir)
	writeRule(t, rulesDir, "sql.yaml", ruleWithoutSeverity)

	loaded, statuses, err := LoadRules(rulesDir, dir)
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, "error", loaded[0].Severity, "the rule that runs carries the default")
	require.Len(t, statuses, 1)
	assert.Equal(t, "error", statuses[0].Rule.Severity, "and so does the status the report shows")
}

// End to end on the default engine: the finding is an error, not info.
func TestRuleWithoutSeverityReportsErrorsOnTheEngine(t *testing.T) {
	client := testEngineClient(t)
	dir := setupTestProject(t, map[string]string{"a.go": "package a\n\nfunc f() { db.Exec(q) }\n"})
	rulesDir := setupRulesDir(t, dir)
	writeRule(t, rulesDir, "sql.yaml", ruleWithoutSeverity)

	result, err := RunRules(rulesDir, dir, nil, client)
	require.NoError(t, err)
	require.Len(t, result.Violations, 1)
	assert.Equal(t, "error", result.Violations[0].Severity)
}

// The Go-to-proto mapping fails closed: a severity it does not recognise is an
// error, never info. Info never fails the check, so defaulting to it turned any
// gap upstream into a silent pass.
func TestSeverityToProtoFailsClosed(t *testing.T) {
	assert.Equal(t, pb.Severity_ERROR, severityToProto("error"))
	assert.Equal(t, pb.Severity_WARNING, severityToProto("warning"))
	assert.Equal(t, pb.Severity_INFO, severityToProto("info"))
	assert.Equal(t, pb.Severity_ERROR, severityToProto(""))
	assert.Equal(t, pb.Severity_ERROR, severityToProto("Error"))
}

// The reverse mapping does the same for a finding with no severity on the wire.
func TestSeverityFromProtoFailsClosed(t *testing.T) {
	assert.Equal(t, "error", severityFromProto(pb.Severity_ERROR))
	assert.Equal(t, "warning", severityFromProto(pb.Severity_WARNING))
	assert.Equal(t, "info", severityFromProto(pb.Severity_INFO))
	assert.Equal(t, "error", severityFromProto(pb.Severity_SEVERITY_UNSPECIFIED))
}
