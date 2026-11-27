package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/engineclient"
)

// shapeCategories are the violation categories that mean a scaffolded project
// does not match the architecture verikt.yaml declares: packages that match no
// component, declared components with no code, forbidden imports between
// components, and missing or forbidden directories. Function metrics and
// anti-patterns are code-quality findings about template bodies, not about the
// architecture shape, so they are out of scope here.
var shapeCategories = map[string]bool{
	"architecture": true,
	"dependency":   true,
	"structure":    true,
}

// fullGoCapabilities is every Go capability that writes Go code, so between
// them they write into every directory a capability template can: adapter/
// (and each architecture's mapping of it), domain/, service/, platform/,
// config/, cmd/, internal/bootstrap and internal/{lifecycle,migrate,testutil,
// validation}.
//
// graphql, templ and htmx (which requires templ) are left out. graphql and
// templ write root tools_*.go files (package tools, behind a build tag) that no
// component claims, and graphql's gqlgen.yml hardcodes adapter/graphql, so its
// package cannot be mapped into clean or layered without desynchronising code
// generation. Those are capability-template fixes, not architecture-manifest
// ones.
const fullGoCapabilities = "api-versioning,audit-log,auth-jwt,bff,bootstrap,bulkhead,circuit-breaker,cors," +
	"cqrs,ddd,dynamodb,elasticsearch,email-gateway,encryption,event-bus,feature-flags,graceful,grpc,health," +
	"http-api,http-client,i18n,idempotency,kafka-consumer,mailpit,migrations,mongodb,multi-tenancy," +
	"mysql,nats,oauth2,observability,outbox,platform,postgres,rate-limiting,redis,repository,request-id," +
	"retry,s3,saga,scheduler,sqlite,squirrel,sse,static-assets,testing,timeout,uuid,validation,websocket,worker"

// TestScaffoldPassesCheck scaffolds each Go architecture and runs `verikt check`
// on the result. A freshly generated project must conform to the architecture
// it declares (issue #18). It needs no network: hooks (go mod tidy) are
// skipped, and check's orphan, component and import-graph analysis read
// source files only.
func TestScaffoldPassesCheck(t *testing.T) {
	if _, err := engineclient.EnginePath(); err != nil {
		t.Skipf("verikt-engine is not available for this platform, so check cannot run: %v", err)
	}

	cases := []struct {
		arch string
		caps string
	}{
		{"hexagonal", "http-api"},
		{"hexagonal", fullGoCapabilities},
		{"clean", "http-api"},
		{"clean", fullGoCapabilities},
		{"layered", "http-api"},
		{"layered", fullGoCapabilities},
		{"flat", "http-api"},
		{"flat", fullGoCapabilities},
	}

	for _, tc := range cases {
		name := tc.arch + "/minimal"
		if tc.caps != "http-api" {
			name = tc.arch + "/full"
		}
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			chdir(t, tmp)
			if _, err := executeCommand(t,
				"new", "svc",
				"--arch", tc.arch,
				"--cap", tc.caps,
				"--module", "example.com/svc",
				"--no-wizard",
				"--set", "skip_hooks=true",
			); err != nil {
				t.Fatalf("scaffold %s with %s: %v", tc.arch, tc.caps, err)
			}

			svcDir := filepath.Join(tmp, "svc")
			chdir(t, svcDir)
			var checkErr error
			out := captureStdout(t, func() {
				_, checkErr = executeCommand(t, "check", "--path", ".", "-o", "json")
			})

			var report struct {
				Result     string              `json:"result"`
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
			if len(shape) > 0 {
				t.Errorf("scaffolded %s project (caps %s) has %d architecture violation(s):\n  %s",
					tc.arch, tc.caps, len(shape), strings.Join(shape, "\n  "))
			}
		})
	}
}

// writeLayeredFixture writes a two-component Go project in which domain may
// depend on nothing and adapter may depend on domain. extra maps
// project-relative file paths to their contents.
func writeLayeredFixture(t *testing.T, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":             "module example.com/fx\n\ngo 1.22\n",
		"domain/order.go":    "package domain\n\ntype Order struct{}\n",
		"adapter/adapter.go": "package adapter\n\nimport \"example.com/fx/domain\"\n\nvar _ domain.Order\n",
		"verikt.yaml": `language: go
architecture: hexagonal
components:
  - name: domain
    in: ["domain/**"]
    may_depend_on: []
  - name: adapters
    in: ["adapter/**"]
    may_depend_on: [domain]
`,
	}
	for k, v := range extra {
		files[k] = v
	}
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const domainImportsAdapter = "import \"example.com/fx/adapter\"\n\nvar _ = adapter.Name\n"

func dependencyViolations(t *testing.T, dir string) []checker.Violation {
	t.Helper()
	chdir(t, dir)
	out := captureStdout(t, func() {
		_, _ = executeCommand(t, "check", "--path", ".", "-o", "json")
	})
	var report struct {
		Violations []checker.Violation `json:"violations"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("check did not print a JSON report: %v\noutput:\n%s", err, out)
	}
	var deps []checker.Violation
	for _, v := range report.Violations {
		if v.Category == "dependency" {
			deps = append(deps, v)
		}
	}
	return deps
}

// TestCheckIgnoresTestFileImportEdges: dependency rules govern production code
// (INV-004). A domain test importing an adapter (for a fixture or a fake) is
// not a domain dependency on adapters. Scaffolded test helpers rely on this.
func TestCheckIgnoresTestFileImportEdges(t *testing.T) {
	if _, err := engineclient.EnginePath(); err != nil {
		t.Skipf("verikt-engine is not available: %v", err)
	}
	dir := writeLayeredFixture(t, map[string]string{
		"adapter/name.go":      "package adapter\n\nconst Name = \"a\"\n",
		"domain/order_test.go": "package domain\n\n" + domainImportsAdapter,
	})
	if deps := dependencyViolations(t, dir); len(deps) != 0 {
		t.Errorf("import in a _test.go file was flagged: %+v", deps)
	}
}

// TestCheckFlagsProductionImportEdges is the control for the test above: the
// same import in a non-test file is a violation.
func TestCheckFlagsProductionImportEdges(t *testing.T) {
	if _, err := engineclient.EnginePath(); err != nil {
		t.Skipf("verikt-engine is not available: %v", err)
	}
	dir := writeLayeredFixture(t, map[string]string{
		"adapter/name.go": "package adapter\n\nconst Name = \"a\"\n",
		"domain/leak.go":  "package domain\n\n" + domainImportsAdapter,
	})
	deps := dependencyViolations(t, dir)
	if len(deps) == 0 {
		t.Fatal("production import domain -> adapter was not flagged")
	}
	for _, v := range deps {
		if !strings.HasSuffix(v.File, "leak.go") {
			t.Errorf("unexpected dependency violation: %+v", v)
		}
	}
}
