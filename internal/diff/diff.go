package diff

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/dcsg/archway/internal/scaffold"
)

// Result holds the full diff between archway.yaml declarations and on-disk files.
type Result struct {
	Architecture    string           `json:"architecture"`
	CapabilityDiffs []CapabilityDiff `json:"capability_diffs"`
	Summary         Summary          `json:"summary"`
}

// CapabilityDiff describes the drift for a single capability (or architecture).
type CapabilityDiff struct {
	Name         string   `json:"name"`
	Status       string   `json:"status"`        // "ok", "partial", "missing"
	MissingFiles []string `json:"missing_files"` // files expected but not on disk
	PresentFiles []string `json:"present_files"` // files that exist
}

// Summary aggregates drift across all capabilities.
type Summary struct {
	TotalCapabilities int     `json:"total_capabilities"`
	FullyPresent      int     `json:"fully_present"`
	PartiallyPresent  int     `json:"partially_present"`
	FullyMissing      int     `json:"fully_missing"`
	DriftScore        float64 `json:"drift_score"` // 0.0 = perfect, 1.0 = total drift
}

// Run computes the structural diff between the declared composition and files on disk.
func Run(templateFS fs.FS, plan *scaffold.CompositionPlan, projectPath string) (*Result, error) {
	vars := plan.Vars
	vars["Partials"] = plan.Partials
	vars["SelectedCapabilities"] = plan.Capabilities

	result := &Result{
		Architecture: plan.Architecture,
	}

	// Diff architecture files.
	archDiff, err := diffFilesDir(templateFS, path.Join(plan.ArchDir, "files"), projectPath, vars, plan.Architecture+" (arch)")
	if err != nil {
		return nil, fmt.Errorf("diff architecture: %w", err)
	}
	result.CapabilityDiffs = append(result.CapabilityDiffs, *archDiff)

	// Diff each capability's files.
	for i, capDir := range plan.CapDirs {
		capName := plan.Capabilities[i]
		capDiff, err := diffFilesDir(templateFS, path.Join(capDir, "files"), projectPath, vars, capName)
		if err != nil {
			return nil, fmt.Errorf("diff capability %s: %w", capName, err)
		}
		result.CapabilityDiffs = append(result.CapabilityDiffs, *capDiff)
	}

	// Compute summary.
	result.Summary = computeSummary(result.CapabilityDiffs)

	return result, nil
}

// diffFilesDir walks a template files/ directory and checks which rendered files exist on disk.
func diffFilesDir(templateFS fs.FS, filesRoot, projectPath string, vars map[string]interface{}, name string) (*CapabilityDiff, error) {
	diff := &CapabilityDiff{Name: name}

	// Check if the directory exists in the template FS.
	if _, err := fs.Stat(templateFS, filesRoot); err != nil {
		// No files directory means nothing expected — always ok.
		diff.Status = "ok"
		return diff, nil
	}

	var expectedFiles []string
	if err := fs.WalkDir(templateFS, filesRoot, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == filesRoot {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		rel := strings.TrimPrefix(current, filesRoot+"/")
		renderedRel, err := scaffold.RenderPath(rel, vars)
		if err != nil {
			return fmt.Errorf("render path %q: %w", rel, err)
		}

		// Strip .tmpl suffix — templates render to files without it.
		renderedRel = strings.TrimSuffix(renderedRel, ".tmpl")

		// Skip files whose template content renders to empty (conditional templates).
		if strings.HasSuffix(current, ".tmpl") {
			content, readErr := fs.ReadFile(templateFS, current)
			if readErr != nil {
				return fmt.Errorf("read template %q: %w", current, readErr)
			}
			if isConditionallyEmpty(string(content), vars) {
				return nil
			}
		}

		expectedFiles = append(expectedFiles, renderedRel)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk template files: %w", err)
	}

	// Check which expected files exist on disk.
	for _, relPath := range expectedFiles {
		absPath := filepath.Join(projectPath, filepath.FromSlash(relPath))
		if _, err := os.Stat(absPath); err == nil {
			diff.PresentFiles = append(diff.PresentFiles, relPath)
		} else {
			diff.MissingFiles = append(diff.MissingFiles, relPath)
		}
	}

	// Determine status.
	switch {
	case len(expectedFiles) == 0:
		diff.Status = "ok"
	case len(diff.MissingFiles) == 0:
		diff.Status = "ok"
	case len(diff.PresentFiles) == 0:
		diff.Status = "missing"
	default:
		diff.Status = "partial"
	}

	return diff, nil
}

// isConditionallyEmpty checks if a template would render to empty content given the vars.
// It uses a simple heuristic: if the entire template is wrapped in an {{if}} block,
// evaluate whether the condition is likely false.
func isConditionallyEmpty(content string, vars map[string]interface{}) bool {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "{{") {
		return false
	}

	// Actually render the template to check.
	rendered, err := renderTemplate(content, vars)
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(rendered)) == 0
}

func computeSummary(diffs []CapabilityDiff) Summary {
	s := Summary{TotalCapabilities: len(diffs)}
	totalExpected := 0
	totalMissing := 0

	for _, d := range diffs {
		switch d.Status {
		case "ok":
			s.FullyPresent++
		case "partial":
			s.PartiallyPresent++
		case "missing":
			s.FullyMissing++
		}
		totalExpected += len(d.PresentFiles) + len(d.MissingFiles)
		totalMissing += len(d.MissingFiles)
	}

	if totalExpected > 0 {
		s.DriftScore = float64(totalMissing) / float64(totalExpected)
	}

	return s
}
