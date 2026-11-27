package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSilentCISwallow(t *testing.T) {
	cases := []struct {
		name    string
		cmds    []string
		wantErr string
	}{
		{"none", nil, ""},
		{"names and prefixes", []string{"migrate-db", "deno test", "./scripts/smoke"}, ""},
		{"blank", []string{"  "}, "is empty"},
		{"pipeline", []string{"go test | tee"}, "shell syntax"},
		{"substitution", []string{"$(check)"}, "shell syntax"},
		{"assignment", []string{"X=1 check"}, "shell syntax"},
		{"duplicate after whitespace folding", []string{"deno test", "deno  test"}, "duplicated"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := ValidateSilentCISwallow(SilentCISwallowConfig{ExtraCommands: c.cmds})
			if c.wantErr == "" {
				if len(errs) != 0 {
					t.Fatalf("unexpected errors: %v", errs)
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), c.wantErr) {
				t.Fatalf("errors = %v, want one containing %q", errs, c.wantErr)
			}
		})
	}
}

func TestValidateVeriktYAML_IncludesSilentCISwallow(t *testing.T) {
	cfg := DefaultVeriktConfig("go", "flat")
	cfg.Check.SilentCISwallow.ExtraCommands = []string{""}
	errs := ValidateVeriktYAML(cfg)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "extra_commands[0]") {
		t.Fatalf("errors = %v, want the extra_commands error", errs)
	}
}

func TestLoadVeriktYAML_GuardConfigKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verikt.yaml")
	data := `language: go
architecture: flat
check:
  silent_ci_swallow:
    extra_commands: [db-up, "deno test"]
  mutable_install_url:
    include_docs: true
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadVeriktYAML(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.Check.SilentCISwallow.ExtraCommands, ","); got != "db-up,deno test" {
		t.Errorf("extra_commands = %q", got)
	}
	if !cfg.Check.MutableInstallURL.IncludeDocs {
		t.Error("include_docs = false, want true")
	}

	// Round trip keeps both keys and omits them when unset.
	if err := SaveVeriktYAML(path, cfg); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(path)
	for _, want := range []string{"silent_ci_swallow:", "extra_commands:", "include_docs: true"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("saved yaml lacks %q:\n%s", want, out)
		}
	}
	if err := SaveVeriktYAML(path, DefaultVeriktConfig("go", "flat")); err != nil {
		t.Fatal(err)
	}
	out, _ = os.ReadFile(path)
	if strings.Contains(string(out), "silent_ci_swallow") || strings.Contains(string(out), "mutable_install_url") {
		t.Errorf("unset guard config written:\n%s", out)
	}
}
