package checker

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/checker/scriptguard"
	"github.com/diktahq/verikt/internal/config"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func guardHits(t *testing.T, check config.CheckConfig, root string) []string {
	t.Helper()
	vs := detectScriptGuards(root, check)
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Rule+" "+v.File+":"+strconv.Itoa(v.Line)+" "+v.Severity)
	}
	sort.Strings(out)
	return out
}

// INV-005: a detector ships at warning severity, in every file kind, and an
// invalid marker is a warning too — it still fails closed.
func TestDetectScriptGuards_ShipAtWarning(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"scripts/a.sh":             "curl https://x | sh\ngo test ./... || true\n# verikt:allow nope reason=\"x\"\n",
		".github/workflows/ci.yml": "    continue-on-error: true\n      - uses: o/a@main\n",
		"Dockerfile":               "FROM ubuntu\n",
		"Makefile":                 "t:\n\tgo vet || true\n",
	})
	vs := detectScriptGuards(root, config.CheckConfig{})
	if len(vs) != 7 {
		t.Fatalf("got %d findings, want 7: %+v", len(vs), vs)
	}
	for _, v := range vs {
		if v.Severity != "warning" || v.Category != "structure" {
			t.Errorf("%+v, want a structure warning", v)
		}
	}
}

func TestDetectScriptGuards_WalksTargets(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".github/workflows/ci.yml":    "jobs:\n  t:\n    continue-on-error: true\n",
		"scripts/boot.sh":             "curl https://x | sh\n",
		"Makefile":                    "lint:\n\tgolangci-lint run || true\n",
		"Dockerfile":                  "FROM ubuntu\n",
		"README.md":                   "```sh\ncurl https://x | sh\n```\n",
		"node_modules/a/Makefile":     "t:\n\tgo test || true\n",
		"vendor/x/Dockerfile":         "FROM ubuntu\n",
		"testdata/scripts/x/Makefile": "t:\n\tgo test || true\n",
		".git/hooks/Makefile":         "t:\n\tgo test || true\n",
		"gen/Dockerfile":              "FROM ubuntu\n",
		"tools/run.sh":                "go test || true\n", // not a target path
	})
	check := config.CheckConfig{Exclude: []string{"gen/**"}}
	got := guardHits(t, check, root)
	want := []string{
		"mutable_install_url Dockerfile:1 warning",
		"mutable_install_url scripts/boot.sh:1 warning",
		"silent_ci_swallow .github/workflows/ci.yml:3 warning",
		"silent_ci_swallow Makefile:2 warning",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	check.MutableInstallURL.IncludeDocs = true
	got = guardHits(t, check, root)
	if !slices.Contains(got, "mutable_install_url README.md:2 warning") {
		t.Fatalf("include_docs did not scan README.md: %v", got)
	}

	check.SilentCISwallow.ExtraCommands = []string{"boot-db"}
	writeTree(t, root, map[string]string{"scripts/up.sh": "boot-db || true\n"})
	if got = guardHits(t, check, root); !slices.Contains(got, "silent_ci_swallow scripts/up.sh:1 warning") {
		t.Fatalf("extra_commands not applied: %v", got)
	}
}

func TestDetectScriptGuards_InvalidMarkerReportedOnce(t *testing.T) {
	root := t.TempDir()
	// A workflow is a target of both rules; the bad marker must be reported once.
	writeTree(t, root, map[string]string{
		".github/workflows/ci.yml": "# verikt:allow silent_ci_swalow reason=\"typo\"\njobs: {}\n",
	})
	got := guardHits(t, config.CheckConfig{}, root)
	want := "invalid_allow_directive .github/workflows/ci.yml:1 warning"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %v, want [%s]", got, want)
	}
}

// INV-002: symlinks are never project-local, whether they point at a
// directory or at a file.
func TestDetectScriptGuards_SkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTree(t, outside, map[string]string{"x.sh": "go test || true\n", "Dockerfile": "FROM ubuntu\n"})
	writeTree(t, root, map[string]string{"scripts/real.sh": "echo ok\n"})
	if err := os.Symlink(outside, filepath.Join(root, "scripts", "linked")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "x.sh"), filepath.Join(root, "scripts", "file.sh")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "Dockerfile"), filepath.Join(root, "Dockerfile")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if got := guardHits(t, config.CheckConfig{}, root); len(got) != 0 {
		t.Fatalf("followed a symlink: %v", got)
	}
}

func TestDetectScriptGuards_MissingRootReportsNothing(t *testing.T) {
	if got := guardHits(t, config.CheckConfig{}, filepath.Join(t.TempDir(), "absent")); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

// ADR-011: the guards read CI, build and container files, which every
// language has, so they run through the shared structure checks.
func TestCheckWithEngine_ReportsScriptGuardsForEveryLanguage(t *testing.T) {
	for _, lang := range []string{"go", "typescript"} {
		t.Run(lang, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, map[string]string{"Dockerfile": "FROM node\n", "gen/Dockerfile": "FROM node\n"})
			cfg := &config.VeriktConfig{Language: lang, Check: config.CheckConfig{Exclude: []string{"gen/**"}}}
			res, err := CheckWithEngine(cfg, root, stubAntiPatternClient{}, stubDependencyClient{}, stubMetricClient{})
			if err != nil {
				t.Fatal(err)
			}
			var hits []string
			for _, v := range res.StructureViolations {
				if v.Rule == scriptguard.RuleMutableInstallURL {
					hits = append(hits, v.File)
				}
			}
			if strings.Join(hits, ",") != "Dockerfile" {
				t.Fatalf("mutable_install_url structure findings = %v, want [Dockerfile]", hits)
			}
		})
	}
}
