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
	// continueOnErrorTrue matches a literal `continue-on-error: true` (after
	// comments are stripped). Expressions like ${{ matrix.x }} are not literal.
	continueOnErrorTrue = regexp.MustCompile(`^\s*(?:-\s*)?continue-on-error\s*:\s*(?i:true|'true'|"true")\s*$`)

	// swallowTail matches what follows "||" in a swallow: true, ":" or exit 0.
	swallowTail = regexp.MustCompile(`^\s*(true|:|exit\s+0)(?:[\s;&|)}]|$)`)

	// exitStatusRead matches a read of $? (or $$? in a Makefile recipe).
	exitStatusRead = regexp.MustCompile(`\$\$?(?:\?|\{\?\})`)

	// captureAssign is VAR=$(cmd) or VAR=`cmd`, optionally declared.
	captureAssign = regexp.MustCompile("^(?:(?:export|local|readonly|declare|typeset)\\s+(?:-\\w+\\s+)*)?[A-Za-z_]\\w*=(?:\\$\\((.*)\\)|`(.*)`)$")

	envAssign = regexp.MustCompile(`^[A-Za-z_]\w*=`)
)

// scanSilentCISwallow applies silent_ci_swallow to f.
func scanSilentCISwallow(f *scriptFile, opts Options) []Finding {
	switch f.kind {
	case KindWorkflow, KindLefthook, KindShell, KindMakefile, KindCIConfig:
	default:
		return nil
	}
	var out []Finding
	for i, ll := range f.logical {
		if f.allowed(RuleSilentCISwallow, ll) {
			continue
		}
		if f.kind == KindWorkflow && continueOnErrorTrue.MatchString(ll.text) {
			out = append(out, f.violation(RuleSilentCISwallow, "continue-on-error: true lets this job or step fail while the workflow stays green. "+
				"Remove it, or mark a deliberate best-effort step with verikt:allow silent_ci_swallow reason=\"...\"", ll))
			continue
		}
		for _, s := range findSwallows(ll.cmd) {
			if msg, ok := swallowFinding(ll.cmd, s, nextText(f.logical, i), opts.ExtraCommands); ok {
				out = append(out, f.violation(RuleSilentCISwallow, msg, ll))
			}
		}
	}
	return out
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
	d0 := sc.depth[pos]
	for j := pos - 1; j >= 0; j-- {
		if sc.quoted[j] || sc.depth[j] > d0 {
			continue
		}
		if sc.depth[j] < d0 {
			return j + 1
		}
		switch text[j] {
		case ';', '&', '{', '}', '\n':
			return j + 1
		case '|':
			if j > 0 && text[j-1] == '|' {
				return j + 1
			}
		}
	}
	return 0
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
	}
	return 0
}

// swallowedVerification reports whether the command whose status is
// swallowed is a verification command, unwrapping VAR=$(cmd) captures.
func swallowedVerification(cmd string, extra []string) (string, bool) {
	for depth := 0; depth < 4; depth++ {
		cmd = strings.TrimSpace(cmd)
		m := captureAssign.FindStringSubmatch(cmd)
		if m == nil {
			break
		}
		inner := m[1] + m[2]
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
