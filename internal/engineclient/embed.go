package engineclient

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// version is bumped whenever the engine binary changes.
// This ensures cached binaries are replaced on upgrade.
const version = "0.1.0"

// EnginePath extracts the embedded engine binary to the user cache directory
// and returns its path. Extraction is skipped if the binary already exists
// at the expected path.
//
// Returns an error on platforms where no engine binary was embedded.
func EnginePath() (string, error) {
	if len(engineBinary) == 0 {
		return "", fmt.Errorf("engine binary not available on %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("user cache dir: %w", err)
	}

	dir := filepath.Join(cacheDir, "verikt", "engine-v"+version)
	name := "verikt-engine"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)

	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	if err := os.WriteFile(path, engineBinary, 0o755); err != nil {
		return "", fmt.Errorf("write engine binary: %w", err)
	}

	return path, nil
}
