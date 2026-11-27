package scriptguard

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestClassify(t *testing.T) {
	cases := map[string]Kind{
		".github/workflows/ci.yml":       KindWorkflow,
		".github/workflows/release.yaml": KindWorkflow,
		"lefthook.yml":                   KindLefthook,
		".lefthook.yml":                  KindLefthook,
		".claude/hooks/post.sh":          KindShell,
		".claude/hooks/sub/x.sh":         KindShell,
		"scripts/install.sh":             KindShell,
		"scripts/ci/deep/x.sh":           KindShell,
		"Makefile":                       KindMakefile,
		"svc/GNUmakefile":                KindMakefile,
		"mk/rules.mk":                    KindMakefile,
		"Dockerfile":                     KindDockerfile,
		"svc/Dockerfile.dev":             KindDockerfile,
		"build/api.Dockerfile":           KindDockerfile,
		".devcontainer/Dockerfile":       KindDockerfile,
	}
	for rel, want := range cases {
		got, ok := Classify(rel, false)
		if !ok || got != want {
			t.Errorf("Classify(%q) = %v, %v; want %v", rel, got, ok, want)
		}
	}
	for _, rel := range []string{
		".github/workflows/sub/x.yml", ".github/ci.yml", "tools/scripts/x.sh", "scripts/x.py",
		"hooks/x.sh", "README.md", "docs/install.md", "Makefile.bak", "lefthook-local.yml", "main.go",
		"Dockerfile.tmpl", "svc/Makefile.tmpl",
	} {
		if k, ok := Classify(rel, false); ok {
			t.Errorf("Classify(%q) = %v, want no target", rel, k)
		}
	}
	for _, rel := range []string{"README.md", "docs/install.md", "docs/a/b.md"} {
		if k, ok := Classify(rel, true); !ok || k != KindMarkdown {
			t.Errorf("Classify(%q, docs) = %v, %v; want markdown", rel, k, ok)
		}
	}
	if _, ok := Classify("pkg/README.md", true); ok {
		t.Error("only the root README.md is a docs target")
	}
}

func TestSnippet_CutsOnRuneBoundary(t *testing.T) {
	got := snippet(strings.Repeat("a", 76) + "éééé")
	if !utf8.ValidString(got) || !strings.HasSuffix(got, "...") || len(got) > 80 {
		t.Fatalf("snippet = %q", got)
	}
}
