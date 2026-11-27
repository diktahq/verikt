package checker

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// RuleLiveDotClaudeFixture is the rule, and severity_overrides key, for an
// agent configuration directory committed under a test, fixture or eval path.
const RuleLiveDotClaudeFixture = "live_dot_claude_fixture"

// agentConfigDir is an agent configuration directory that a coding agent loads
// automatically, and the entries inside it that make it load-bearing.
type agentConfigDir struct {
	name    string          // directory name, lower case, e.g. ".claude"
	markers map[string]bool // direct children the agent auto-loads, lower case
}

// liveAgentConfigDirs lists the agent directories this detector recognises.
//
// Only rules/ and skills/ are markers: they are the entries Claude Code
// documents loading from a .claude/ below the working directory, on demand,
// once Claude reads or edits a file in that subtree (code.claude.com/docs/en/
// memory.md for rules, skills.md for skills). Everything else is not reachable
// from a nested .claude/ and is not a marker (INV-005):
//   - agents/: subagents are discovered walking up from the working directory,
//     never down (sub-agents.md);
//   - settings.json, settings.local.json, and hooks/ scripts, which run only
//     when a settings hook names them: settings are read from the session's
//     primary working directory only (settings.md, hooks-guide.md);
//   - commands/ and .claude/CLAUDE.md: not documented to load from a nested
//     directory (claude-directory.md, memory.md).
var liveAgentConfigDirs = []agentConfigDir{
	{name: ".claude", markers: map[string]bool{"skills": true, "rules": true}},
}

// fixturePathSegments are path segments that mark test, fixture or eval
// content. A segment must match whole, ignoring case: "latest" and "mytests"
// do not. Case is ignored because the filesystems most agents run on (macOS,
// Windows) ignore it.
var fixturePathSegments = map[string]bool{
	"test": true, "tests": true, "testdata": true, "fixtures": true, "evals": true, "examples": true,
	"__tests__": true, "__fixtures__": true,
}

// dataFixtureSegments hold test subjects, not code. A standalone CLAUDE.md is
// reported only inside a case directory below one of these: directly in
// testdata/, or anywhere under tests/ or examples/, it is plausibly the team's
// guidance for working there (INV-005).
var dataFixtureSegments = map[string]bool{
	"testdata": true, "fixtures": true, "__fixtures__": true, "evals": true,
}

// agentMemoryFiles are the instruction files, lower case, that Claude Code
// loads on demand from a subdirectory once Claude reads, writes or edits a
// file there (code.claude.com/docs/en/memory.md).
var agentMemoryFiles = map[string]bool{"claude.md": true, "claude.local.md": true}

// agentFixtureSkipDirs are never descended into: version-control metadata and
// third-party trees the author does not control (INV-005).
var agentFixtureSkipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true}

// detectLiveAgentFixtures reports each agent configuration directory (.claude/)
// that sits under a test, fixture or eval path and directly contains rules/ or
// skills/, and each standalone CLAUDE.md or CLAUDE.local.md inside a fixture
// case. One finding per directory or file.
//
// The risk is reachable as documented: Claude Code started at the repository
// root loads nested CLAUDE.md, CLAUDE.local.md, .claude/rules/ and
// .claude/skills/ on demand, the first time Claude reads or edits a file in
// that subdirectory (code.claude.com/docs/en/memory.md, skills.md). An agent
// working on the fixture therefore takes the test subject as its own
// instructions. A .claude/ holding only entries that are not loaded from a
// nested directory (agents/, commands/, hooks/, settings files, CLAUDE.md) is
// not reported.
//
// It runs for every language, honours check.exclude, never follows symlinks
// (INV-002), and skips what it cannot read rather than failing the check.
func detectLiveAgentFixtures(projectPath string, excludes []string) []Violation {
	var violations []Violation
	// Per-entry errors are skipped inside the callback; an unreadable path is
	// reported by detectUnreadablePaths, and a failed walk has found nothing.
	_ = filepath.WalkDir(projectPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		// Before the directory filter: WalkDir reports a symlink from Lstat, so
		// IsDir() is false even for a link to a directory (INV-002).
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, relErr := filepath.Rel(projectPath, p)
		if relErr != nil || rel == "." {
			return nil //nolint:nilerr // p is under projectPath; Rel cannot fail
		}
		rel = filepath.ToSlash(rel)
		if !d.IsDir() {
			if d.Type().IsRegular() && isFixtureMemoryFile(rel) && !isExcluded(rel, excludes) {
				violations = append(violations, liveAgentFixtureViolation(rel, ""))
			}
			return nil
		}
		if agentFixtureSkipDirs[d.Name()] || isExcluded(rel, excludes) {
			return filepath.SkipDir
		}
		agentDir, ok := lookupAgentConfigDir(d.Name())
		if !ok {
			return nil
		}
		// An agent directory is a leaf for this rule: reported at most once,
		// never descended into.
		if underFixturePath(rel) && hasLiveMarker(p, agentDir) {
			violations = append(violations, liveAgentFixtureViolation(rel, agentDir.name))
		}
		return filepath.SkipDir
	})
	return violations
}

func lookupAgentConfigDir(name string) (agentConfigDir, bool) {
	for _, a := range liveAgentConfigDirs {
		if a.name == strings.ToLower(name) {
			return a, true
		}
	}
	return agentConfigDir{}, false
}

// underFixturePath reports whether any ancestor segment of rel — a
// slash-separated path to the agent directory — is a fixture segment.
func underFixturePath(rel string) bool {
	segments := strings.Split(rel, "/")
	for _, s := range segments[:len(segments)-1] {
		if fixturePathSegments[strings.ToLower(s)] {
			return true
		}
	}
	return false
}

// isFixtureMemoryFile reports whether rel is a CLAUDE.md or CLAUDE.local.md
// with a case directory between it and a data fixture segment, as in
// testdata/<case>/CLAUDE.md.
func isFixtureMemoryFile(rel string) bool {
	segments := strings.Split(rel, "/")
	n := len(segments)
	if !agentMemoryFiles[strings.ToLower(segments[n-1])] {
		return false
	}
	for _, s := range segments[:max(n-2, 0)] {
		if dataFixtureSegments[strings.ToLower(s)] {
			return true
		}
	}
	return false
}

// hasLiveMarker reports whether dir directly contains an entry the agent
// loads from a nested directory. An unreadable directory has shown no evidence, so it is not
// reported (INV-005).
func hasLiveMarker(dir string, agentDir agentConfigDir) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if agentDir.markers[strings.ToLower(e.Name())] {
			return true
		}
	}
	return false
}

// liveAgentFixtureViolation reports rel: an agent directory named
// agentDirName, or a standalone CLAUDE.md or CLAUDE.local.md when agentDirName
// is empty.
func liveAgentFixtureViolation(rel, agentDirName string) Violation {
	var msg string
	if agentDirName == "" {
		base := path.Base(rel)
		stored := path.Join(path.Dir(rel), base[:len(base)-len(".md")]+".fixture.md")
		msg = fmt.Sprintf("%s is a %s under a test fixture; Claude Code loads it on demand when it reads or edits "+
			"a file in that directory, so an agent working on the fixture takes it as real instructions — "+
			"commit it as %s and copy it into place in a temporary directory at test time",
			rel, base, stored)
	} else {
		stored := path.Join(path.Dir(rel), "dot-"+strings.TrimPrefix(agentDirName, ".")) + "/"
		msg = fmt.Sprintf("%s holds Claude Code rules or skills under a fixture path; Claude Code loads them on demand "+
			"when it reads or edits a file in that directory, so an agent working on the fixture takes them as real "+
			"instructions — commit it as %s and copy it to %s/ in a temporary directory at test time",
			rel, stored, agentDirName)
	}
	return Violation{Category: "structure", File: rel, Message: msg, Rule: RuleLiveDotClaudeFixture, Severity: "warning"}
}
