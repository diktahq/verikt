package checker

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/config"
)

// Every fixture tree here is built at runtime inside t.TempDir(). Never commit a
// real .claude/ directory to this repository, not even under testdata: agents
// working here would load it as live instructions — the failure
// detectLiveAgentFixtures exists to catch.

// mkTree creates each relative path under root. Paths ending in "/" are
// directories; everything else is a small file, with parents created as needed.
func mkTree(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))
		if strings.HasSuffix(p, "/") {
			require.NoError(t, os.MkdirAll(full, 0o755))
			continue
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte("x"), 0o600))
	}
}

// fixtureFiles returns the sorted reported directories, asserting each finding
// has the shape the rest of the check pipeline relies on.
func fixtureFiles(t *testing.T, vs []Violation) []string {
	t.Helper()
	files := []string{}
	for _, v := range vs {
		if v.Rule != RuleLiveDotClaudeFixture {
			continue
		}
		assert.Equal(t, "warning", v.Severity, "a new detector ships at warning (INV-005)")
		assert.Equal(t, "structure", v.Category)
		assert.Contains(t, v.Message, "dot-claude/", "the message names the remedy")
		assert.Contains(t, v.Message, "t.TempDir()", "the message names the remedy")
		files = append(files, v.File)
	}
	sort.Strings(files)
	return files
}

func TestLiveAgentFixtures_ReportsLiveDirectoryUnderAFixturePath(t *testing.T) {
	cases := map[string]struct {
		tree []string
		want string
	}{
		"testdata skills":    {[]string{"testdata/x/.claude/skills/foo/SKILL.md"}, "testdata/x/.claude"},
		"fixtures settings":  {[]string{"fixtures/.claude/settings.json"}, "fixtures/.claude"},
		"evals nested rules": {[]string{"evals/a/b/.claude/rules/r.md"}, "evals/a/b/.claude"},
		"tests hooks":        {[]string{"tests/.claude/hooks/"}, "tests/.claude"},
		"test segment deep":  {[]string{"pkg/test/case/.claude/rules/"}, "pkg/test/case/.claude"},
		"examples settings":  {[]string{"examples/demo/.claude/settings.json"}, "examples/demo/.claude"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			mkTree(t, root, tc.tree...)
			assert.Equal(t, []string{tc.want}, fixtureFiles(t, detectLiveAgentFixtures(root, nil)))
		})
	}
}

func TestLiveAgentFixtures_IgnoresDirectoriesAgentsWouldNotMistakeForFixtures(t *testing.T) {
	cases := map[string][]string{
		"repo-root .claude":            {".claude/skills/a/SKILL.md", ".claude/settings.json"},
		"dot-claude under testdata":    {"testdata/x/dot-claude/skills/a/SKILL.md"},
		".claude under non-test dir":   {"internal/foo/.claude/rules/r.md"},
		"fixture .claude no markers":   {"testdata/x/.claude/README.md", "testdata/y/.claude/settings.local.json"},
		"segment substring only":       {"latest/.claude/rules/", "mytests/.claude/rules/"},
		"fixture segment below claude": {"pkg/.claude/skills/test/SKILL.md"},
	}
	for name, tree := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			mkTree(t, root, tree...)
			assert.Empty(t, fixtureFiles(t, detectLiveAgentFixtures(root, nil)))
		})
	}
}

func TestLiveAgentFixtures_SkipsGitNodeModulesAndVendor(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root,
		"node_modules/pkg/test/.claude/rules/",
		"vendor/x/testdata/.claude/skills/",
		".git/testdata/.claude/rules/",
	)

	assert.Empty(t, fixtureFiles(t, detectLiveAgentFixtures(root, nil)))
}

func TestLiveAgentFixtures_ReportsEachDirectoryOnce(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root,
		"testdata/x/.claude/skills/a/SKILL.md",
		"testdata/x/.claude/skills/b/SKILL.md",
		"testdata/x/.claude/rules/r.md",
		"testdata/x/.claude/hooks/h.sh",
		"testdata/x/.claude/settings.json",
		"testdata/y/.claude/rules/r.md",
	)

	assert.Equal(t, []string{"testdata/x/.claude", "testdata/y/.claude"},
		fixtureFiles(t, detectLiveAgentFixtures(root, nil)))
}

func TestLiveAgentFixtures_SkipsSymlinkedDirectories(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	mkTree(t, outside, "test/.claude/skills/a/SKILL.md")
	mkTree(t, root, "testdata/")
	if err := os.Symlink(outside, filepath.Join(root, "testdata", "link")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	assert.Empty(t, fixtureFiles(t, detectLiveAgentFixtures(root, nil)), "INV-002")
}

func TestLiveAgentFixtures_SymlinkedClaudeDirectoryIsNotReported(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	mkTree(t, outside, "skills/a/SKILL.md")
	mkTree(t, root, "testdata/case/")
	if err := os.Symlink(outside, filepath.Join(root, "testdata", "case", ".claude")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	assert.Empty(t, fixtureFiles(t, detectLiveAgentFixtures(root, nil)), "INV-002")
}

func TestLiveAgentFixtures_HonoursCheckExclude(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root,
		"testdata/keep/.claude/rules/r.md",
		"testdata/vendored/.claude/rules/r.md",
		"evals/a/.claude/skills/s/SKILL.md",
	)

	got := fixtureFiles(t, detectLiveAgentFixtures(root, []string{"testdata/vendored/**", "evals/a/.claude"}))

	assert.Equal(t, []string{"testdata/keep/.claude"}, got)
}

func TestLiveAgentFixtures_MissingRootReportsNothing(t *testing.T) {
	assert.Empty(t, detectLiveAgentFixtures(filepath.Join(t.TempDir(), "missing"), nil))
}

// The detector runs for every language: an agent loads .claude/ whatever the
// project is written in. These go through CheckWithEngine so they fail if the
// wiring is removed.
func TestCheckWithEngine_ReportsLiveAgentFixturesForGo(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "go.mod", "module example.com/demo\n\ngo 1.22\n")
	mkTree(t, dir, "testdata/case/.claude/skills/x/SKILL.md")
	cfg := &config.VeriktConfig{Language: "go", Architecture: "flat"}

	result, err := CheckWithEngine(cfg, dir,
		emptyAntiPatternClient, dependencyClientReturning(nil), emptyMetricClient)

	require.NoError(t, err)
	assert.Equal(t, []string{"testdata/case/.claude"}, fixtureFiles(t, result.StructureViolations))
}

func TestCheckWithEngine_ReportsLiveAgentFixturesForTypeScript(t *testing.T) {
	dir := typescriptProject(t)
	mkTree(t, dir, "tests/.claude/rules/r.md")
	cfg := &config.VeriktConfig{Language: "typescript", Architecture: "flat"}

	result, err := CheckWithEngine(cfg, dir, nil, dependencyClientReturning(nil), nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"tests/.claude"}, fixtureFiles(t, result.StructureViolations))
}

func TestCheckWithEngine_LiveAgentFixturesHonourCheckExclude(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "go.mod", "module example.com/demo\n\ngo 1.22\n")
	mkTree(t, dir, "testdata/case/.claude/skills/x/SKILL.md")
	cfg := &config.VeriktConfig{Language: "go", Architecture: "flat",
		Check: config.CheckConfig{Exclude: []string{"testdata/**"}}}

	result, err := CheckWithEngine(cfg, dir,
		emptyAntiPatternClient, dependencyClientReturning(nil), emptyMetricClient)

	require.NoError(t, err)
	assert.Empty(t, fixtureFiles(t, result.StructureViolations))
}
