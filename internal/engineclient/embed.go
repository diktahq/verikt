package engineclient

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
//
// The name uses the first 64 bits of the digest, which is plenty to keep engine
// builds apart. It is not what decides whether a cached file is trusted: every
// cache hit is checked against the full digest (see cachedEngineValid).
func engineCacheKey(binary []byte) string {
	sum := sha256.Sum256(binary)
	return hex.EncodeToString(sum[:8])
}

// embeddedEngineDigest is the SHA-256 of engineBinary, computed once per
// process rather than on every EnginePath call.
var embeddedEngineDigest = sync.OnceValue(func() [sha256.Size]byte {
	return sha256.Sum256(engineBinary)
})

// EnginePath extracts the embedded engine binary to the user cache directory
// and returns its path.
//
// A binary already in the cache is used only after it is verified to be the
// embedded one; anything else at that path is replaced. Extraction is atomic, so
// concurrent callers, in one process or several, never see a partial binary.
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

	return extractEngineWithDigest(engineBinary, embeddedEngineDigest(), filepath.Join(cacheDir, "verikt"))
}

// extractEngine installs binary under cacheRoot and returns the path of a
// verified copy. It is EnginePath with the binary and the cache root injected,
// so the cache behaviour can be tested without a real engine or the user's
// cache directory.
func extractEngine(binary []byte, cacheRoot string) (string, error) {
	return extractEngineWithDigest(binary, sha256.Sum256(binary), cacheRoot)
}

func extractEngineWithDigest(binary []byte, digest [sha256.Size]byte, cacheRoot string) (string, error) {
	if len(binary) == 0 {
		return "", errors.New("engine binary is empty")
	}

	dir := filepath.Join(cacheRoot, "engine-"+hex.EncodeToString(digest[:8]))
	name := "verikt-engine"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	size := int64(len(binary))

	// The cache used to be trusted on existence alone: whatever file sat at this
	// path was executed. A truncated extraction, a corrupted block, or a file
	// planted in the user's cache directory ran as the engine without a word.
	if cachedEngineValid(path, digest, size) {
		return path, nil
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create engine cache dir: %w", err)
	}
	// MkdirAll accepts an existing symlink to a directory. Writing an executable
	// through one would install it wherever the link points.
	info, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("stat engine cache dir: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("engine cache dir %s is not a directory", dir)
	}

	if err := extractEngineAtomically(binary, path, dir); err != nil {
		return "", err
	}

	// Checked again after the rename: a concurrent process may have installed
	// its copy over ours, and that copy is held to the same standard.
	if !cachedEngineValid(path, digest, size) {
		return "", fmt.Errorf("engine binary at %s failed verification after extraction", path)
	}

	pruneStaleEngineCaches(cacheRoot, filepath.Base(dir))

	return path, nil
}

// cachedEngineValid reports whether path is the embedded engine: a regular file
// (not a symlink, whose target could change after this check), executable by its
// owner, of the expected size, and with the expected SHA-256.
//
// The size check is a cheap filter in front of the hash; the hash is what
// decides.
func cachedEngineValid(path string, digest [sha256.Size]byte, size int64) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return false
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		return false
	}

	f, err := os.Open(path)
	if err != nil {
		return false
	}
	// A read-only handle: a close error cannot change what was read, and a read
	// failure is caught by io.Copy.
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(h.Sum(nil), digest[:]) == 1
}

// extractEngineAtomically writes the engine to a temporary file in the same
// directory and renames it into place.
//
// Writing straight to the final path leaves a window in which the file exists but
// is incomplete, and a second process starting during that window execs a
// truncated binary. On Linux, exec of a file another process still holds open
// for writing fails with ETXTBSY.
//
// The content-addressed cache key makes this more likely rather than less. Every
// engine change forces a fresh extraction, and `go test ./...` runs the packages
// that need the engine concurrently, so the first run after any engine edit is
// exactly the race.
//
// rename within a directory is atomic, and replaces whatever was at the
// destination — including a file that failed verification. Concurrent
// extractions all install identical content, so whichever rename lands last
// wins harmlessly; the caller verifies the result either way.
func extractEngineAtomically(binary []byte, path, dir string) error {
	tmp, err := os.CreateTemp(dir, ".verikt-engine-*")
	if err != nil {
		return fmt.Errorf("create temp engine file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		// No-op once the rename has succeeded.
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(binary); err != nil {
		// The write already failed; the close error adds nothing and the temp
		// file is removed by the deferred cleanup either way.
		_ = tmp.Close()
		return fmt.Errorf("write engine binary: %w", err)
	}
	// Owner-only: nobody else needs to read or run the engine, and a group- or
	// world-accessible executable in a cache directory is one more thing that
	// can be swapped.
	if err := tmp.Chmod(0o700); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod engine binary: %w", err)
	}
	// Close before rename: Windows cannot rename an open file, and the exec that
	// follows must not see a handle still open for writing.
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close engine binary: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
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
