package engineclient

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnginePath_ExtractsAndReturnsExecutable(t *testing.T) {
	path, err := EnginePath()
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.NotZero(t, info.Mode()&0o111, "binary should be executable")
}

// The cache directory must be derived from the binary's content. It was keyed on
// a hand-maintained version constant, so rebuilding the engine without also
// bumping that constant left every existing user running the previously
// extracted binary — silently, and indistinguishably from a working upgrade.
func TestEngineCacheKeyIsContentAddressed(t *testing.T) {
	first := engineCacheKey([]byte("engine-build-one"))
	second := engineCacheKey([]byte("engine-build-two"))

	assert.NotEqual(t, first, second, "different engine builds must not share a cache directory")
	assert.Equal(t, first, engineCacheKey([]byte("engine-build-one")), "same content must be stable across runs")
	assert.NotEmpty(t, first)
	assert.NotContains(t, first, string(os.PathSeparator), "cache key is a single directory name")
}

// A cached extraction from an older engine build must not be returned for a
// different embedded binary.
func TestEnginePath_UsesDistinctDirPerBuild(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	if runtime.GOOS == "darwin" {
		t.Setenv("HOME", cacheDir)
	}

	path, err := EnginePath()
	require.NoError(t, err)

	assert.Contains(t, path, engineCacheKey(engineBinary),
		"extracted path must be namespaced by the embedded binary's content")
}

// Content-addressed caching creates a directory per engine build, so stale ones
// must be removed or every upgrade leaks another copy of the binary.
func TestEnginePath_PrunesStaleCacheDirs(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	if runtime.GOOS == "darwin" {
		t.Setenv("HOME", cacheDir)
	}

	path, err := EnginePath()
	require.NoError(t, err)
	veriktCache := filepath.Dir(filepath.Dir(path))

	// Simulate directories left by earlier engine builds, plus an unrelated one.
	stale := filepath.Join(veriktCache, "engine-0000000000000000")
	unrelated := filepath.Join(veriktCache, "templates")
	require.NoError(t, os.MkdirAll(stale, 0o755))
	require.NoError(t, os.MkdirAll(unrelated, 0o755))

	// Force a re-extraction so pruning runs.
	require.NoError(t, os.Remove(path))
	_, err = EnginePath()
	require.NoError(t, err)

	assert.NoDirExists(t, stale, "stale engine cache should be removed")
	assert.FileExists(t, path, "current engine must survive pruning")
	assert.DirExists(t, unrelated, "non-engine cache entries must be left alone")
}

func TestEnginePath_IdempotentOnSecondCall(t *testing.T) {
	path1, err := EnginePath()
	require.NoError(t, err)

	path2, err := EnginePath()
	require.NoError(t, err)

	assert.Equal(t, path1, path2)
}
