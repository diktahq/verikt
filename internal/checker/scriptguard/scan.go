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
	"unicode/utf8"
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
	// Reason is the reason a verikt:allow marker gave, set only on a finding
	// in Result.Waived.
	Reason string
}

// Result is what ScanAll finds in one file. Findings still count; Waived are
// the findings a valid verikt:allow marker covers, kept so a waiver stays
// visible with its reason instead of vanishing.
type Result struct {
	Findings []Finding
	Waived   []Finding
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
// markers, ordered by line. rel is used as Finding.File. Findings a marker
// waives are left out; ScanAll returns them.
func Scan(rel string, kind Kind, content []byte, opts Options) []Finding {
	return ScanAll(rel, kind, content, opts).Findings
}

// ScanAll is Scan that also returns the findings verikt:allow markers waive,
// each with the marker's reason. Both lists are ordered by line.
func ScanAll(rel string, kind Kind, content []byte, opts Options) Result {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	fences := markdownFences(lines, kind)
	syntax := func(n int) CommentSyntax {
		if kind == KindMarkdown && fences[n] == 0 {
			return CommentHTML
		}
		return AllComments
	}
	markers, invalid := ParseAllowMarkers(rel, lines, syntax, inlineAllowRules)
	logical := logicalLines(lines, fences, kind)
	for i := range logical {
		logical[i].cmd = commandText(kind, logical[i].text)
	}
	f := &scriptFile{rel: rel, kind: kind, markers: markers, logical: logical}
	f.res.Findings = invalid
	scanSilentCISwallow(f, opts)
	scanMutableInstallURL(f)
	for _, list := range []*[]Finding{&f.res.Findings, &f.res.Waived} {
		sort.SliceStable(*list, func(i, j int) bool { return (*list)[i].Line < (*list)[j].Line })
	}
	return f.res
}

// scriptFile is one file prepared for the line-based rules, and what they
// have reported so far.
type scriptFile struct {
	rel     string
	kind    Kind
	markers *AllowMarkers
	logical []logicalLine
	res     Result
}

// report records a finding of rule for ll: waived, with the marker's reason,
// when a verikt:allow marker covers it, otherwise as a finding.
func (f *scriptFile) report(rule, msg string, ll logicalLine) {
	v := Finding{File: f.rel, Line: ll.start, Message: msg, Rule: rule}
	reason, ok := f.markers.Reason(rule, ll.start, ll.end)
	if !ok && ll.fence > 0 {
		// In markdown, a marker directly above the opening fence covers the block.
		reason, ok = f.markers.Reason(rule, ll.fence, ll.fence)
	}
	if ok {
		v.Reason = reason
		f.res.Waived = append(f.res.Waived, v)
		return
	}
	f.res.Findings = append(f.res.Findings, v)
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
		case c == '\\':
			i++ // an escaped character outside quotes: `\"` opens nothing, `\#` is no comment
		case (c == '"' || c == '\'') && opensQuote(line, i):
			quote = c
		case c == '#' && atWordStart(line, i):
			return line[:i]
		}
	}
	return line
}

// yamlKeyPrefix is the "- key: " in front of a command in YAML files.
var yamlKeyPrefix = regexp.MustCompile(`^\s*(?:-\s+)?(?:[A-Za-z_][\w.-]*\s*:(?:\s+|$))?`)

// commandText returns the shell command a logical line holds. In YAML files
// that is the value after "key:" or "- ", unwrapped and unescaped from a
// quoted scalar so `run: "go test -run \"A\" || true"` reads as the runner
// receives it.
func commandText(kind Kind, text string) string {
	if kind != KindWorkflow && kind != KindLefthook && kind != KindCIConfig {
		return text
	}
	v := strings.TrimSpace(yamlKeyPrefix.ReplaceAllString(text, ""))
	if unquoted, ok := yamlQuotedScalar(v); ok {
		return unquoted
	}
	return v
}

// yamlDoubleQuotedEscapes are the YAML escapes that matter in a command.
// Any other escaped character is kept as written.
var yamlDoubleQuotedEscapes = map[byte]string{'"': `"`, '\\': `\`, '/': "/", 'n': "\n", 't': "\t", ' ': " "}

// yamlQuotedScalar unquotes v when it is a whole single-line YAML quoted
// scalar: "..." with backslash escapes, or '...' with a doubled quote inside. A
// scalar that does not close on this line, or has text after its closing
// quote, is not unwrapped.
func yamlQuotedScalar(v string) (string, bool) {
	if len(v) < 2 || (v[0] != '"' && v[0] != '\'') {
		return "", false
	}
	q := v[0]
	var b strings.Builder
	for i := 1; i < len(v); i++ {
		c := v[i]
		switch {
		case q == '"' && c == '\\' && i+1 < len(v):
			i++
			if e, ok := yamlDoubleQuotedEscapes[v[i]]; ok {
				b.WriteString(e)
			} else {
				b.WriteByte('\\')
				b.WriteByte(v[i])
			}
		case q == '\'' && c == '\'' && i+1 < len(v) && v[i+1] == '\'':
			i++
			b.WriteByte('\'')
		case c == q:
			return b.String(), strings.TrimSpace(v[i+1:]) == ""
		default:
			b.WriteByte(c)
		}
	}
	return "", false
}

// snippet shortens source text echoed in a finding message to at most 80
// bytes, cutting on a UTF-8 boundary.
func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= 80 {
		return s
	}
	cut := 77
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}
