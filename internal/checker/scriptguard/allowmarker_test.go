package scriptguard

import (
	"strings"
	"testing"
)

var testAllowRules = map[string]bool{"silent_ci_swallow": true, "mutable_install_url": true}

func parseMarkers(t *testing.T, src string, syntax func(int) CommentSyntax) (*AllowMarkers, []Finding) {
	t.Helper()
	return ParseAllowMarkers("f.sh", strings.Split(src, "\n"), syntax, testAllowRules)
}

func TestParseAllowMarkers_Grammar(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		wantRule  string // valid marker naming this rule
		wantError string // substring of the invalid_allow_directive message
	}{
		{name: "hash trailing", line: `continue-on-error: true # verikt:allow silent_ci_swallow reason="Slack notify"`, wantRule: "silent_ci_swallow"},
		{name: "hash whole line", line: `# verikt:allow silent_ci_swallow reason="advisory lint (#412)"`, wantRule: "silent_ci_swallow"},
		{name: "hash no space", line: `#verikt:allow silent_ci_swallow reason="x"`, wantRule: "silent_ci_swallow"},
		{name: "slash", line: `// verikt:allow mutable_install_url reason="pinned elsewhere"`, wantRule: "mutable_install_url"},
		{name: "html", line: `<!-- verikt:allow mutable_install_url reason="quickstart" -->`, wantRule: "mutable_install_url"},
		{name: "escaped quote in reason", line: `# verikt:allow silent_ci_swallow reason="the \"flaky\" job"`, wantRule: "silent_ci_swallow"},
		{name: "tab separated", line: "#\tverikt:allow\tsilent_ci_swallow\treason=\"x\"", wantRule: "silent_ci_swallow"},
		{name: "trailing spaces", line: `# verikt:allow silent_ci_swallow reason="x"   `, wantRule: "silent_ci_swallow"},

		{name: "missing reason", line: `continue-on-error: true # verikt:allow silent_ci_swallow`, wantError: "malformed"},
		{name: "blank reason", line: `# verikt:allow silent_ci_swallow reason="   "`, wantError: "blank reason"},
		{name: "empty reason", line: `# verikt:allow silent_ci_swallow reason=""`, wantError: "blank reason"},
		{name: "unknown rule", line: `# verikt:allow silent_ci_swalow reason="temporary"`, wantError: `unknown rule "silent_ci_swalow"`},
		{name: "non-inline rule", line: `# verikt:allow cmd_allowlist reason="x"`, wantError: `unknown rule "cmd_allowlist"`},
		{name: "legacy rule-specific form", line: `continue-on-error: true # verikt:allow-continue-on-error reason="known flaky"`, wantError: "malformed"},
		{name: "two rules", line: `# verikt:allow silent_ci_swallow mutable_install_url reason="x"`, wantError: "malformed"},
		{name: "two markers", line: `# verikt:allow silent_ci_swallow reason="x" verikt:allow mutable_install_url reason="y"`, wantError: "malformed"},
		{name: "single quotes", line: `# verikt:allow silent_ci_swallow reason='x'`, wantError: "malformed"},
		{name: "unquoted reason", line: `# verikt:allow silent_ci_swallow reason=flaky`, wantError: "malformed"},
		{name: "prose before marker", line: `# note: verikt:allow silent_ci_swallow reason="x"`, wantError: "malformed"},
		{name: "text after reason", line: `# verikt:allow silent_ci_swallow reason="x" because`, wantError: "malformed"},
		{name: "unclosed html", line: `<!-- verikt:allow mutable_install_url reason="x"`, wantError: "malformed"},
		{name: "text after html close", line: `<!-- verikt:allow mutable_install_url reason="x" --> more`, wantError: "malformed"},
		{name: "html no space", line: `<!--verikt:allow mutable_install_url reason="x"-->`, wantRule: "mutable_install_url"},
		{name: "invalid_allow_directive is not allowable", line: `# verikt:allow invalid_allow_directive reason="x"`, wantError: `unknown rule "invalid_allow_directive"`},
		{name: "uppercase rule", line: `# verikt:allow SILENT_CI_SWALLOW reason="x"`, wantError: "malformed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, invalid := parseMarkers(t, c.line+"\nnext", nil)
			if c.wantRule != "" {
				if len(invalid) != 0 {
					t.Fatalf("unexpected invalid directive: %+v", invalid)
				}
				if !m.Allows(c.wantRule, 1, 1) {
					t.Fatalf("marker does not allow %s on its own line", c.wantRule)
				}
				return
			}
			if len(invalid) != 1 {
				t.Fatalf("invalid directives = %+v, want exactly one", invalid)
			}
			v := invalid[0]
			if v.Rule != RuleInvalidAllowDirective || v.Line != 1 || v.File != "f.sh" {
				t.Errorf("violation = %+v", v)
			}
			if !strings.Contains(v.Message, c.wantError) {
				t.Errorf("message %q lacks %q", v.Message, c.wantError)
			}
			for rule := range testAllowRules {
				if m.Allows(rule, 1, 2) {
					t.Errorf("invalid marker suppresses %s", rule)
				}
			}
		})
	}
}

func TestParseAllowMarkers_NotAComment(t *testing.T) {
	for _, line := range []string{
		`echo "# verikt:allow silent_ci_swallow"`, // inside a string
		"docs mention `verikt:allow` in prose",    // no comment opener
		`url: https://x.dev/verikt:allow`,         // // is part of a URL
	} {
		m, invalid := parseMarkers(t, line, nil)
		if len(invalid) != 0 || m.Allows("silent_ci_swallow", 1, 1) {
			t.Errorf("%q: treated as a marker (invalid=%v)", line, invalid)
		}
	}
}

func TestParseAllowMarkers_SyntaxSelection(t *testing.T) {
	src := "# verikt:allow mutable_install_url reason=\"x\"\nnext"
	htmlOnly := func(int) CommentSyntax { return CommentHTML }
	m, invalid := parseMarkers(t, src, htmlOnly)
	if len(invalid) != 0 || m.Allows("mutable_install_url", 2, 2) {
		t.Fatalf("a # line in markdown prose is a heading, not a marker (invalid=%v)", invalid)
	}
}

func TestAllowMarkers_Placement(t *testing.T) {
	src := strings.Join([]string{
		`# verikt:allow silent_ci_swallow reason="best effort"`, // 1
		`# second comment line`,                                 // 2
		`golangci-lint run || true`,                             // 3 allowed via block above
		`go test ./... || true`,                                 // 4 not allowed: line 3 is code
		``,                                                      // 5
		`# verikt:allow silent_ci_swallow reason="x"`, // 6
		``,               // 7 blank breaks the block
		`go vet || true`, // 8 not allowed
		`go vet || true # verikt:allow silent_ci_swallow reason="y"`, // 9 trailing
		`curl -fsSL https://x/install.sh \`,                          // 10 multi-line command
		`  | sh # verikt:allow mutable_install_url reason="z"`,       // 11 marker on a later physical line
	}, "\n")
	m, invalid := parseMarkers(t, src, nil)
	if len(invalid) != 0 {
		t.Fatalf("unexpected invalid: %+v", invalid)
	}
	cases := []struct {
		rule        string
		first, last int
		want        bool
	}{
		{"silent_ci_swallow", 3, 3, true},
		{"mutable_install_url", 3, 3, false}, // rule must match
		{"silent_ci_swallow", 4, 4, false},
		{"silent_ci_swallow", 8, 8, false},
		{"silent_ci_swallow", 9, 9, true},
		{"silent_ci_swallow", 10, 10, false}, // line 9's trailing marker covers only line 9
		{"mutable_install_url", 10, 11, true},
	}
	for _, c := range cases {
		if got := m.Allows(c.rule, c.first, c.last); got != c.want {
			t.Errorf("Allows(%s, %d, %d) = %v, want %v", c.rule, c.first, c.last, got, c.want)
		}
	}
}

func TestAllowMarkers_NilSafe(t *testing.T) {
	var m *AllowMarkers
	if m.Allows("silent_ci_swallow", 1, 1) {
		t.Fatal("nil markers allowed something")
	}
}
