package guide

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTarget(t *testing.T, dir, target string) error {
	t.Helper()
	return Generate(GenerateOptions{
		ProjectDir:   dir,
		Target:       target,
		Architecture: "hexagonal",
		Capabilities: []string{"http-api"},
	})
}

// --- AGENTS.md ---

func TestAgentsTarget_CreatesFileWithMarkedSection(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, generateTarget(t, dir, "agents"))

	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	content := string(data)

	assert.True(t, strings.HasPrefix(content, sentinelStart+"\n"))
	assert.True(t, strings.HasSuffix(content, sentinelEnd+"\n"))
	assert.Contains(t, content, "Architecture: hexagonal")
	assert.Contains(t, content, ".verikt/references/")
}

func TestAgentsTarget_PreservesUserContentInExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	user := "# Project agents\n\nUse pnpm, not npm.\n"
	require.NoError(t, os.WriteFile(path, []byte(user), 0o644))

	require.NoError(t, generateTarget(t, dir, "agents"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.True(t, strings.HasPrefix(content, user), "user content must stay first and unchanged")
	assert.Contains(t, content, sentinelStart)
	assert.Contains(t, content, "Architecture: hexagonal")
}

func TestAgentsTarget_ReplacesOnlyVeriktSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	before := "# Top\n\nuser rules above\n\n"
	after := "\n## Below\n\nuser rules below\n"
	existing := before + sentinelStart + "\nstale verikt rules\n" + sentinelEnd + "\n" + after
	require.NoError(t, os.WriteFile(path, []byte(existing), 0o644))

	require.NoError(t, generateTarget(t, dir, "agents"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.True(t, strings.HasPrefix(content, before))
	assert.True(t, strings.HasSuffix(content, sentinelEnd+"\n"+after))
	assert.NotContains(t, content, "stale verikt rules")
	assert.Equal(t, 1, strings.Count(content, sentinelStart))
}

func TestAgentsTarget_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	require.NoError(t, os.WriteFile(path, []byte("user content\n"), 0o644))

	require.NoError(t, generateTarget(t, dir, "agents"))
	first, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, generateTarget(t, dir, "agents"))
	second, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second))
}

func TestAgentsTarget_MalformedMarkersFailClosed(t *testing.T) {
	cases := map[string]string{
		"start only":    "user\n" + sentinelStart + "\nhalf a section\n",
		"end only":      "user\n" + sentinelEnd + "\n",
		"end first":     sentinelEnd + "\nuser\n" + sentinelStart + "\n",
		"two sections":  sentinelStart + "\na\n" + sentinelEnd + "\n" + sentinelStart + "\nb\n" + sentinelEnd + "\n",
		"two start one": sentinelStart + "\n" + sentinelStart + "\n" + sentinelEnd + "\n",
	}
	for name, existing := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "AGENTS.md")
			require.NoError(t, os.WriteFile(path, []byte(existing), 0o644))

			err := generateTarget(t, dir, "agents")
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrMalformedSection), "got %v", err)

			data, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, existing, string(data), "file must be left untouched")
		})
	}
}

// --- Cursor ---

func TestCursorTarget_WritesMDCWithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, generateTarget(t, dir, "cursor"))

	data, err := os.ReadFile(filepath.Join(dir, ".cursor", "rules", "verikt.mdc"))
	require.NoError(t, err)
	content := string(data)

	require.True(t, strings.HasPrefix(content, "---\n"))
	end := strings.Index(content[4:], "\n---\n")
	require.GreaterOrEqual(t, end, 0, "frontmatter must be closed")
	frontmatter := content[4 : 4+end]
	assert.Contains(t, frontmatter, "description: ")
	assert.Contains(t, frontmatter, "globs:")
	assert.Contains(t, frontmatter, "alwaysApply: true")
	assert.Contains(t, content, "Architecture: hexagonal")
	assert.NoFileExists(t, filepath.Join(dir, ".cursorrules"))
}

func TestCursorTarget_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursor", "rules", "verikt.mdc")
	require.NoError(t, generateTarget(t, dir, "cursor"))
	first, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, generateTarget(t, dir, "cursor"))
	second, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
}

func generateCursorWithNotices(t *testing.T, dir string) string {
	t.Helper()
	var notices bytes.Buffer
	require.NoError(t, Generate(GenerateOptions{
		ProjectDir:   dir,
		Target:       "cursor",
		Architecture: "hexagonal",
		Notices:      &notices,
	}))
	return notices.String()
}

func TestCursorTarget_LegacyOnlyVeriktSectionIsRemoved(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, ".cursorrules")
	require.NoError(t, os.WriteFile(legacy, []byte(sentinelStart+"\nold verikt rules\n"+sentinelEnd+"\n"), 0o644))

	notices := generateCursorWithNotices(t, dir)

	assert.NoFileExists(t, legacy)
	assert.Contains(t, notices, "removed legacy .cursorrules")
	assert.FileExists(t, filepath.Join(dir, ".cursor", "rules", "verikt.mdc"))
}

func TestCursorTarget_LegacyUserContentIsKept(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, ".cursorrules")
	user := "Always use tabs.\n"
	require.NoError(t, os.WriteFile(legacy, []byte(user+"\n"+sentinelStart+"\nold verikt rules\n"+sentinelEnd+"\n"), 0o644))

	notices := generateCursorWithNotices(t, dir)

	data, err := os.ReadFile(legacy)
	require.NoError(t, err)
	assert.Equal(t, user, string(data), "only verikt's section is stripped")
	assert.Contains(t, notices, "your own content")
}

func TestCursorTarget_LegacyWithoutVeriktSectionIsUntouched(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, ".cursorrules")
	user := "Pure user rules.\n"
	require.NoError(t, os.WriteFile(legacy, []byte(user), 0o644))

	notices := generateCursorWithNotices(t, dir)

	data, err := os.ReadFile(legacy)
	require.NoError(t, err)
	assert.Equal(t, user, string(data))
	assert.Empty(t, notices)
}

func TestCursorTarget_LegacyMalformedIsUntouchedWithNotice(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, ".cursorrules")
	existing := "user rules\n" + sentinelStart + "\nverikt without an end marker\n"
	require.NoError(t, os.WriteFile(legacy, []byte(existing), 0o644))

	notices := generateCursorWithNotices(t, dir)

	data, err := os.ReadFile(legacy)
	require.NoError(t, err)
	assert.Equal(t, existing, string(data))
	assert.Contains(t, notices, "left .cursorrules untouched")
	assert.FileExists(t, filepath.Join(dir, ".cursor", "rules", "verikt.mdc"))
}

// --- References ---

func TestReferences_StaleArchitectureRemovedInCatalogOnly(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, generateTarget(t, dir, "claude"))
	arch := filepath.Join(dir, ".verikt", "references", "architecture.md")
	require.FileExists(t, arch)

	require.NoError(t, Generate(GenerateOptions{ProjectDir: dir, Target: "claude", CatalogOnly: true}))
	assert.NoFileExists(t, arch)
	assert.FileExists(t, filepath.Join(dir, ".verikt", "references", "interview.md"))
}

// The scaffold interview is onboarding for a project with no verikt.yaml, so it
// is produced only in catalog-only output (INV-001), and there as an on-demand
// reference rather than in the always-loaded file.
func TestReferences_InterviewOnlyInCatalogOnlyAndOnDemand(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Generate(GenerateOptions{ProjectDir: dir, Target: "claude", CatalogOnly: true}))

	always, err := os.ReadFile(filepath.Join(dir, ".claude", "rules", "verikt.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(always), "## AI Interview")
	assert.Contains(t, string(always), ".verikt/references/interview.md")

	interview, err := os.ReadFile(filepath.Join(dir, ".verikt", "references", "interview.md"))
	require.NoError(t, err)
	assert.Contains(t, string(interview), "## AI Interview: Scaffold a New Service")

	// A guide generated from verikt.yaml has nothing to scaffold: the stale
	// interview reference is removed and the index does not point to it.
	require.NoError(t, generateTarget(t, dir, "claude"))
	assert.NoFileExists(t, filepath.Join(dir, ".verikt", "references", "interview.md"))
	index, err := os.ReadFile(filepath.Join(dir, ".claude", "rules", "verikt-index.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(index), "interview.md")
	assert.NotContains(t, string(index), "## AI Interview")
}

// References are derived from verikt.yaml alone, so the same options produce
// byte-identical references on every run (ADR-008).
func TestReferences_Reproducible(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	require.NoError(t, generateTarget(t, dirA, "claude"))
	require.NoError(t, generateTarget(t, dirB, "claude"))
	for _, name := range []string{"architecture.md", "capabilities.md", "review.md"} {
		a, err := os.ReadFile(filepath.Join(dirA, ".verikt", "references", name))
		require.NoError(t, err)
		b, err := os.ReadFile(filepath.Join(dirB, ".verikt", "references", name))
		require.NoError(t, err)
		assert.Equal(t, string(a), string(b), name)
	}
}

// --- Section helpers ---

func TestRemoveSection_RestoresOriginalAfterReplace(t *testing.T) {
	for _, original := range []string{"", "user\n", "user without newline"} {
		withSection, err := ReplaceSection(original, GuideMarkers, "body\n")
		require.NoError(t, err)
		out, removed, err := RemoveSection(withSection, GuideMarkers)
		require.NoError(t, err)
		assert.True(t, removed)
		assert.Equal(t, strings.TrimSuffix(original, "\n"), strings.TrimSuffix(out, "\n"))
	}
}

// LLM-F1: a body carrying verikt's own markers (for example from a component
// name in verikt.yaml) would leave the file with several sections, and every
// later run would refuse it as malformed. ReplaceSection refuses it up front.
func TestReplaceSection_RefusesBodyContainingMarkers(t *testing.T) {
	for _, marker := range []string{sentinelStart, sentinelEnd} {
		_, err := ReplaceSection("user content\n", GuideMarkers, "rules\n"+marker+"\nmore\n")
		require.ErrorIs(t, err, ErrMarkerInBody, "body with %q", marker)
		assert.Contains(t, err.Error(), marker)
	}
}

func TestAgentsTarget_MarkerInConfigLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	existing := "# Mine\n"
	require.NoError(t, os.WriteFile(path, []byte(existing), 0o644))

	err := Generate(GenerateOptions{
		ProjectDir: dir, Target: "agents", Architecture: "hexagonal",
		Components: []config.Component{{Name: "domain " + sentinelEnd, In: []string{"domain/**"}}},
	})

	require.ErrorIs(t, err, ErrMarkerInBody)
	data, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, existing, string(data))
}
