// Package scriptguard holds the line-based checks verikt runs over CI,
// hook, script, build and container files: silent_ci_swallow,
// mutable_install_url, and the verikt:allow marker they share
// (invalid_allow_directive). It reads file content; walking the project is
// the checker's job.
package scriptguard

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

// Kind classifies the CI, hook, build and container files that the
// line-based guards (silent_ci_swallow, mutable_install_url) read.
type Kind int

const (
	// KindWorkflow is a GitHub Actions workflow (.github/workflows/*.yml).
	KindWorkflow Kind = iota + 1
	// KindLefthook is lefthook.yml or .lefthook.yml.
	KindLefthook
	// KindShell is a shell script under scripts/ or .claude/hooks/.
	KindShell
	// KindMakefile is a Makefile, GNUmakefile or *.mk.
	KindMakefile
	// KindDockerfile is a Dockerfile, Dockerfile.* or *.Dockerfile.
	KindDockerfile
	// KindMarkdown is README.md or docs/**/*.md (fenced code blocks only).
	KindMarkdown
	// KindCIConfig is any other YAML CI configuration (.gitlab-ci.yml,
	// bitbucket-pipelines.yml). The project walk does not classify files as
	// this kind; callers such as the template security test pass it
	// explicitly.
	KindCIConfig
)

// Finding is one line-based finding. Severity is the caller's decision:
// verikt check reports every rule at warning (INV-005), the template security
// test fails on any finding.
type Finding struct {
	Rule    string
	File    string
	Line    int
	Message string
}

// inlineAllowRules are the rule ids a verikt:allow marker may name.
var inlineAllowRules = map[string]bool{
	RuleSilentCISwallow:   true,
	RuleMutableInstallURL: true,
}

// Options carries the verikt.yaml settings the line-based guards use.
type Options struct {
	// ExtraCommands are extra verification commands for silent_ci_swallow
	// (check.silent_ci_swallow.extra_commands).
	ExtraCommands []string
}

// Classify maps a project-relative, slash-separated path to the
// kind of file the line-based guards read. includeDocs adds README.md and
// docs/**/*.md (check.mutable_install_url.include_docs).
func Classify(rel string, includeDocs bool) (Kind, bool) {
	base := path.Base(rel)
	dir := path.Dir(rel)
	switch {
	case dir == ".github/workflows" && (strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml")):
		return KindWorkflow, true
	case base == "lefthook.yml" || base == ".lefthook.yml":
		return KindLefthook, true
	case strings.HasSuffix(base, ".sh") && (strings.HasPrefix(rel, "scripts/") || strings.HasPrefix(rel, ".claude/hooks/")):
		return KindShell, true
	case base == "Makefile" || base == "GNUmakefile" || strings.HasSuffix(base, ".mk"):
		return KindMakefile, true
	case (base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.") || strings.HasSuffix(base, ".Dockerfile")) && !isTemplateFile(base):
		return KindDockerfile, true
	case includeDocs && (rel == "README.md" || strings.HasPrefix(rel, "docs/") && strings.HasSuffix(base, ".md")):
		return KindMarkdown, true
	}
	return 0, false
}

// templateSuffixes mark a file that renders into a Dockerfile rather than
// being one: its FROM lines hold placeholders, not image references.
var templateSuffixes = []string{".tmpl", ".tpl", ".j2", ".jinja", ".template"}

func isTemplateFile(base string) bool {
	for _, suffix := range templateSuffixes {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}

// Scan runs the line-based guards that apply to kind over one
// file's content and returns their findings plus any invalid verikt:allow
// markers, ordered by line. rel is used as Finding.File.
func Scan(rel string, kind Kind, content []byte, opts Options) []Finding {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	fences := markdownFences(lines, kind)
	syntax := func(n int) CommentSyntax {
		if kind == KindMarkdown && fences[n] == 0 {
			return CommentHTML
		}
		return AllComments
	}
	markers, out := ParseAllowMarkers(rel, lines, syntax, inlineAllowRules)
	logical := logicalLines(lines, fences, kind)
	for i := range logical {
		logical[i].cmd = commandText(kind, logical[i].text)
	}
	f := &scriptFile{rel: rel, kind: kind, markers: markers, logical: logical}
	out = append(out, scanSilentCISwallow(f, opts)...)
	out = append(out, scanMutableInstallURL(f)...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}

// scriptFile is one file prepared for the line-based rules.
type scriptFile struct {
	rel     string
	kind    Kind
	markers *AllowMarkers
	logical []logicalLine
}

// allowed reports whether a verikt:allow marker covers rule for ll.
func (f *scriptFile) allowed(rule string, ll logicalLine) bool {
	if f.markers.Allows(rule, ll.start, ll.end) {
		return true
	}
	// In markdown, a marker directly above the opening fence covers the block.
	return ll.fence > 0 && f.markers.Allows(rule, ll.fence, ll.fence)
}

// logicalLine is a command with comments removed and trailing-backslash
// continuations joined. start is the line findings are reported at.
type logicalLine struct {
	start, end int
	fence      int    // markdown: line of the opening fence; 0 elsewhere
	text       string // the line as written, comments removed
	cmd        string // the shell command in text: for YAML, the value without its key and quotes
}

// logicalLines strips comments and joins continuations. For markdown only
// lines inside fenced code blocks are kept.
func logicalLines(lines []string, fences map[int]int, kind Kind) []logicalLine {
	var out []logicalLine
	var cur *logicalLine
	for i, raw := range lines {
		n := i + 1
		if kind == KindMarkdown && fences[n] <= 0 {
			cur = nil
			continue
		}
		text := strings.TrimRight(stripLineComment(raw), " \t")
		cont := strings.HasSuffix(text, "\\")
		text = strings.TrimSuffix(text, "\\")
		if cur != nil {
			cur.text += " " + strings.TrimSpace(text)
			cur.end = n
		} else {
			out = append(out, logicalLine{start: n, end: n, fence: max(fences[n], 0), text: text})
			cur = &out[len(out)-1]
		}
		if !cont {
			cur = nil
		}
	}
	return out
}

// markdownFences maps each line inside a fenced code block to the line of
// its opening fence. Fence lines themselves map to -1. Nil for other kinds.
func markdownFences(lines []string, kind Kind) map[int]int {
	if kind != KindMarkdown {
		return nil
	}
	out := map[int]int{}
	open, marker := 0, ""
	for i, raw := range lines {
		n := i + 1
		t := strings.TrimLeft(raw, " ")
		if len(raw)-len(t) <= 3 && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")) {
			switch {
			case open == 0:
				open, marker = n, t[:3]
			case strings.HasPrefix(t, marker):
				open = 0
			}
			out[n] = -1
			continue
		}
		if open > 0 {
			out[n] = open
		}
	}
	return out
}

// stripLineComment removes a "#" comment: a "#" outside quotes at the start
// of the line or after whitespace. "${#x}", "$#" and URL fragments stay.
func stripLineComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch {
		case (c == '"' || c == '\'') && opensQuote(line, i):
			quote = c
		case c == '#' && atWordStart(line, i):
			return line[:i]
		}
	}
	return line
}

// violation builds a finding of rule for ll.
func (f *scriptFile) violation(rule, msg string, ll logicalLine) Finding {
	return Finding{File: f.rel, Line: ll.start, Message: msg, Rule: rule}
}

// yamlKeyPrefix is the "- key: " in front of a command in YAML files.
var yamlKeyPrefix = regexp.MustCompile(`^\s*(?:-\s+)?(?:[A-Za-z_][\w.-]*\s*:(?:\s+|$))?`)

// commandText returns the shell command a logical line holds. In YAML files
// that is the value after "key:" or "- ", unwrapped from a quoted scalar so
// `run: "go test || true"` reads the same as the unquoted form.
func commandText(kind Kind, text string) string {
	if kind != KindWorkflow && kind != KindLefthook && kind != KindCIConfig {
		return text
	}
	v := strings.TrimSpace(yamlKeyPrefix.ReplaceAllString(text, ""))
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] && !strings.ContainsRune(v[1:len(v)-1], rune(v[0])) {
		return v[1 : len(v)-1]
	}
	return v
}

// snippet shortens a command for a finding message.
func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}
