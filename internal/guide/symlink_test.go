package guide

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// outsideSecret is what every file outside the project holds. A guide run that
// follows a link would overwrite it, or copy it into the repository.
const outsideSecret = "OUTSIDE-SECRET-MATERIAL\n"

// snapshotDir returns every regular file under dir, keyed by relative path.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out[rel] = string(data)
		return nil
	}))
	return out
}

// assertNoLeak checks that no file in the project holds the outside content.
func assertNoLeak(t *testing.T, project string) {
	t.Helper()
	_ = filepath.WalkDir(project, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.Type()&fs.ModeSymlink != 0 || d.IsDir() {
			return nil //nolint:nilerr // a walk error only hides files from this check
		}
		data, readErr := os.ReadFile(p)
		if readErr == nil {
			assert.NotContains(t, string(data), "OUTSIDE-SECRET", "outside content leaked into %s", p)
		}
		return nil
	})
}

type symlinkCase struct {
	name    string
	target  string
	link    string // project-relative path that becomes a symlink
	linkDir bool   // link points to an outside directory instead of a file
	catalog bool
	setup   func(t *testing.T, project string)
}

func symlinkCases() []symlinkCase {
	writeYAML := func(t *testing.T, project string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(project, "verikt.yaml"), []byte("architecture: hexagonal\n"), 0o644))
	}
	return []symlinkCase{
		{name: "AGENTS.md file", target: "agents", link: "AGENTS.md"},
		{name: "copilot file", target: "copilot", link: filepath.Join(".github", "copilot-instructions.md")},
		{name: "copilot parent dir", target: "copilot", link: ".github", linkDir: true},
		{name: "windsurf file", target: "windsurf", link: ".windsurfrules"},
		{name: "cursor mdc file", target: "cursor", link: filepath.Join(".cursor", "rules", "verikt.mdc")},
		{name: "cursor parent dir", target: "cursor", link: ".cursor", linkDir: true},
		{name: "legacy cursorrules", target: "cursor", link: legacyCursorRules},
		{name: "claude index file", target: "claude", link: filepath.Join(".claude", "rules", "verikt-index.md")},
		{name: "claude category file", target: "claude", link: filepath.Join(".claude", "rules", "verikt-http.md")},
		{name: "claude rules dir", target: "claude", link: filepath.Join(".claude", "rules"), linkDir: true},
		{name: "claude catalog file", target: "claude", link: filepath.Join(".claude", "rules", "verikt.md"), catalog: true},
		{name: "check hook file", target: "claude", link: filepath.Join(".claude", "hooks", "verikt-check.sh")},
		{name: "refresh hook file", target: "claude", link: filepath.Join(".claude", "hooks", "verikt-guide-refresh.sh")},
		{name: "hooks dir", target: "claude", link: filepath.Join(".claude", "hooks"), linkDir: true},
		{name: "settings.json", target: "claude", link: filepath.Join(".claude", "settings.json")},
		{name: "claude dir", target: "claude", link: ".claude", linkDir: true},
		{name: "reference file", target: "agents", link: filepath.Join(".verikt", "references", "review.md")},
		{name: "stale reference file", target: "agents", link: filepath.Join(".verikt", "references", "interview.md")},
		{name: "references dir", target: "agents", link: filepath.Join(".verikt", "references"), linkDir: true},
		{name: "verikt dir", target: "agents", link: ".verikt", linkDir: true},
		{name: "guide hash", target: "claude", link: filepath.Join(".verikt", ".guide-hash"), setup: writeYAML},
	}
}

// RT-F1 / SEC-F2: verikt guide must refuse any project path whose target or a
// parent inside the project is a symlink. Following one overwrote the file a
// hook script linked to, and copied a linked AGENTS.md's target into the repo.
func TestGenerate_RefusesSymlinkedPaths(t *testing.T) {
	for _, tc := range symlinkCases() {
		t.Run(tc.name, func(t *testing.T) {
			project, outside := t.TempDir(), t.TempDir()
			linkTarget := filepath.Join(outside, "victim")
			if tc.linkDir {
				require.NoError(t, os.MkdirAll(linkTarget, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(linkTarget, "keep.txt"), []byte(outsideSecret), 0o644))
			} else {
				require.NoError(t, os.WriteFile(linkTarget, []byte(outsideSecret), 0o644))
			}
			linkPath := filepath.Join(project, tc.link)
			require.NoError(t, os.MkdirAll(filepath.Dir(linkPath), 0o755))
			require.NoError(t, os.Symlink(linkTarget, linkPath))
			if tc.setup != nil {
				tc.setup(t, project)
			}
			before := snapshotDir(t, outside)

			err := Generate(GenerateOptions{
				ProjectDir: project, Target: tc.target, CatalogOnly: tc.catalog,
				Architecture: "hexagonal", Capabilities: []string{"http-api"},
			})

			require.Error(t, err, "a symlinked %s must be refused", tc.link)
			assert.True(t, errors.Is(err, ErrSymlinkInProject), "got %v", err)
			assert.Contains(t, err.Error(), filepath.ToSlash(tc.link), "the error must name the path")
			assert.Equal(t, before, snapshotDir(t, outside), "the file outside the project must be untouched")
			info, lerr := os.Lstat(linkPath)
			require.NoError(t, lerr)
			assert.NotZero(t, info.Mode()&fs.ModeSymlink, "the link must not be replaced by a regular file")
			assertNoLeak(t, project)
		})
	}
}

// A symlink that stays inside the project is refused too: these are files
// verikt owns, and writing through a link would modify some other file.
func TestGenerate_RefusesInProjectSymlink(t *testing.T) {
	project := t.TempDir()
	other := filepath.Join(project, "docs", "notes.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(other), 0o755))
	require.NoError(t, os.WriteFile(other, []byte("my notes\n"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join("docs", "notes.md"), filepath.Join(project, "AGENTS.md")))

	err := generateTarget(t, project, "agents")

	require.ErrorIs(t, err, ErrSymlinkInProject)
	data, rerr := os.ReadFile(other)
	require.NoError(t, rerr)
	assert.Equal(t, "my notes\n", string(data))
}

// WriteProjectFile is the guarded writer other commands (verikt setup's local
// skill install) use for project-scoped files.
func TestWriteProjectFile_RefusesSymlinkedParent(t *testing.T) {
	project, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(project, ".claude")))

	err := WriteProjectFile(project, filepath.Join(".claude", "skills", "x", "SKILL.md"), []byte("skill"))

	require.ErrorIs(t, err, ErrSymlinkInProject)
	assert.Contains(t, err.Error(), ".claude")
	assert.Empty(t, snapshotDir(t, outside))
}

func TestWriteProjectFile_WritesRegularFile(t *testing.T) {
	project := t.TempDir()
	rel := filepath.Join(".claude", "skills", "x", "SKILL.md")

	require.NoError(t, WriteProjectFile(project, rel, []byte("skill")))

	data, err := os.ReadFile(filepath.Join(project, rel))
	require.NoError(t, err)
	assert.Equal(t, "skill", string(data))
}
