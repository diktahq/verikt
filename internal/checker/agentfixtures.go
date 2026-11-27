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
	name    string          // directory name, e.g. ".claude"
	markers map[string]bool // direct children the agent auto-loads
}

// liveAgentConfigDirs lists the agent directories this detector recognises.
var liveAgentConfigDirs = []agentConfigDir{
	{name: ".claude", markers: map[string]bool{"skills": true, "rules": true, "hooks": true, "settings.json": true}},
}

// fixturePathSegments are path segments that mark test, fixture or eval
// content. A segment must match exactly: "latest" and "mytests" do not.
var fixturePathSegments = map[string]bool{
	"test": true, "tests": true, "testdata": true, "fixtures": true, "evals": true, "examples": true,
}

// agentFixtureSkipDirs are never descended into: version-control metadata and
// third-party trees the author does not control (INV-005).
var agentFixtureSkipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true}

// detectLiveAgentFixtures reports each agent configuration directory (.claude/)
// that sits under a test, fixture or eval path and directly contains something
// an agent auto-loads: skills, rules, hooks or settings.json. One finding per
// directory.
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
			return nil
		}
		// Before the directory filter: WalkDir reports a symlink from Lstat, so
		// IsDir() is false even for a link to a directory (INV-002).
		if d.Type()&fs.ModeSymlink != 0 || !d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(projectPath, p)
		if relErr != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
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
		if a.name == name {
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
		if fixturePathSegments[s] {
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
		if agentDir.markers[e.Name()] {
			return true
		}
	}
	return false
}

func liveAgentFixtureViolation(rel, agentDirName string) Violation {
	stored := path.Join(path.Dir(rel), "dot-"+strings.TrimPrefix(agentDirName, ".")) + "/"
	return Violation{
		Category: "structure",
		File:     rel,
		Message: fmt.Sprintf("%s is a live %s/ directory under a test or fixture path: agents working in this repository "+
			"will load it as real instructions — commit it as %s and copy it to %s/ inside t.TempDir() at test time",
			rel, agentDirName, stored, agentDirName),
		Rule:     RuleLiveDotClaudeFixture,
		Severity: "warning",
	}
}
