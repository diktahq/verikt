package scriptguard

import (
	"fmt"
	"regexp"
	"strings"
)

// RuleInvalidAllowDirective is reported for a verikt:allow comment that does
// not parse, has a blank reason, or names a rule that does not accept inline
// markers. Such a comment suppresses nothing (fail closed).
const RuleInvalidAllowDirective = "invalid_allow_directive"

// allowToken is what makes a comment an allow marker. Any comment containing
// it must be a well-formed marker (fail closed).
const allowToken = "verikt:allow"

// CommentSyntax is a set of comment openers recognised on a line.
type CommentSyntax uint8

const (
	// CommentHash is a "#" comment at the start of a line or after whitespace.
	CommentHash CommentSyntax = 1 << iota
	// CommentSlash is a "//" comment at the start of a line or after whitespace.
	CommentSlash
	// CommentHTML is a "<!-- ... -->" comment closed on the same line.
	CommentHTML

	// AllComments recognises every supported comment form.
	AllComments = CommentHash | CommentSlash | CommentHTML
)

// markerBody is the grammar of the text inside the comment:
//
//	verikt:allow <rule_id> reason="<text>"
//
// rule_id is lower snake case; the reason is double-quoted, with \" and \\
// as the only escapes.
var markerBody = regexp.MustCompile(`^verikt:allow[ \t]+([a-z][a-z0-9_]*)[ \t]+reason="((?:[^"\\]|\\["\\])*)"$`)

// AllowMarkers holds the valid verikt:allow markers of one file, for
// line-based rules to consult before reporting a finding.
type AllowMarkers struct {
	rules       map[int][]string // line -> rule ids allowed by a marker on that line
	commentOnly map[int]bool     // lines holding nothing but a comment
}

// ParseAllowMarkers finds every verikt:allow marker in lines (lines[0] is
// line 1). syntax reports which comment openers apply to a line; nil means
// AllComments everywhere. known is the set of rule ids that honour inline
// markers.
//
// Valid markers are returned in the AllowMarkers. Every comment that mentions
// verikt:allow but is malformed, has a blank reason, or names a rule outside
// known yields an invalid_allow_directive error at its line and allows
// nothing.
func ParseAllowMarkers(file string, lines []string, syntax func(line int) CommentSyntax, known map[string]bool) (*AllowMarkers, []Finding) {
	m := &AllowMarkers{rules: map[int][]string{}, commentOnly: map[int]bool{}}
	var invalid []Finding
	for i, line := range lines {
		n := i + 1
		syn := AllComments
		if syntax != nil {
			syn = syntax(n)
		}
		if isCommentOnly(line, syn) {
			m.commentOnly[n] = true
		}
		body, ok := markerComment(line, syn)
		if !ok {
			continue
		}
		rule, problem := parseMarkerBody(body, known)
		if problem != "" {
			invalid = append(invalid, Finding{
				File: file,
				Line: n,
				Message: fmt.Sprintf("%s; it suppresses nothing. Use: verikt:allow <rule_id> reason=\"<why>\" (rules: silent_ci_swallow, mutable_install_url)",
					problem),
				Rule: RuleInvalidAllowDirective,
			})
			continue
		}
		m.rules[n] = append(m.rules[n], rule)
	}
	return m, invalid
}

// Allows reports whether rule is allowed for a finding spanning lines first
// to last: by a trailing marker on any of those lines, or by a marker in the
// contiguous comment-only lines directly above first.
func (m *AllowMarkers) Allows(rule string, first, last int) bool {
	if m == nil {
		return false
	}
	for n := first; n <= last; n++ {
		if m.has(n, rule) {
			return true
		}
	}
	for n := first - 1; n >= 1 && m.commentOnly[n]; n-- {
		if m.has(n, rule) {
			return true
		}
	}
	return false
}

func (m *AllowMarkers) has(line int, rule string) bool {
	for _, r := range m.rules[line] {
		if r == rule {
			return true
		}
	}
	return false
}

// parseMarkerBody validates the comment text. It returns the rule id, or a
// description of what is wrong.
func parseMarkerBody(body string, known map[string]bool) (rule, problem string) {
	match := markerBody.FindStringSubmatch(strings.TrimSpace(body))
	if match == nil {
		return "", fmt.Sprintf("malformed %s comment %q", allowToken, strings.TrimSpace(body))
	}
	rule = match[1]
	reason := strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(match[2])
	if strings.TrimSpace(reason) == "" {
		return "", fmt.Sprintf("%s %s has a blank reason", allowToken, rule)
	}
	if !known[rule] {
		return "", fmt.Sprintf("%s names unknown rule %q", allowToken, rule)
	}
	return rule, ""
}

// markerComment returns the text of the comment that contains the allow
// token, or false when the token is not inside a comment of the given
// syntax.
func markerComment(line string, syn CommentSyntax) (string, bool) {
	tok := strings.Index(line, allowToken)
	if tok < 0 {
		return "", false
	}
	start, end, ok := commentStart(line[:tok], syn)
	if !ok {
		return "", false
	}
	body := line[end:]
	if strings.HasPrefix(line[start:], "<!--") {
		inner, after, closed := strings.Cut(body, "-->")
		if !closed || strings.TrimSpace(after) != "" {
			// Unclosed, or text after "-->": make it fail the grammar.
			return body + " <unclosed or trailing text after -->>", true
		}
		body = inner
	}
	return body, true
}

// commentStart finds the first comment opener in s (the text before the
// allow token) that sits outside quotes. It returns the opener's start and
// the index just past it.
func commentStart(s string, syn CommentSyntax) (start, end int, ok bool) {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' && quote == '"' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch {
		case (c == '"' || c == '\'') && opensQuote(s, i):
			quote = c
		case syn&CommentHTML != 0 && strings.HasPrefix(s[i:], "<!--"):
			return i, i + 4, true
		case syn&CommentHash != 0 && c == '#' && atWordStart(s, i):
			return i, i + 1, true
		case syn&CommentSlash != 0 && strings.HasPrefix(s[i:], "//") && atWordStart(s, i):
			return i, i + 2, true
		}
	}
	return 0, 0, false
}

// isCommentOnly reports whether line holds only a comment of syntax syn.
func isCommentOnly(line string, syn CommentSyntax) bool {
	t := strings.TrimSpace(line)
	switch {
	case t == "":
		return false
	case syn&CommentHash != 0 && strings.HasPrefix(t, "#"):
		return true
	case syn&CommentSlash != 0 && strings.HasPrefix(t, "//"):
		return true
	case syn&CommentHTML != 0 && strings.HasPrefix(t, "<!--") && strings.HasSuffix(t, "-->"):
		return true
	}
	return false
}

// atWordStart reports whether s[i] begins the line or follows whitespace.
func atWordStart(s string, i int) bool {
	return i == 0 || s[i-1] == ' ' || s[i-1] == '\t'
}

// opensQuote reports whether the quote character at s[i] starts a quoted
// string rather than being an apostrophe inside a word ("project's").
func opensQuote(s string, i int) bool {
	if i == 0 {
		return true
	}
	p := s[i-1]
	return (p < 'a' || p > 'z') && (p < 'A' || p > 'Z') && (p < '0' || p > '9')
}
