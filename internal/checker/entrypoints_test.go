package checker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/config"
)

func writeProjectFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
}

const mainSrc = "package main\n\nfunc main() {}\n"

func entrypointCfg(entries ...string) *config.VeriktConfig {
	cfg := &config.VeriktConfig{Language: "go", Architecture: "flat"}
	list := append([]string{}, entries...)
	cfg.Check.AllowedEntrypoints = &list
	return cfg
}

// entrypointFiles returns the reported files, asserting every finding has the
// shape the rest of the check pipeline relies on.
func entrypointFiles(t *testing.T, vs []Violation) []string {
	t.Helper()
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		if v.Rule != RuleCmdAllowlist {
			continue
		}
		assert.Equal(t, "warning", v.Severity, "a new detector ships at warning (INV-005)")
		assert.Equal(t, "structure", v.Category)
		out = append(out, v.File)
	}
	return out
}

func TestUnapprovedEntrypoints_ApprovedEntrypointsAreNotReported(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/server/main.go", mainSrc)
	writeProjectFile(t, dir, "cmd/cli/main.go", mainSrc)

	got := entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg("cmd/server", "./cmd/cli/"), dir))

	assert.Empty(t, got)
}

func TestUnapprovedEntrypoints_ReportsUnapprovedHelperOncePerDirectory(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/server/main.go", mainSrc)
	writeProjectFile(t, dir, "cmd/helper/run.go", mainSrc)
	writeProjectFile(t, dir, "cmd/helper/zz_main.go", "package main\n")

	vs := detectUnapprovedEntrypoints(entrypointCfg("cmd/server"), dir)

	assert.Equal(t, []string{"cmd/helper/run.go"}, entrypointFiles(t, vs))
	assert.Contains(t, vs[0].Message, "cmd/helper")
	assert.Contains(t, vs[0].Message, "check.allowed_entrypoints")
}

func TestUnapprovedEntrypoints_ReportsMainGoWhenTheDirectoryHasOne(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/helper/app.go", mainSrc)
	writeProjectFile(t, dir, "cmd/helper/main.go", "package main\n")

	got := entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg(), dir))

	assert.Equal(t, []string{"cmd/helper/main.go"}, got)
}

func TestUnapprovedEntrypoints_IgnoresNestedNonMainPackages(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/server/main.go", mainSrc)
	writeProjectFile(t, dir, "cmd/server/internal/wire/wire.go", "package wire\n")
	writeProjectFile(t, dir, "cmd/shared/flags.go", "package shared\n")

	got := entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg("cmd/server"), dir))

	assert.Empty(t, got)
}

func TestUnapprovedEntrypoints_IgnoresTestOnlyMainPackages(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/server/main.go", mainSrc)
	writeProjectFile(t, dir, "cmd/server/internal/wire/main_test.go", "package main\n")

	got := entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg("cmd/server"), dir))

	assert.Empty(t, got, "detectors never see test files (INV-004)")
}

func TestUnapprovedEntrypoints_IgnoresBuildIgnoredGenerators(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/server/main.go", mainSrc)
	writeProjectFile(t, dir, "cmd/server/internal/gen/gen.go", "//go:build ignore\n\npackage main\n")

	got := entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg("cmd/server"), dir))

	assert.Empty(t, got)
}

// A file is a generator only when its build constraint cannot be true without
// the ignore tag. Mentioning ignore is not enough: `!ignore` and `x || ignore`
// build by default, so the binary exists and must be reported.
func TestUnapprovedEntrypoints_BuildConstraintNeedsIgnoreToHide(t *testing.T) {
	cases := []struct {
		constraint string
		reported   bool
	}{
		{"//go:build ignore", false},
		{"//go:build ignore && linux", false},
		{"//go:build linux && ignore", false},
		{"//go:build !windows && ignore", false},
		{"//go:build (ignore || ignore) && amd64", false},
		{"//go:build ignore && !ignore", false},
		{"//go:build !ignore", true},
		{"//go:build linux || ignore", true},
		{"//go:build go1.1 || ignore", true},
		{"//go:build !(ignore)", true},
		{"//go:build !(ignore && linux)", true},
		{"//go:build !windows", true},
		{"//go:build linux && !ignore", true},
		{"//go:build (ignore || linux) && !windows", true},
	}
	for _, c := range cases {
		t.Run(c.constraint, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectFile(t, dir, "cmd/helper/main.go", c.constraint+"\n\npackage main\n\nfunc main() {}\n")

			got := entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg(), dir))

			if c.reported {
				assert.Equal(t, []string{"cmd/helper/main.go"}, got)
			} else {
				assert.Empty(t, got)
			}
		})
	}
}

func TestUnapprovedEntrypoints_ReportsNestedMainUnderCmd(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/server/main.go", mainSrc)
	writeProjectFile(t, dir, "cmd/server/tools/migrate/main.go", mainSrc)

	got := entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg("cmd/server"), dir))

	assert.Equal(t, []string{"cmd/server/tools/migrate/main.go"}, got)
}

func TestUnapprovedEntrypoints_OSConstrainedMainIsStillAnEntrypoint(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/winsvc/main.go", "//go:build windows\n\npackage main\n")

	got := entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg(), dir))

	assert.Equal(t, []string{"cmd/winsvc/main.go"}, got)
}

func TestUnapprovedEntrypoints_OffWhenAllowlistKeyIsAbsent(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/helper/main.go", mainSrc)
	writeProjectFile(t, dir, "main.go", mainSrc)

	cfg := &config.VeriktConfig{Language: "go", Architecture: "flat"}

	assert.Empty(t, detectUnapprovedEntrypoints(cfg, dir))
}

func TestUnapprovedEntrypoints_EmptyAllowlistReportsEveryEntrypoint(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/server/main.go", mainSrc)
	writeProjectFile(t, dir, "main.go", mainSrc)

	got := entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg(), dir))

	assert.Equal(t, []string{"main.go", "cmd/server/main.go"}, got)
}

func TestUnapprovedEntrypoints_RootMainIsApprovedAsDot(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "main.go", mainSrc)

	assert.Equal(t, []string{"main.go"},
		entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg("cmd/server"), dir)))
	assert.Empty(t, entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg("."), dir)))
}

func TestUnapprovedEntrypoints_RootLibraryIsNotAnEntrypoint(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "lib.go", "package mylib\n")

	assert.Empty(t, detectUnapprovedEntrypoints(entrypointCfg(), dir))
}

func TestUnapprovedEntrypoints_OnlyCmdAndTheRootAreEntrypointLocations(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "tools/gen/main.go", mainSrc)
	writeProjectFile(t, dir, "internal/x/main.go", mainSrc)

	assert.Empty(t, detectUnapprovedEntrypoints(entrypointCfg(), dir))
}

func TestUnapprovedEntrypoints_SkipsSymlinkedDirectoryUnderCmd(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeProjectFile(t, outside, "helper/main.go", mainSrc)
	writeProjectFile(t, dir, "cmd/server/main.go", mainSrc)
	if err := os.Symlink(filepath.Join(outside, "helper"), filepath.Join(dir, "cmd", "helper")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	assert.Empty(t, detectUnapprovedEntrypoints(entrypointCfg("cmd/server"), dir), "INV-002")
}

func TestUnapprovedEntrypoints_SkipsSymlinkedCmd(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeProjectFile(t, outside, "helper/main.go", mainSrc)
	if err := os.Symlink(outside, filepath.Join(dir, "cmd")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	assert.Empty(t, detectUnapprovedEntrypoints(entrypointCfg(), dir), "INV-002")
}

func TestUnapprovedEntrypoints_SkipsSymlinkedMainFile(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeProjectFile(t, outside, "main.go", mainSrc)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cmd", "helper"), 0o755))
	if err := os.Symlink(filepath.Join(outside, "main.go"), filepath.Join(dir, "cmd", "helper", "main.go")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	assert.Empty(t, detectUnapprovedEntrypoints(entrypointCfg(), dir), "INV-002")
}

func TestUnapprovedEntrypoints_SkipsDirectoriesTheGoToolIgnores(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/_scratch/main.go", mainSrc)
	writeProjectFile(t, dir, "cmd/.tmp/main.go", mainSrc)
	writeProjectFile(t, dir, "cmd/server/testdata/fixture/main.go", mainSrc)
	writeProjectFile(t, dir, "cmd/vendor/x/main.go", mainSrc)

	got := entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg(), dir))

	assert.Empty(t, got)
}

func TestUnapprovedEntrypoints_OffForTypeScript(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/helper/main.go", mainSrc)
	cfg := entrypointCfg()
	cfg.Language = "typescript"

	assert.Empty(t, detectUnapprovedEntrypoints(cfg, dir))
}

// A file whose package clause does not parse does not compile, so it is not an
// entrypoint. Aborting the whole check over it would turn one broken file into
// no report at all; reporting it would be a finding without evidence (INV-005).
func TestUnapprovedEntrypoints_UnparsableFileIsNotAnEntrypoint(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "cmd/broken/main.go", "this is not go\n")
	writeProjectFile(t, dir, "cmd/helper/main.go", mainSrc)

	got := entrypointFiles(t, detectUnapprovedEntrypoints(entrypointCfg(), dir))

	assert.Equal(t, []string{"cmd/helper/main.go"}, got)
}

// The tests above call the detector directly. These go through CheckWithEngine,
// so they fail if the wiring is removed.
func TestCheckWithEngine_ReportsUnapprovedEntrypointsAsStructureWarnings(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "go.mod", "module example.com/demo\n\ngo 1.22\n")
	writeProjectFile(t, dir, "cmd/helper/main.go", mainSrc)

	result, err := CheckWithEngine(entrypointCfg(), dir,
		emptyAntiPatternClient, dependencyClientReturning(nil), emptyMetricClient)

	require.NoError(t, err)
	assert.Equal(t, []string{"cmd/helper/main.go"}, entrypointFiles(t, result.StructureViolations))
}

// Projects exclude cmd/** from orphan detection; verikt's own defaults once did.
// Applying check.exclude here would switch the detector off in exactly those
// projects, so it is deliberately not applied.
func TestCheckWithEngine_EntrypointsIgnoreCheckExclude(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "go.mod", "module example.com/demo\n\ngo 1.22\n")
	writeProjectFile(t, dir, "cmd/helper/main.go", mainSrc)
	cfg := entrypointCfg()
	cfg.Check.Exclude = []string{"cmd/**"}

	result, err := CheckWithEngine(cfg, dir,
		emptyAntiPatternClient, dependencyClientReturning(nil), emptyMetricClient)

	require.NoError(t, err)
	assert.Equal(t, []string{"cmd/helper/main.go"}, entrypointFiles(t, result.StructureViolations))
}
