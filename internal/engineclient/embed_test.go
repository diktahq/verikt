package engineclient

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireEmbeddedEngine skips when no engine was embedded for this platform.
//
// That is a supported configuration since the embed became a directory lookup: a
// checkout that has not run `mise run build-engine` has no binary, and extraction
// has nothing to test.
func requireEmbeddedEngine(t *testing.T) {
	t.Helper()
	if len(engineBinary) == 0 {
		t.Skip("no engine embedded for this platform — run `mise run build-engine`")
	}
}

// A missing engine must be reported as an error, not crash or return a path to
// nothing: callers fall back to Go-native analysis on this signal.
func TestEnginePath_ReportsAbsentEngine(t *testing.T) {
	if len(engineBinary) > 0 {
		t.Skip("an engine is embedded; this covers the absent case")
	}
	if _, err := EnginePath(); err == nil {
		t.Fatal("expected an error when no engine is embedded")
	}
}

// An unknown platform has no embedded binary and must resolve to nil rather than
// failing to build, which is the whole point of the directory embed.
func TestEmbeddedEngineFor_UnknownPlatformIsNil(t *testing.T) {
	if got := embeddedEngineFor("plan9", "riscv64"); got != nil {
		t.Errorf("expected nil for an unbuilt platform, got %d bytes", len(got))
	}
}

// The embed pattern must always match at least one file, or the package does not
// compile. bin/README.md is committed for exactly that reason.
func TestEngineFS_ContainsCommittedReadme(t *testing.T) {
	if _, err := engineFS.ReadFile("bin/README.md"); err != nil {
		t.Fatalf("bin/README.md must stay committed to satisfy //go:embed bin: %v", err)
	}
}

func TestEnginePath_ExtractsAndReturnsExecutable(t *testing.T) {
	requireEmbeddedEngine(t)
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
	requireEmbeddedEngine(t)
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
	requireEmbeddedEngine(t)
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	if runtime.GOOS == "darwin" {
		t.Setenv("HOME", cacheDir)
	}

	path, err := EnginePath()
	require.NoError(t, err)
	veriktCache := filepath.Dir(filepath.Dir(path))

	// Simulate a directory left by an earlier engine build, aged past the threshold,
	// plus a recent one and an unrelated one.
	stale := filepath.Join(veriktCache, "engine-0000000000000000")
	recent := filepath.Join(veriktCache, "engine-1111111111111111")
	unrelated := filepath.Join(veriktCache, "templates")
	require.NoError(t, os.MkdirAll(stale, 0o755))
	require.NoError(t, os.MkdirAll(recent, 0o755))
	require.NoError(t, os.MkdirAll(unrelated, 0o755))

	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(stale, old, old))

	// Force a re-extraction so pruning runs.
	require.NoError(t, os.Remove(path))
	_, err = EnginePath()
	require.NoError(t, err)

	assert.NoDirExists(t, stale, "an aged engine cache should be removed")
	assert.FileExists(t, path, "current engine must survive pruning")
	assert.DirExists(t, unrelated, "non-engine cache entries must be left alone")

	// A concurrent process running an older verikt may have just resolved this path
	// and not yet executed it. Deleting it would break that process, so recent
	// directories are left alone.
	assert.DirExists(t, recent, "a recently used engine cache must not be pruned")
}

func TestEnginePath_IdempotentOnSecondCall(t *testing.T) {
	requireEmbeddedEngine(t)
	path1, err := EnginePath()
	require.NoError(t, err)

	path2, err := EnginePath()
	require.NoError(t, err)

	assert.Equal(t, path1, path2)
}

// The release build must embed exactly one engine.
//
// The development build embeds the bin/ directory so a fresh clone can compile
// before any engine exists. During a release the workflow stages all four
// platform engines for one parallel GoReleaser run, so that same pattern put
// every engine into every artefact — roughly 17 MB per artefact of payload the
// target platform can never execute.
//
// This asserts the two build modes still exist and disagree in the right
// direction; the selection itself is a compile-time constraint, so the guard is
// that the release files are present and constrained.
func TestReleaseBuildSelectsASingleEngine(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	platforms := map[string]bool{
		"embed_engine_release_linux_amd64.go":  false,
		"embed_engine_release_linux_arm64.go":  false,
		"embed_engine_release_darwin_amd64.go": false,
		"embed_engine_release_darwin_arm64.go": false,
	}

	for _, entry := range entries {
		name := entry.Name()
		if _, ok := platforms[name]; !ok {
			continue
		}
		data, readErr := os.ReadFile(name)
		if readErr != nil {
			t.Fatalf("read %s: %v", name, readErr)
		}
		source := string(data)
		if !strings.Contains(source, "//go:build verikt_release") {
			t.Errorf("%s is not gated on the verikt_release tag, so it would break the default build", name)
		}
		if strings.Count(source, "//go:embed") != 1 {
			t.Errorf("%s must embed exactly one engine", name)
		}
		platforms[name] = true
	}

	for name, found := range platforms {
		if !found {
			t.Errorf("%s is missing: that platform's release build would have no engine", name)
		}
	}

	// And the default build must stay tolerant of an absent engine, or a fresh
	// clone cannot compile.
	def, err := os.ReadFile("embed_engine.go")
	if err != nil {
		t.Fatalf("read embed_engine.go: %v", err)
	}
	if !strings.Contains(string(def), "//go:build !verikt_release") {
		t.Error("the default embed must be excluded from release builds, or both would define engineBinary")
	}
	if !strings.Contains(string(def), "//go:embed bin\n") {
		t.Error("the default embed must match the directory so a missing engine still compiles")
	}
}
