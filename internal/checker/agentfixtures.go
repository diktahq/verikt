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
// settings.local.json is not a marker: Claude Code reads it from the project
// root only, and there is no evidence it loads a nested one.
var liveAgentConfigDirs = []agentConfigDir{
	{name: ".claude", markers: map[string]bool{
		"skills": true, "rules": true, "hooks": true, "settings.json": true,
		"claude.md": true, "agents": true, "commands": true,
	}},
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

// agentMemoryFile is the instruction file Claude Code loads from any
// directory an agent works in.
const agentMemoryFile = "claude.md"

// agentFixtureSkipDirs are never descended into: version-control metadata and
// third-party trees the author does not control (INV-005).
var agentFixtureSkipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true}

// detectLiveAgentFixtures reports each agent configuration directory (.claude/)
// that sits under a test, fixture or eval path and directly contains something
// an agent auto-loads (skills, rules, hooks, agents, commands, settings.json,
// CLAUDE.md), and each standalone CLAUDE.md inside a fixture case. One finding
// per directory or file.
//
// The risk is reachable by construction: an agent opened anywhere in the
// repository walks up and loads these, so the test subject becomes the
// instructions of whoever is working on the tests. A .claude/ with none of the
// markers loads nothing and is not reported.
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

// isFixtureMemoryFile reports whether rel is a CLAUDE.md with a case
// directory between it and a data fixture segment, as in
// testdata/<case>/CLAUDE.md.
func isFixtureMemoryFile(rel string) bool {
	segments := strings.Split(rel, "/")
	n := len(segments)
	if strings.ToLower(segments[n-1]) != agentMemoryFile {
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
// auto-loads. An unreadable directory has shown no evidence, so it is not
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
// agentDirName, or a standalone CLAUDE.md when agentDirName is empty.
func liveAgentFixtureViolation(rel, agentDirName string) Violation {
	var msg string
	if agentDirName == "" {
		stored := path.Join(path.Dir(rel), "CLAUDE.fixture.md")
		msg = fmt.Sprintf("%s is a live %s under a test fixture: agents working in this repository "+
			"will load it as real instructions — commit it as %s and copy it into a temporary directory at test time",
			rel, path.Base(rel), stored)
	} else {
		stored := path.Join(path.Dir(rel), "dot-"+strings.TrimPrefix(agentDirName, ".")) + "/"
		msg = fmt.Sprintf("%s is a live %s/ directory under a test or fixture path: agents working in this repository "+
			"will load it as real instructions — commit it as %s and copy it to %s/ inside a temporary directory at test time",
			rel, agentDirName, stored, agentDirName)
	}
	return Violation{Category: "structure", File: rel, Message: msg, Rule: RuleLiveDotClaudeFixture, Severity: "warning"}
}
