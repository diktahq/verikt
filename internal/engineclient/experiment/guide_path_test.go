package experiment

import (
	"testing"

	"github.com/diktahq/verikt/internal/guide"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LLM-F7: the harness read .cursorrules, which `verikt guide --target cursor`
// no longer writes, so every experiment that generates a guide failed. The
// harness must read the file the cursor target actually writes.
func TestReadCursorGuide_ReadsWhatTheCursorTargetWrites(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, guide.Generate(guide.GenerateOptions{
		ProjectDir: dir, Target: "cursor", Architecture: "hexagonal", Capabilities: []string{"http-api"},
	}))

	text := readCursorGuide(t, dir)

	assert.Contains(t, text, "Architecture: hexagonal")
}
