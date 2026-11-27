package cli

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/engineclient"
)

// addPathCase holds where each Go architecture puts the hexagonal directories
// that capability templates are written for. The values mirror path_mappings
// in providers/golang/templates/architectures/<arch>/manifest.yaml; "" is the
// project root.
type addPathCase struct {
	arch    string
	handler string // adapter/httphandler
	redis   string // adapter/redisrepo
	service string // service
	domain  string // domain
}

var addPathCases = []addPathCase{
	{arch: "hexagonal", handler: "adapter/httphandler", redis: "adapter/redisrepo", service: "service", domain: "domain"},
	{arch: "clean", handler: "internal/interface/handler", redis: "internal/infrastructure/redis", service: "internal/usecase", domain: "internal/entity"},
	{arch: "layered", handler: "internal/handler", redis: "internal/repository", service: "internal/service", domain: "internal/model"},
	{arch: "flat", handler: "handler", redis: "repository", service: "", domain: "model"},
}

// hexagonalDirs are the capability templates' own top-level directories.
// Outside hexagonal, every one of them is mapped elsewhere.
var hexagonalDirs = []string{"adapter", "service", "domain", "port"}

// scaffoldMinimal runs `verikt new svc --arch <arch> --cap http-api` in a temp
// dir and changes into the project. It returns the project directory.
func scaffoldMinimal(t *testing.T, arch string) string {
	t.Helper()
	tmp := t.TempDir()
	chdir(t, tmp)
	out := captureStdout(t, func() {
		if _, err := executeCommand(t,
			"new", "svc",
			"--arch", arch,
			"--cap", "http-api",
			"--module", "example.com/svc",
			"--no-wizard",
			"--set", "skip_hooks=true",
		); err != nil {
			t.Errorf("scaffold %s: %v", arch, err)
		}
	})
	if t.Failed() {
		t.Fatalf("scaffold output:\n%s", out)
	}
	svcDir := filepath.Join(tmp, "svc")
	chdir(t, svcDir)
	return svcDir
}

// runAddQuiet runs `verikt add` with stdout captured and returns the output.
func runAddQuiet(t *testing.T, args ...string) string {
	t.Helper()
	var addErr error
	out := captureStdout(t, func() {
		_, addErr = executeCommand(t, append([]string{"add"}, args...)...)
	})
	if addErr != nil {
		t.Fatalf("verikt add %s: %v\noutput:\n%s", strings.Join(args, " "), addErr, out)
	}
	return out
}

// shapeFindings runs `verikt check` in the current directory and returns the
// architecture, dependency and structure findings.
func shapeFindings(t *testing.T) []string {
	t.Helper()
	var checkErr error
	out := captureStdout(t, func() {
		_, checkErr = executeCommand(t, "check", "--path", ".", "-o", "json")
	})
	var report struct {
		Violations []checker.Violation `json:"violations"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("check did not print a JSON report (err=%v): %v\noutput:\n%s", checkErr, err, out)
	}
	var shape []string
	for _, v := range report.Violations {
		if shapeCategories[v.Category] {
			shape = append(shape, "["+v.Category+"/"+v.Rule+"] "+v.File+" "+v.Message)
		}
	}
	return shape
}

// emptyDirs returns the project-relative empty directories under dir.
func emptyDirs(t *testing.T, dir string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			rel, relErr := filepath.Rel(dir, p)
			if relErr != nil {
				return relErr
			}
			out[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertFile(t *testing.T, dir, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
		t.Errorf("expected %s to exist: %v", rel, err)
	}
}

// assertNoHexagonalDirs fails when add wrote a capability template's own
// (unmapped) top-level directory into a non-hexagonal project.
func assertNoHexagonalDirs(t *testing.T, dir, arch string) {
	t.Helper()
	if arch == "hexagonal" {
		return
	}
	for _, rel := range hexagonalDirs {
		if _, err := os.Stat(filepath.Join(dir, rel)); err == nil {
			t.Errorf("verikt add created the unmapped hexagonal directory %s/ in a %s project", rel, arch)
		}
	}
}

func assertNoNewEmptyDirs(t *testing.T, dir string, before map[string]bool) {
	t.Helper()
	for rel := range emptyDirs(t, dir) {
		if !before[rel] {
			t.Errorf("verikt add left an empty directory behind: %s", rel)
		}
	}
}

func joinRel(dir, file string) string {
	if dir == "" {
		return file
	}
	return dir + "/" + file
}

func skipWithoutEngine(t *testing.T) {
	t.Helper()
	if _, err := engineclient.EnginePath(); err != nil {
		t.Skipf("verikt-engine is not available for this platform, so check cannot run: %v", err)
	}
}

// TestAddAppliesArchitecturePathMappings: `verikt add` places capability files
// where the project's architecture puts them, exactly as `verikt new` does, so
// the project still passes its own `verikt check` (issue #23). cors and redis
// write adapter/, testing writes service/ and internal/testutil/, ddd writes
// domain/, observability and retry write platform/.
func TestAddAppliesArchitecturePathMappings(t *testing.T) {
	skipWithoutEngine(t)

	for _, tc := range addPathCases {
		t.Run(tc.arch, func(t *testing.T) {
			dir := scaffoldMinimal(t, tc.arch)
			before := emptyDirs(t, dir)
			runAddQuiet(t, "cors", "redis", "observability", "testing", "ddd", "retry")

			assertFile(t, dir, joinRel(tc.handler, "middleware_cors.go"))
			assertFile(t, dir, joinRel(tc.redis, "connection.go"))
			assertFile(t, dir, joinRel(tc.service, "example_test.go"))
			assertFile(t, dir, joinRel(tc.domain, "aggregate.go"))
			assertFile(t, dir, joinRel(tc.domain, "event.go"))
			assertFile(t, dir, "platform/observability/metrics.go")
			assertFile(t, dir, "platform/retry/retry.go")
			assertFile(t, dir, "internal/testutil/helpers.go")
			assertNoHexagonalDirs(t, dir, tc.arch)
			assertNoNewEmptyDirs(t, dir, before)

			if shape := shapeFindings(t); len(shape) > 0 {
				t.Errorf("%s: %d architecture finding(s) after verikt add:\n  %s",
					tc.arch, len(shape), strings.Join(shape, "\n  "))
			}
		})
	}
}

// TestAddRendersArchPathsIntoTemplates: templates name their package through
// ArchPaths, which add must provide just as new does.
func TestAddRendersArchPathsIntoTemplates(t *testing.T) {
	dir := scaffoldMinimal(t, "clean")
	runAddQuiet(t, "cors")

	got, err := os.ReadFile(filepath.Join(dir, "internal", "interface", "handler", "middleware_cors.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "package handler\n") && !strings.Contains(string(got), "\npackage handler\n") {
		t.Errorf("middleware_cors.go should declare package handler, got:\n%s", got)
	}
}

// TestAddResolvesDependenciesWithPathMappings: a capability that requires
// another (saga requires event-bus) pulls the dependency in, and both land at
// the architecture's mapped paths.
func TestAddResolvesDependenciesWithPathMappings(t *testing.T) {
	skipWithoutEngine(t)

	for _, tc := range addPathCases {
		t.Run(tc.arch, func(t *testing.T) {
			dir := scaffoldMinimal(t, tc.arch)
			before := emptyDirs(t, dir)
			out := runAddQuiet(t, "saga")
			if !strings.Contains(out, "event-bus (auto-dependency)") {
				t.Errorf("expected event-bus to be auto-resolved, output:\n%s", out)
			}

			assertFile(t, dir, joinRel(tc.service, "saga/orchestrator.go"))
			assertFile(t, dir, joinRel(tc.domain, "event.go"))
			assertFile(t, dir, "platform/eventbus/bus.go")
			assertNoHexagonalDirs(t, dir, tc.arch)
			assertNoNewEmptyDirs(t, dir, before)

			if shape := shapeFindings(t); len(shape) > 0 {
				t.Errorf("%s: %d architecture finding(s) after verikt add:\n  %s",
					tc.arch, len(shape), strings.Join(shape, "\n  "))
			}
		})
	}
}

// TestAddSkipsExistingFilesAtMappedPath: a file already at the mapped path is
// never overwritten, and is reported as skipped.
func TestAddSkipsExistingFilesAtMappedPath(t *testing.T) {
	dir := scaffoldMinimal(t, "clean")
	target := filepath.Join(dir, "internal", "interface", "handler", "middleware_cors.go")
	if err := os.WriteFile(target, []byte("package handler\n// keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runAddQuiet(t, "cors")
	// Compare the mapped suffix: the reported path is absolute and may resolve
	// the temp dir through a symlink (/var vs /private/var on macOS).
	mapped := filepath.Join("internal", "interface", "handler", "middleware_cors.go")
	if !strings.Contains(out, "Skipped (exists): ") || !strings.Contains(out, mapped) || strings.Contains(out, "Created: ") {
		t.Errorf("expected %s to be reported as skipped, output:\n%s", mapped, out)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "keep me") {
		t.Errorf("existing file was overwritten:\n%s", got)
	}
}

// TestAddDryRunReportsMappedPaths: --dry-run previews the same mapped paths a
// real add writes, and writes nothing.
func TestAddDryRunReportsMappedPaths(t *testing.T) {
	dir := scaffoldMinimal(t, "clean")

	out := runAddQuiet(t, "cors", "--dry-run")
	mapped := filepath.Join("internal", "interface", "handler", "middleware_cors.go")
	if !strings.Contains(out, "Would create: ") || !strings.Contains(out, mapped) {
		t.Errorf("expected the dry run to report %s, output:\n%s", mapped, out)
	}
	if strings.Contains(out, filepath.Join("adapter", "httphandler")) {
		t.Errorf("dry run reported an unmapped hexagonal path, output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, mapped)); err == nil {
		t.Error("dry run wrote a file")
	}
}

func writeVeriktYAML(t *testing.T, dir, architectureLine string) {
	t.Helper()
	yaml := "language: go\n" + architectureLine + "capabilities:\n  - http-api\n"
	if err := os.WriteFile(filepath.Join(dir, "verikt.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertOnlyVeriktYAML(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "verikt.yaml" {
			t.Errorf("no files should be written when the architecture is invalid, found %s", e.Name())
		}
	}
}

// TestAddRejectsUnknownArchitecture: a verikt.yaml naming an architecture the
// provider does not ship fails before anything is written.
func TestAddRejectsUnknownArchitecture(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	writeVeriktYAML(t, dir, "architecture: onion\n")

	var addErr error
	captureStdout(t, func() {
		_, addErr = executeCommand(t, "add", "cors")
	})
	if addErr == nil {
		t.Fatal("expected an error for an unknown architecture")
	}
	if !strings.Contains(addErr.Error(), `unknown architecture "onion"`) || !strings.Contains(addErr.Error(), "hexagonal") {
		t.Errorf("error should name the unknown architecture and the available ones, got: %v", addErr)
	}
	assertOnlyVeriktYAML(t, dir)
}

// TestAddRejectsMissingArchitecture: without `architecture:` there are no path
// mappings to apply, so add refuses rather than guessing hexagonal.
func TestAddRejectsMissingArchitecture(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	writeVeriktYAML(t, dir, "")

	var addErr error
	captureStdout(t, func() {
		_, addErr = executeCommand(t, "add", "cors")
	})
	if addErr == nil {
		t.Fatal("expected an error for a missing architecture")
	}
	if !strings.Contains(addErr.Error(), "architecture") {
		t.Errorf("error should say the architecture is missing, got: %v", addErr)
	}
	assertOnlyVeriktYAML(t, dir)
}
