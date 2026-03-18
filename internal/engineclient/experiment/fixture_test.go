package experiment

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// embedFixture reads all files in dir and formats them as a prompt block.
// Returns the block text and a truncated sha256 of the fixture contents.
//
// Output format:
//
//	=== domain/order.go ===
//	[content]
//	=== port/order_service.go ===
//	[content]
//
// Files named .cursorrules and .gitignore are excluded — they are generated
// or tool-specific and not part of the service source.
func embedFixture(dir string) (block string, sha string, err error) {
	type entry struct {
		rel     string
		content string
	}
	var entries []entry

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base == ".cursorrules" || base == ".gitignore" {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		entries = append(entries, entry{rel: rel, content: string(data)})
		return nil
	})
	if err != nil {
		return "", "", fmt.Errorf("walk fixture %s: %w", dir, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].rel < entries[j].rel
	})

	h := sha256.New()
	var sb strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&sb, "=== %s ===\n%s\n", e.rel, e.content)
		fmt.Fprintf(h, "%s\n%s\n", e.rel, e.content)
	}

	return sb.String(), fmt.Sprintf("%x", h.Sum(nil))[:16], nil
}

// greenfieldSystemPrompt is the system prompt for Mode A (greenfield) experiments.
// Identical in effect to fileOutputSystemPrompt but named for the methodology.
const greenfieldSystemPrompt = `You are a Go engineer. When implementing code, return ONLY the file contents using this exact format with no other text:
=== <filepath> ===
[complete file content]

Include all files needed for a working implementation. Module name will be specified in the task.`

// embeddedFixtureSystemPrompt is the system prompt for Mode B (embedded fixture) experiments.
// The fixture block is appended by RunFeatureAddition before calling the agent.
const embeddedFixtureSystemPrompt = `You are a Go engineer working on an existing codebase shown below.

Return ONLY the files you modified or created, using this exact format:
=== <filepath> ===
[complete file content]

Do not return unchanged files. Do not add explanations outside the file blocks.`

// openFixtureSystemPrompt is used when the fixture is placed in the user prompt instead of
// the system prompt (FixtureInUserPrompt: true). It does not restrict content after the file
// blocks, allowing the agent to append sections like === RECOMMENDATIONS ===.
const openFixtureSystemPrompt = `You are a Go engineer working on an existing codebase.

Return modified or created files using this exact format:
=== <filepath> ===
[complete file content]

Do not return unchanged files.`
