package engineclient

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
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

	if err := extractEngineAtomically(path, dir); err != nil {
		return "", err
	}

	pruneStaleEngineCaches(filepath.Dir(dir), filepath.Base(dir))

	return path, nil
}

// extractEngineAtomically writes the engine to a temporary file in the same
// directory and renames it into place.
//
// Writing straight to the final path leaves a window in which the file exists but
// is incomplete, and the Stat above treats existence as "ready": a second process
// starting during that window execs a truncated binary. On Linux, exec of a file
// another process still holds open for writing fails with ETXTBSY.
//
// The content-addressed cache key makes this more likely rather than less. Every
// engine change forces a fresh extraction, and `go test ./...` runs the packages
// that need the engine concurrently, so the first run after any engine edit is
// exactly the race.
//
// rename within a directory is atomic on POSIX and on Windows for a destination
// that does not yet exist. If another process won the race, its file is already
// complete and correct — the content hash is in the directory name — so losing
// the rename is not an error.
func extractEngineAtomically(path, dir string) error {
	tmp, err := os.CreateTemp(dir, ".verikt-engine-*")
	if err != nil {
		return fmt.Errorf("create temp engine file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		// No-op once the rename has succeeded.
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(engineBinary); err != nil {
		// The write already failed; the close error adds nothing and the temp
		// file is removed by the deferred cleanup either way.
		_ = tmp.Close()
		return fmt.Errorf("write engine binary: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod engine binary: %w", err)
	}
	// Close before rename: Windows cannot rename an open file, and the exec that
	// follows must not see a handle still open for writing.
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close engine binary: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		// A concurrent process may have completed the same extraction first. Its
		// content is identical — the directory name is the content hash — so an
		// existing, readable binary is success.
		if _, statErr := os.Stat(path); statErr == nil {
			return nil
		}
		return fmt.Errorf("install engine binary: %w", err)
	}

	return nil
}

// staleEngineCacheAge is how long an unused engine cache directory must have been
// idle before it is removed. It exists to avoid deleting a directory a concurrent
// process has resolved but not yet executed.
const staleEngineCacheAge = 24 * time.Hour

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

		// Only prune directories that have been untouched for a while. A concurrent
		// process running an older verikt can resolve its engine path and be deleted
		// before it execs — removing a file after exec is safe on Unix, but this
		// window is before it. An age threshold makes that race practically
		// unreachable while still bounding disk growth.
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < staleEngineCacheAge {
			continue
		}

		_ = os.RemoveAll(filepath.Join(root, entry.Name()))
	}
}
