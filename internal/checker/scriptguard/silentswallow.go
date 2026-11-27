package scriptguard

import (
	"fmt"
	"regexp"
	"strings"
)

// RuleSilentCISwallow flags CI, hook and script constructs that turn a failed
// test, lint, check or migration into a passing step.
const RuleSilentCISwallow = "silent_ci_swallow"

var (
	// trueValue is a YAML value that is literally true: true, 'true',
	// "true" (any case) or the expression ${{ true }}. Other expressions,
	// like ${{ matrix.x }}, are not literal.
	trueValue = `(?:(?i:true|'true'|"true")|\$\{\{\s*true\s*\}\})`

	// continueOnErrorTrue matches `continue-on-error: true` (after comments
	// are stripped).
	continueOnErrorTrue = regexp.MustCompile(`^\s*(?:-\s*)?continue-on-error\s*:\s*` + trueValue + `\s*$`)

	// trueAlone is a line holding only a true value.
	trueAlone = regexp.MustCompile(`^\s*` + trueValue + `\s*$`)

	// continueOnErrorKey is `continue-on-error:` with its value on the next line.
	continueOnErrorKey = regexp.MustCompile(`^\s*(?:-\s*)?(continue-on-error)\s*:\s*$`)

	// swallowTail matches what follows "||" in a swallow: true (or its path),
	// ":", exit 0 or (exit 0).
	swallowTail = regexp.MustCompile(`^\s*(true|/(?:usr/)?bin/true|:|exit\s+0|\(\s*exit\s+0\s*\))(?:[\s;&|)}]|$)`)

	// shellCommandString is `sh -c "cmd"` (any -…c… flag cluster, optional
	// path), capturing the quoted command string. Unquoted words after it
	// ($0 and positional arguments, redirections) do not change its status.
	shellCommandString = regexp.MustCompile(`^(?:/(?:usr/)?(?:local/)?bin/)?(?:ba|z|da|k|a)?sh\s+(?:[-+]\S+\s+)*-[A-Za-z]*c[A-Za-z]*\s+(?:"(.*)"|'(.*)')(?:\s+[^\s"']+)*\s*$`)

	// braceGroup is `{ cmd; }`, capturing the commands inside.
	braceGroup = regexp.MustCompile(`^\{\s(.*?);?\s*\}$`)

	// makeIgnoreErrors is a Make recipe line whose prefix includes "-", which
	// tells make to ignore the command's exit status.
	makeIgnoreErrors = regexp.MustCompile(`^\t[@+]*-[-@+]*`)

	// exitStatusRead matches a read of $? (or $$? in a Makefile recipe).
	exitStatusRead = regexp.MustCompile(`\$\$?(?:\?|\{\?\})`)

	// captureAssign is VAR=$(cmd) or VAR=`cmd`, optionally declared.
	captureAssign = regexp.MustCompile("^(?:(?:export|local|readonly|declare|typeset)\\s+(?:-\\w+\\s+)*)?[A-Za-z_]\\w*=(?:\\$\\((.*)\\)|`(.*)`)$")

	envAssign = regexp.MustCompile(`^[A-Za-z_]\w*=`)
)

// scanSilentCISwallow applies silent_ci_swallow to f.
func scanSilentCISwallow(f *scriptFile, opts Options) {
	switch f.kind {
	case KindWorkflow, KindLefthook, KindShell, KindMakefile, KindCIConfig:
	default:
		return
	}
	for i, ll := range f.logical {
		next := nextText(f.logical, i)
		if f.kind == KindWorkflow && continueOnErrorIsTrue(ll.text, next) {
			f.report(RuleSilentCISwallow, "continue-on-error: true lets this job or step fail while the workflow stays green. "+
				"Remove it, or mark a deliberate best-effort step with verikt:allow silent_ci_swallow reason=\"...\"", ll)
			continue
		}
		reported := false
		for _, s := range findSwallows(ll.cmd) {
			if msg, ok := swallowFinding(ll.cmd, s, next, opts.ExtraCommands); ok {
				f.report(RuleSilentCISwallow, msg, ll)
				reported = true
			}
		}
		if f.kind == KindMakefile && !reported {
			if msg, ok := makeIgnoredFinding(ll.text, opts.ExtraCommands); ok {
				f.report(RuleSilentCISwallow, msg, ll)
			}
		}
	}
}

// continueOnErrorIsTrue reports `continue-on-error: true`, including the
// value written alone on the next, further-indented line.
func continueOnErrorIsTrue(text, next string) bool {
	if continueOnErrorTrue.MatchString(text) {
		return true
	}
	m := continueOnErrorKey.FindStringSubmatchIndex(text)
	if m == nil {
		return false
	}
	indent := len(next) - len(strings.TrimLeft(next, " "))
	return indent > m[2] && trueAlone.MatchString(next)
}

// makeIgnoredFinding reports a Make recipe line whose "-" prefix discards the
// exit status of a verification command in it.
func makeIgnoredFinding(text string, extra []string) (string, bool) {
	prefix := makeIgnoreErrors.FindString(text)
	if prefix == "" {
		return "", false
	}
	for _, part := range splitTopLevel(text[len(prefix):], listSep) {
		if name, ok := swallowedVerification(part, extra); ok {
			return fmt.Sprintf("the \"-\" prefix on %q makes make ignore the exit status of %s, so a failing check passes. "+
				"Remove the \"-\", or mark a deliberate one with verikt:allow silent_ci_swallow reason=\"...\"",
				snippet(text), name), true
		}
	}
	return "", false
}

// nextText returns the next non-empty logical line after index i.
func nextText(lls []logicalLine, i int) string {
	for _, ll := range lls[i+1:] {
		if strings.TrimSpace(ll.text) != "" {
			return ll.text
		}
	}
	return ""
}

// swallowFinding decides whether one swallow is reported, and why.
func swallowFinding(text string, s swallow, next string, extra []string) (string, bool) {
	if s.tail != "exit 0" && (exitStatusRead.MatchString(text[s.end:]) || exitStatusRead.MatchString(next)) {
		return fmt.Sprintf("%q is followed by a read of $?, which is always 0 after || %s, so the failure it checks for can never be seen. "+
			"Capture the status without the swallow (set +e; cmd; rc=$?; set -e)", snippet(text[s.cmdStart:s.end]), s.tail), true
	}
	name, ok := swallowedVerification(text[s.cmdStart:s.pos], extra)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%q swallows the exit status of %s, so a failing check passes. "+
		"Remove the swallow, or mark a deliberate one with verikt:allow silent_ci_swallow reason=\"...\"", snippet(text[s.cmdStart:s.end]), name), true
}

// swallow is one "|| true", "|| :" or "|| exit 0" in a logical line.
type swallow struct {
	cmdStart int    // start of the command whose status is swallowed
	pos      int    // index of "||"
	end      int    // index just past the swallow
	tail     string // "true", ":" or "exit 0"
}

// findSwallows returns every swallow outside quotes in text.
func findSwallows(text string) []swallow {
	sc := scanShell(text)
	var out []swallow
	for i := 0; i+1 < len(text); i++ {
		if sc.quoted[i] || text[i] != '|' || text[i+1] != '|' || (i > 0 && text[i-1] == '|') {
			continue
		}
		m := swallowTail.FindStringSubmatchIndex(text[i+2:])
		if m == nil {
			continue
		}
		tail := strings.Join(strings.Fields(text[i+2+m[2]:i+2+m[3]]), " ")
		out = append(out, swallow{cmdStart: commandStart(text, sc, i), pos: i, end: i + 2 + m[3], tail: tail})
	}
	return out
}

// commandStart walks left from the "||" at pos to the start of the command
// (pipeline) it applies to: the previous ;, &, &&, || or the opening of the
// enclosing group, at the same nesting depth and outside quotes.
func commandStart(text string, sc shellScan, pos int) int {
	from := pos
	if open, ok := groupOpen(text, sc, pos); ok {
		// `{ cmd; } || true` swallows the group: start before its "{".
		from = open
	}
	d0 := sc.depth[pos]
	for j := from - 1; j >= 0; j-- {
		if sc.quoted[j] || sc.depth[j] > d0 {
			continue
		}
		if sc.depth[j] < d0 {
			return j + 1
		}
		switch text[j] {
		case ';', '{', '}', '\n':
			return j + 1
		case '&':
			if ampersandIsOperator(text, j) {
				return j + 1
			}
		case '|':
			if j > 0 && text[j-1] == '|' {
				return j + 1
			}
		}
	}
	return 0
}

// groupOpen finds the "{" of the brace group whose "}" ends the command
// before the "||" at pos. A "}" closes a group only as a word after ";" or
// "&"; `${VAR}` does not.
func groupOpen(text string, sc shellScan, pos int) (int, bool) {
	k := lastNonSpace(text, pos)
	if k < 0 || text[k] != '}' || sc.quoted[k] || !closesGroup(text, k) {
		return 0, false
	}
	nest := 1
	for j := k - 1; j >= 0; j-- {
		if sc.quoted[j] {
			continue
		}
		switch {
		case text[j] == '}' && closesGroup(text, j):
			nest++
		case text[j] == '{' && opensGroup(text, j):
			if nest--; nest == 0 {
				return j, true
			}
		}
	}
	return 0, false
}

// lastNonSpace returns the index of the last non-blank byte before end, or -1.
func lastNonSpace(text string, end int) int {
	k := end - 1
	for k >= 0 && (text[k] == ' ' || text[k] == '\t') {
		k--
	}
	return k
}

func closesGroup(text string, k int) bool {
	p := lastNonSpace(text, k)
	return p >= 0 && (text[p] == ';' || text[p] == '&' || text[p] == '\n')
}

// ampersandIsOperator reports whether the "&" at i ends a command (&, &&),
// rather than being part of a redirection (2>&1, <&3, &>file) or "|&".
func ampersandIsOperator(text string, i int) bool {
	if i > 0 && strings.IndexByte("<>|", text[i-1]) >= 0 {
		return false
	}
	return i+1 >= len(text) || text[i+1] != '>'
}

func opensGroup(text string, j int) bool {
	return (j == 0 || strings.IndexByte(" \t;&|(", text[j-1]) >= 0) && j+1 < len(text) && (text[j+1] == ' ' || text[j+1] == '\t')
}

// shellScan records, per byte, whether it is inside quotes and its
// parenthesis depth. An opening paren has the depth outside it; the closing
// paren the depth inside.
type shellScan struct {
	quoted []bool
	depth  []int
}

func scanShell(text string) shellScan {
	sc := shellScan{quoted: make([]bool, len(text)), depth: make([]int, len(text))}
	var quote byte
	depth := 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		sc.depth[i] = depth
		switch {
		case quote != 0:
			sc.quoted[i] = true
			if c == '\\' && quote == '"' && i+1 < len(text) {
				i++
				sc.quoted[i], sc.depth[i] = true, depth
			} else if c == quote {
				quote = 0
			}
		case c == '\\' && i+1 < len(text):
			// Outside quotes a backslash makes the next character literal:
			// `\"` is not an opening quote, `\|` is not a pipe.
			i++
			sc.quoted[i], sc.depth[i] = true, depth
		case c == '"' || c == '\'':
			if opensQuote(text, i) {
				quote = c
				sc.quoted[i] = true
			}
		case c == '(':
			depth++
		case c == ')' && depth > 0:
			depth--
		}
	}
	return sc
}

// wrappedCommands returns the command list inside ( ... ), VAR=$(...),
// sh -c "..." or { ...; }: the exit status of each is that of its last
// command.
func wrappedCommands(cmd string) (string, bool) {
	if inner, ok := subshellBody(cmd); ok {
		return inner, true
	}
	for _, re := range []*regexp.Regexp{captureAssign, shellCommandString, braceGroup} {
		if m := re.FindStringSubmatch(cmd); m != nil {
			return strings.Join(m[1:], ""), true
		}
	}
	return "", false
}

// subshellBody returns the commands inside `( ... )` when the parentheses
// enclose the whole of cmd: `(a) | (b)` is a pipeline, not one subshell.
func subshellBody(cmd string) (string, bool) {
	if len(cmd) < 2 || cmd[0] != '(' || cmd[len(cmd)-1] != ')' {
		return "", false
	}
	sc := scanShell(cmd)
	for i := 1; i < len(cmd)-1; i++ {
		if sc.depth[i] == 0 {
			return "", false
		}
	}
	return cmd[1 : len(cmd)-1], true
}

// splitTopLevel splits s at separators outside quotes and parentheses.
// isSep returns the separator's length at i, or 0.
func splitTopLevel(s string, isSep func(s string, i int) int) []string {
	sc := scanShell(s)
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if sc.quoted[i] || sc.depth[i] != 0 {
			continue
		}
		if n := isSep(s, i); n > 0 {
			parts = append(parts, s[start:i])
			start = i + n
			i += n - 1
		}
	}
	return append(parts, s[start:])
}

func pipeSep(s string, i int) int {
	if s[i] == '|' && (i+1 >= len(s) || s[i+1] != '|') && (i == 0 || s[i-1] != '|') {
		return 1
	}
	return 0
}

func listSep(s string, i int) int {
	switch {
	case strings.HasPrefix(s[i:], "&&"), strings.HasPrefix(s[i:], "||"):
		return 2
	case s[i] == ';' || s[i] == '\n':
		return 1
	case s[i] == '&' && ampersandIsOperator(s, i):
		return 1 // a background job: the list goes on without its status
	}
	return 0
}

// swallowedVerification reports whether the command whose status is
// swallowed is a verification command, unwrapping VAR=$(cmd) captures.
func swallowedVerification(cmd string, extra []string) (string, bool) {
	for depth := 0; depth < 4; depth++ {
		inner, ok := wrappedCommands(strings.TrimSpace(cmd))
		if !ok {
			break
		}
		parts := splitTopLevel(inner, listSep)
		cmd = parts[len(parts)-1]
	}
	cmd = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cmd), "("))
	for _, stage := range splitTopLevel(cmd, pipeSep) {
		if name, ok := verificationCommand(strings.Fields(stage), extra); ok {
			return name, true
		}
	}
	return "", false
}
