package experiment

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// repoRoot returns the absolute path to the repository root by walking up from
// this source file's location.
func repoRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	// Walk up: experiment/ → engineclient/ → internal/ → repo root
	return filepath.Join(filepath.Dir(filename), "..", "..", "..")
}

// experimentArtifactDir returns the path to write run artifacts.
// Pattern: experiments/<experimentID>/results/<agentID>_<condition>_run<N>_<date>/
func experimentArtifactDir(experimentID, aid, condition string, run int) string {
	date := time.Now().Format("2006-01-02")
	name := fmt.Sprintf("%s_%s_run%d_%s", aid, condition, run, date)
	return filepath.Join(repoRoot(), "experiments", experimentID, "results", name)
}

// runManifest is the JSON record of a single experiment run's inputs.
type runManifest struct {
	ExperimentID  string       `json:"experiment_id"`
	Condition     string       `json:"condition"`
	Run           int          `json:"run"`
	Date          string       `json:"date"`
	Agent         agentRunMeta `json:"agent"`
	Fixture       *fixtureMeta `json:"fixture,omitempty"`
	SystemPrompt  string       `json:"system_prompt"`
	Guide         *string      `json:"guide,omitempty"`
	TaskPrompt    string       `json:"task_prompt"`
	FullPromptSHA string       `json:"full_prompt_sha256"`
}

type agentRunMeta struct {
	ID         string `json:"id"`
	ToolAccess bool   `json:"tool_access"`
}

type fixtureMeta struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// runMetrics is the JSON record of a single experiment run's outputs.
type runMetrics struct {
	ViolationsDep   int      `json:"violations_dep"`
	ViolationsFn    int      `json:"violations_fn"`
	ViolationsAP    int      `json:"violations_ap"`
	ViolationsArch  int      `json:"violations_arch"`
	ViolationsTotal int      `json:"violations_total"`
	Passed          bool     `json:"passed"`
	HexagonalShape  bool     `json:"hexagonal_shape"`
	Packages        []string `json:"packages"`
	FilesGenerated  int      `json:"files_generated"`
	InputTokens     int      `json:"input_tokens"`
	CacheTokens     int      `json:"cache_tokens"`
	OutputTokens    int      `json:"output_tokens"`
	DurationMS      int      `json:"duration_ms"`
}

// runArtifacts holds everything produced by one run.
type runArtifacts struct {
	Manifest       runManifest
	Response       string
	GeneratedFiles map[string]string
	CheckResult    json.RawMessage
	CheckDiff      json.RawMessage
	Metrics        runMetrics
}

// persistArtifacts writes all run artifacts to disk.
func persistArtifacts(dir string, a runArtifacts) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := writeJSON(filepath.Join(dir, "manifest.json"), a.Manifest); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "response.txt"), []byte(a.Response), 0644); err != nil {
		return fmt.Errorf("write response.txt: %w", err)
	}
	if err := writeJSON(filepath.Join(dir, "metrics.json"), a.Metrics); err != nil {
		return err
	}
	if a.CheckResult != nil {
		if err := os.WriteFile(filepath.Join(dir, "verikt-check.json"), a.CheckResult, 0644); err != nil {
			return fmt.Errorf("write verikt-check.json: %w", err)
		}
	}
	if a.CheckDiff != nil {
		if err := os.WriteFile(filepath.Join(dir, "verikt-check-diff.json"), a.CheckDiff, 0644); err != nil {
			return fmt.Errorf("write verikt-check-diff.json: %w", err)
		}
	}
	filesDir := filepath.Join(dir, "files")
	for path, content := range a.GeneratedFiles {
		full := filepath.Join(filesDir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", path, err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func promptSHA(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		fmt.Fprintln(h, p)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// updateExperimentIndex updates the experiment status in experiments/index.json.
// If the experiment is found and marked needs-rerun, it flips to complete. Safe to
// call concurrently only when experiments are not running in parallel.
func updateExperimentIndex(experimentID, agentID string) {
	indexPath := filepath.Join(repoRoot(), "experiments", "index.json")

	var idx struct {
		Generated   string           `json:"generated"`
		Experiments []map[string]any `json:"experiments"`
	}

	data, err := os.ReadFile(indexPath)
	if err == nil {
		_ = json.Unmarshal(data, &idx) //nolint:errcheck
	}

	idx.Generated = time.Now().UTC().Format(time.RFC3339)

	found := false
	for _, e := range idx.Experiments {
		if e["id"] == experimentID {
			agents, _ := e["agents"].(map[string]any)
			if agents == nil {
				agents = map[string]any{}
			}
			agents[agentID] = map[string]any{
				"latest": time.Now().Format("2006-01-02"),
			}
			e["agents"] = agents
			if e["status"] == "needs-rerun" {
				e["status"] = "complete"
				delete(e, "note")
			}
			found = true
			break
		}
	}
	if !found {
		idx.Experiments = append(idx.Experiments, map[string]any{
			"id":     experimentID,
			"status": "complete",
			"agents": map[string]any{
				agentID: map[string]any{"latest": time.Now().Format("2006-01-02")},
			},
		})
	}

	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(indexPath, b, 0644) //nolint:errcheck
}
