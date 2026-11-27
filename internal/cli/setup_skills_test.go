package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/verikt/internal/guide"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RT-F1: the local skill install writes into the project, so a symlinked
// .claude must be refused rather than followed out of the project.
func TestInstallLocalSkill_RefusesSymlinkedClaudeDir(t *testing.T) {
	project, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(project, ".claude")))
	t.Chdir(project)

	err := installLocalSkill(&cobra.Command{}, "verikt:init", "skill body")

	require.ErrorIs(t, err, guide.ErrSymlinkInProject)
	entries, rerr := os.ReadDir(outside)
	require.NoError(t, rerr)
	assert.Empty(t, entries, "nothing may be written through the link")
}

func TestInstallLocalSkill_WritesSkillFile(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)

	require.NoError(t, installLocalSkill(&cobra.Command{}, "verikt:init", "skill body"))

	data, err := os.ReadFile(filepath.Join(project, ".claude", "skills", "verikt:init", "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "skill body", string(data))
}
