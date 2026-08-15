package engineclient

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// detectorCountClaim matches a documented detector count, e.g. "14 AST-based
// detectors", "14 architecture detectors", "✅ 14 detectors".
var detectorCountClaim = regexp.MustCompile(`(\d+)\s+(?:[A-Za-z-]+\s+)*detectors`)

// Every place that states how many detectors verikt has must state the real
// number.
//
// Six website pages and the README said 11, docs/reference/language-support.md
// said 12, and there were 14 — the count went stale in every location at once
// when two detectors were added, and nothing noticed. A number in prose is a
// claim like any other; this makes it one the build checks.
//
// Historical changelog entries are exempt: "0.1.0 shipped with 11 detectors" was
// true when it was written, and rewriting released release notes to match the
// present would be a different kind of inaccuracy.
func TestDocumentedDetectorCountMatchesImplementation(t *testing.T) {
	want := len(detectorSeverities)

	root := repoRoot(t)

	// Only files the repository actually ships are checked. Untracked working-copy
	// files — review reports, scratch notes — are not documentation, and scanning
	// them made this guard fail on prose *about* a stale count.
	tracked := trackedFiles(t, root)

	exempt := map[string]bool{
		"CHANGELOG.md":                           true,
		"website/src/content/docs/changelog.mdx": true,
	}

	var checked int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "target", "experiments", "dist", "testdata":
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".md" && ext != ".mdx" {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		slug := filepath.ToSlash(rel)
		if exempt[slug] || !tracked[slug] {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		for i, line := range strings.Split(string(data), "\n") {
			for _, match := range detectorCountClaim.FindAllStringSubmatch(line, -1) {
				got, convErr := strconv.Atoi(match[1])
				if convErr != nil {
					continue
				}
				checked++
				if got != want {
					t.Errorf("%s:%d claims %d detectors, there are %d: %q",
						filepath.ToSlash(rel), i+1, got, want, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs: %v", err)
	}

	if checked == 0 {
		t.Error("no documented detector count found; this test no longer guards anything")
	}
}

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

// trackedFiles returns the repository-relative paths git tracks.
func trackedFiles(t *testing.T, root string) map[string]bool {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	require.NoError(t, err, "git ls-files is needed to tell shipped docs from scratch files")

	files := map[string]bool{}
	for _, name := range strings.Split(string(out), "\x00") {
		if name != "" {
			files[name] = true
		}
	}
	require.NotEmpty(t, files, "no tracked files found")
	return files
}
