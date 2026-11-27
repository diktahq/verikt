package rules

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/engineclient"
)

// testEngineClient returns a client for the engine under test:
// VERIKT_ENGINE_BIN when set (a freshly built engine), else the embedded one.
// It skips the test on a platform with neither.
func testEngineClient(t *testing.T) *engineclient.Client {
	t.Helper()
	if p := os.Getenv("VERIKT_ENGINE_BIN"); p != "" {
		return engineclient.New(p)
	}
	enginePath, err := engineclient.EnginePath()
	if err != nil {
		t.Skipf("embedded engine unavailable on this platform: %v", err)
	}
	return engineclient.New(enginePath)
}

// findingKeys runs one rule and returns each finding as "file:line:match".
func findingKeys(t *testing.T, files map[string]string, rule string, client *engineclient.Client) []string {
	t.Helper()

	dir := setupTestProject(t, files)
	rulesDir := setupRulesDir(t, dir)
	writeRule(t, rulesDir, "rule.yaml", rule)

	result, err := RunRules(rulesDir, dir, nil, client)
	require.NoError(t, err)
	for _, s := range result.Statuses {
		require.Equal(t, "valid", s.Status, "rule %s: %s", s.Rule.ID, s.Error)
	}

	keys := make([]string, 0, len(result.Violations))
	for _, v := range result.Violations {
		keys = append(keys, fmt.Sprintf("%s:%d:%s", v.File, v.Line, v.Match))
	}
	sort.Strings(keys)
	return keys
}

// The engine inverted must-contain, must-not-contain and file-must-contain: it
// skipped the violating line or file and reported the compliant one. The parity
// test only covered bare `pattern:` rules, so it never saw it — and verikt's own
// generated rules (cap-kafka-error-handling, cap-tracing-context, cap-grpc-proto,
// cap-auth-check) use exactly these fields.
//
// Each case states the expected findings, so the Go side is pinned too, and then
// requires the engine to report the same file, line and match text.
func TestGrepSpecFieldParityBetweenGoAndEngine(t *testing.T) {
	client := testEngineClient(t)

	cases := []struct {
		name  string
		files map[string]string
		rule  string
		want  []string
	}{
		{
			name: "must-contain reports the line that lacks the required text",
			files: map[string]string{
				"k.go": "package k\n\nfunc bad() { msg, _ := reader.ReadMessage(ctx) }\nfunc good() { msg, err := reader.ReadMessage(ctx) }\n",
			},
			rule: `
id: kafka-err
engine: grep
severity: error
pattern: 'ReadMessage\('
must-contain: 'err :?='
scope: ["**/*.go"]
`,
			want: []string{"k.go:3:func bad() { msg, _ := reader.ReadMessage(ctx) }"},
		},
		{
			name: "must-not-contain reports the line that has the forbidden text",
			files: map[string]string{
				"k.go": "package k\n\nfunc bad() { msg, _ := reader.ReadMessage(ctx) }\nfunc good() { msg, err := reader.ReadMessage(ctx) }\n",
			},
			rule: `
id: kafka-err
engine: grep
severity: error
pattern: 'ReadMessage\('
must-not-contain: ', _ :='
scope: ["**/*.go"]
`,
			want: []string{"k.go:3:func bad() { msg, _ := reader.ReadMessage(ctx) }"},
		},
		{
			name: "file-must-contain reports the file that lacks the text, and only that",
			files: map[string]string{
				"proto/ok.proto":      "syntax = \"proto3\";\npackage y;\n",
				"proto/missing.proto": "package z;\n",
			},
			rule: `
id: proto3
engine: grep
severity: error
file-must-contain: 'syntax = "proto3"'
scope: ["**/*.proto"]
`,
			want: []string{`proto/missing.proto:0:file does not contain pattern: syntax = "proto3"`},
		},
		{
			name: "file-must-contain does not stop the line check in the same file",
			files: map[string]string{
				"a.go": "package a\n\nvar _ = Sprintf\n",
			},
			rule: `
id: both
engine: grep
severity: error
pattern: 'Sprintf'
file-must-contain: 'MUST-BE-HERE'
scope: ["**/*.go"]
`,
			want: []string{"a.go:0:file does not contain pattern: MUST-BE-HERE", "a.go:3:var _ = Sprintf"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goKeys := findingKeys(t, tc.files, tc.rule, nil)
			engineKeys := findingKeys(t, tc.files, tc.rule, client)

			assert.Equal(t, tc.want, goKeys, "Go path findings")
			assert.Equal(t, goKeys, engineKeys, "the engine must report what the Go path reports")
		})
	}
}

// The match text is the same on both paths: trimmed, and cut at 120 bytes on a
// UTF-8 boundary. The engine echoed the whole line, however long, and the Go
// side cut mid-rune, which JSON then turned into a replacement character.
func TestGrepMatchTextParityBetweenGoAndEngine(t *testing.T) {
	client := testEngineClient(t)

	// 119 ASCII bytes then "é" (2 bytes): byte 120 falls inside the rune.
	long := "\t\tSprintf" + strings.Repeat("x", 119-len("Sprintf")) + "é" + strings.Repeat("y", 50)
	files := map[string]string{"a.go": "package a\n\n" + long + "\n"}
	const rule = `
id: no-sprintf
engine: grep
severity: error
pattern: 'Sprintf'
scope: ["**/*.go"]
`

	goKeys := findingKeys(t, files, rule, nil)
	engineKeys := findingKeys(t, files, rule, client)

	want := "a.go:3:Sprintf" + strings.Repeat("x", 119-len("Sprintf")) + "..."
	assert.Equal(t, []string{want}, goKeys, "Go path match text")
	assert.Equal(t, goKeys, engineKeys, "the engine must truncate the same way")
}

func TestTruncateCutsOnARuneBoundary(t *testing.T) {
	s := strings.Repeat("a", 119) + "é" + "tail"
	got := truncate(s, 120)
	assert.True(t, utf8.ValidString(got), "a cut inside a rune is not valid UTF-8: %q", got)
	assert.Equal(t, strings.Repeat("a", 119)+"...", got)

	assert.Equal(t, "short", truncate("short", 120))
}

// cap-tracing-context means "an external call should pass a context", so the
// finding is the call line without ctx. It was written as must-not-contain:
// ctx, which only read that way on the engine while the engine had
// must-not-contain inverted; by the documented meaning (engine.proto: "matched
// lines must NOT match this") it reported exactly the calls that do pass one.
func TestGeneratedTracingRuleReportsTheCallWithoutAContext(t *testing.T) {
	rule := observabilityRule()
	files := map[string]string{
		"svc.go": "package svc\n\nfunc a() { r, err := http.Get(url) }\nfunc b() { rows, err := sql.Query(ctx, q) }\n",
	}
	dir := setupTestProject(t, files)

	violations, err := RunGrep(rule, dir, nil)
	require.NoError(t, err)
	require.Len(t, violations, 1)
	assert.Equal(t, 3, violations[0].Line)
}
