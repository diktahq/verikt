package guide

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/config"
	"github.com/stretchr/testify/require"
)

// INV-001 enforcement for the always-loaded writer rules.
//
// These tests generate every target for representative verikt.yaml configs and
// fail closed when an always-loaded file exceeds the budget. On-demand files
// under .verikt/references/ are deliberately not budgeted: they are read only
// when the task calls for them.

const (
	// maxInstructions is INV-001's hard ceiling ("fails if >30 instructions").
	maxInstructions = 30
	// maxApproxTokens is INV-001's upper bound of 1,500 tokens per file.
	maxApproxTokens = 1500
	// bytesPerToken approximates tokens as ceil(bytes/4). For English prose and
	// markdown this over-counts slightly versus BPE tokenizers, and multi-byte
	// characters (emoji, em dashes) count as several tokens, so the estimate
	// errs on the side of failing.
	bytesPerToken = 4
)

var (
	numberedItem = regexp.MustCompile(`^\d+[.)]\s`)
	tableDivider = regexp.MustCompile(`^\|[\s:|-]+\|?$`)
)

// countInstructions counts the directive lines in a generated markdown file.
//
// An "instruction" is any line, outside YAML frontmatter and fenced code
// blocks, that is:
//   - a bullet item ("- ", "* ", "+ "), at any indentation, including checklist items;
//   - a numbered item ("1. ", "2) "), at any indentation;
//   - a table body row (a "|" row after a table's header and divider rows).
//
// Headings, prose paragraphs, table headers and code examples do not count.
// Prose can still carry directives, so this is a floor on the real instruction
// count, which is why the ceiling is INV-001's hard limit rather than its
// 15-25 target.
func countInstructions(content string) int {
	lines := strings.Split(content, "\n")
	count := 0
	inFence := false
	inFrontmatter := len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"
	tableRow := 0 // rows seen in the current table
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if inFrontmatter {
			if i > 0 && line == "---" {
				inFrontmatter = false
			}
			continue
		}
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(line, "|") {
			tableRow++
			if tableRow > 1 && !tableDivider.MatchString(line) {
				count++
			}
			continue
		}
		tableRow = 0
		switch {
		case strings.HasPrefix(line, "- "), strings.HasPrefix(line, "* "), strings.HasPrefix(line, "+ "):
			count++
		case numberedItem.MatchString(line):
			count++
		}
	}
	return count
}

func approxTokens(content string) int {
	return (len(content) + bytesPerToken - 1) / bytesPerToken
}

type budgetCase struct {
	name string
	opts GenerateOptions
	// rules are proxy rules written to .verikt/rules before generation.
	rules map[string]string
}

func budgetCases() []budgetCase {
	heavyComponents := []config.Component{
		{Name: "domain", In: []string{"domain/**"}, MayDependOn: []string{}},
		{Name: "ports", In: []string{"port/**"}, MayDependOn: []string{"domain"}},
		{Name: "service", In: []string{"service/**"}, MayDependOn: []string{"domain", "ports"}},
		{Name: "adapters", In: []string{"adapter/**"}, MayDependOn: []string{"ports", "domain"}},
		{Name: "platform", In: []string{"platform/**"}, MayDependOn: []string{"domain"}},
		{Name: "cmd", In: []string{"cmd/**"}, MayDependOn: []string{"domain", "ports", "service", "adapters", "platform"}},
	}
	heavyCaps := []string{
		// Triggers all three critical (MUST) warnings: http-client without
		// circuit-breaker, multi-tenancy without auth, event-bus without outbox.
		"http-api", "http-client", "multi-tenancy", "event-bus",
		"grpc", "kafka-consumer", "postgres", "mysql", "redis", "migrations",
		"retry", "rate-limiting", "idempotency", "cors", "audit-log",
		"observability", "docker", "ci-github", "testing", "linting",
		"cqrs", "ddd", "saga", "scheduler", "worker", "templ", "htmx",
	}
	heavyFeatures := map[string]bool{}
	for _, k := range featureOrder {
		heavyFeatures[k] = true
	}
	heavyDecisions := []config.Decision{
		{Topic: "architecture-pattern", Tier: 1, Status: "decided", Choice: "hexagonal"},
		{Topic: "authentication-strategy", Tier: 1, Status: "undecided"},
		{Topic: "tenant-isolation", Tier: 1, Status: "undecided"},
		{Topic: "failure-strategy", Tier: 2, Status: "undecided"},
	}
	heavyRules := map[string]string{
		"domain-isolation.yaml": "id: domain-isolation\nengine: grep\nseverity: error\ndescription: Domain must not import infrastructure\npattern: \"infrastructure\"\nscope:\n  - \"domain/**/*.go\"\n",
		"no-fmt-print.yaml":     "id: no-fmt-print\nengine: grep\nseverity: warning\ndescription: Use the logger\npattern: \"fmt.Println\"\nscope:\n  - \"domain/**/*.go\"\n",
	}

	cases := make([]budgetCase, 0, 6)
	cases = append(cases, budgetCase{
		name: "minimal-flat",
		opts: GenerateOptions{Architecture: "flat"},
	})
	cases = append(cases, budgetCase{
		name: "typical-hexagonal",
		opts: GenerateOptions{
			Architecture: "hexagonal",
			Capabilities: []string{"http-api", "mysql", "health", "request-id"},
			Components:   heavyComponents[:4],
		},
	})
	for _, mode := range []string{"passive", "audit", "prompted"} {
		cases = append(cases, budgetCase{
			name: "capability-heavy-" + mode,
			opts: GenerateOptions{
				Architecture:    "hexagonal",
				Capabilities:    heavyCaps,
				Components:      heavyComponents,
				Decisions:       heavyDecisions,
				LanguageVersion: "Go 1.26",
				Features:        heavyFeatures,
				GuideMode:       mode,
				TemplateFS:      testCapFS(),
			},
			rules: heavyRules,
		})
	}
	cases = append(cases, budgetCase{
		name: "catalog-only",
		opts: GenerateOptions{
			Capabilities: heavyCaps,
			CatalogOnly:  true,
			TemplateFS:   testCapFS(),
		},
	})
	return cases
}

// alwaysLoadedFiles generates every target for bc and returns the content
// each agent loads on every turn, keyed by a label.
func alwaysLoadedFiles(t *testing.T, bc budgetCase) map[string]string {
	t.Helper()
	dir := t.TempDir()
	if len(bc.rules) > 0 {
		rulesDir := filepath.Join(dir, ".verikt", "rules")
		require.NoError(t, os.MkdirAll(rulesDir, 0o755))
		for name, body := range bc.rules {
			require.NoError(t, os.WriteFile(filepath.Join(rulesDir, name), []byte(body), 0o644))
		}
		// Rules are only "valid" when their scope matches a file.
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "domain"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "domain", "entity.go"), []byte("package domain\n"), 0o644))
	}

	opts := bc.opts
	opts.ProjectDir = dir
	opts.Target = "all"
	require.NoError(t, Generate(opts))

	read := func(rel string) string {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		require.NoError(t, err, rel)
		return string(data)
	}
	claudeFile := filepath.Join(".claude", "rules", "verikt-index.md")
	if opts.CatalogOnly {
		claudeFile = filepath.Join(".claude", "rules", "verikt.md")
	}
	agents := read("AGENTS.md")
	start := strings.Index(agents, sentinelStart)
	end := strings.Index(agents, sentinelEnd)
	require.True(t, start >= 0 && end > start, "AGENTS.md must contain the verikt section")

	return map[string]string{
		"claude":         read(claudeFile),
		"cursor":         read(filepath.Join(".cursor", "rules", "verikt.mdc")),
		"copilot":        read(filepath.Join(".github", "copilot-instructions.md")),
		"windsurf":       read(".windsurfrules"),
		"agents-section": agents[start : end+len(sentinelEnd)],
	}
}

// budgetedFiles returns every file INV-001's budget applies to for bc: the
// always-loaded files plus Claude's per-category files. Those load whenever an
// edited path matches their `paths:` globs, and "domain/**" or "**/*_test.go" match most
// edits, so they are held to the same budget.
func budgetedFiles(t *testing.T, bc budgetCase) map[string]string {
	t.Helper()
	files := alwaysLoadedFiles(t, bc)
	if bc.opts.CatalogOnly {
		return files
	}
	for category, content := range buildSplitContent(bc.opts).Categories {
		files["claude/verikt-"+category+".md"] = content
	}
	return files
}

func TestGuideOutput_InstructionCount(t *testing.T) {
	for _, bc := range budgetCases() {
		t.Run(bc.name, func(t *testing.T) {
			for label, content := range budgetedFiles(t, bc) {
				n := countInstructions(content)
				t.Logf("%s: %d instructions", label, n)
				if n > maxInstructions {
					t.Errorf("%s: %d instructions, INV-001 allows at most %d; move detail into .verikt/references/", label, n, maxInstructions)
				}
			}
		})
	}
}

func TestGuideOutput_TokenBudget(t *testing.T) {
	for _, bc := range budgetCases() {
		t.Run(bc.name, func(t *testing.T) {
			for label, content := range budgetedFiles(t, bc) {
				n := approxTokens(content)
				t.Logf("%s: ~%d tokens", label, n)
				if n > maxApproxTokens {
					t.Errorf("%s: ~%d tokens (%d bytes), INV-001 allows at most %d; move detail into .verikt/references/", label, n, len(content), maxApproxTokens)
				}
			}
		})
	}
}

// TestGuideOutput_CriticalRulesAtStartAndEnd checks INV-001's placement rule:
// the critical guidance opens the file (agent instructions) and closes it.
func TestGuideOutput_CriticalRulesAtStartAndEnd(t *testing.T) {
	for _, bc := range budgetCases() {
		t.Run(bc.name, func(t *testing.T) {
			for label, content := range alwaysLoadedFiles(t, bc) {
				howTo := strings.Index(content, "## How to Use This Guide")
				finish := strings.LastIndex(content, "## Before You Finish")
				require.GreaterOrEqual(t, howTo, 0, label)
				require.Greater(t, finish, howTo, label)
				tail := content[finish:]
				require.Contains(t, tail, "verikt check", label)
				require.NotContains(t, tail[len("## Before You Finish"):], "\n## ", "%s: Before You Finish must be the last section", label)
			}
		})
	}
}

func TestCountInstructions_Definition(t *testing.T) {
	content := "---\nglobs:\n- not counted\n---\n" +
		"# Title\n\nProse with a directive is not counted.\n\n" +
		"- bullet\n* star\n+ plus\n  - nested\n- [ ] checklist\n" +
		"1. first\n2) second\n" +
		"| Header | Col |\n|---|---|\n| row1 | a |\n| row2 | b |\n\n" +
		"```\n- inside fence\n1. inside fence\n```\n"
	// 5 bullets + 2 numbered + 2 table body rows.
	require.Equal(t, 9, countInstructions(content))
}

func TestApproxTokens_RoundsUp(t *testing.T) {
	require.Equal(t, 0, approxTokens(""))
	require.Equal(t, 1, approxTokens("abc"))
	require.Equal(t, 2, approxTokens("abcde"))
}

// TestBudget_FailsClosedOnBloatedOutput guards the guard: the full monolithic
// guide (everything the references hold) must be over budget, otherwise the
// budget tests would pass vacuously. Before the references existed, that
// monolithic guide was what cursor, copilot and windsurf received.
func TestBudget_FailsClosedOnBloatedOutput(t *testing.T) {
	bc := budgetCases()[2] // capability-heavy-passive
	full := buildContent(bc.opts)
	require.Greater(t, countInstructions(full), maxInstructions)
	require.Greater(t, approxTokens(full), maxApproxTokens)
}
