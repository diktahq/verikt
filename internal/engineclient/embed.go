package engineclient

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// engineCacheKey derives the cache directory name from the engine binary's
// content, so a rebuilt engine can never be shadowed by a stale extraction.
//
// This was previously a hand-maintained version constant. Changing the engine
// without also bumping it left every existing user running the binary extracted
// by an earlier release: the new engine was embedded but never written to disk,
// because the versioned directory already existed. The failure was silent and
// looked exactly like a successful upgrade.
func engineCacheKey(binary []byte) string {
	sum := sha256.Sum256(binary)
	return hex.EncodeToString(sum[:8])
}

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

	dir := filepath.Join(cacheDir, "verikt", "engine-"+engineCacheKey(engineBinary))
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

	pruneStaleEngineCaches(filepath.Dir(dir), filepath.Base(dir))

	return path, nil
}

// pruneStaleEngineCaches removes engine cache directories other than current.
//
// Because the cache is content-addressed, every engine build extracts to a new
// directory — without pruning, each upgrade would leave another copy of a
// multi-megabyte binary behind forever.
//
// Failures are ignored: this is opportunistic cleanup, and a concurrent verikt
// process may still hold an older binary open. On Unix, removing a running
// executable's file is safe (the inode outlives the directory entry); on Windows
// the removal simply fails and the directory is retried on the next run.
func pruneStaleEngineCaches(root, current string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == current || !strings.HasPrefix(entry.Name(), "engine-") {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, entry.Name()))
	}
}
