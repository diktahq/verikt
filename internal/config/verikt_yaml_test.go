package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadVeriktYAML(t *testing.T) {
	path := filepath.Join("testdata", "verikt.yaml")
	cfg, err := LoadVeriktYAML(path)
	if err != nil {
		t.Fatalf("LoadVeriktYAML() error = %v", err)
	}
	if cfg.Language != "go" {
		t.Fatalf("Language = %q, want go", cfg.Language)
	}
	if cfg.Architecture != "hexagonal" {
		t.Fatalf("Architecture = %q, want hexagonal", cfg.Architecture)
	}
	if len(cfg.Components) != 5 {
		t.Fatalf("Components len = %d, want 5", len(cfg.Components))
	}
	if cfg.Components[0].Name != "domain" {
		t.Fatalf("Components[0].Name = %q, want domain", cfg.Components[0].Name)
	}
}

func TestValidateVeriktYAML(t *testing.T) {
	errs := ValidateVeriktYAML(&VeriktConfig{})
	if len(errs) == 0 {
		t.Fatal("expected validation errors")
	}
	valid := DefaultVeriktConfig("go", "hexagonal")
	if got := ValidateVeriktYAML(valid); len(got) != 0 {
		t.Fatalf("unexpected validation errors: %v", got)
	}
}

func TestSaveLoadVeriktYAMLRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verikt.yaml")
	in := DefaultVeriktConfig("go", "hexagonal")
	if err := SaveVeriktYAML(path, in); err != nil {
		t.Fatalf("SaveVeriktYAML() error = %v", err)
	}
	out, err := LoadVeriktYAML(path)
	if err != nil {
		t.Fatalf("LoadVeriktYAML() error = %v", err)
	}
	if out.Language != in.Language || out.Architecture != in.Architecture {
		t.Fatalf("round-trip mismatch: in=%+v out=%+v", in, out)
	}
}

func TestFindVeriktYAML(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	nested := filepath.Join(project, "internal", "adapter")
	if err := mkdirAll(nested); err != nil {
		t.Fatalf("mkdirAll: %v", err)
	}
	cfg := DefaultVeriktConfig("go", "hexagonal")
	path := filepath.Join(project, "verikt.yaml")
	if err := SaveVeriktYAML(path, cfg); err != nil {
		t.Fatalf("SaveVeriktYAML: %v", err)
	}
	found, err := FindVeriktYAML(nested)
	if err != nil {
		t.Fatalf("FindVeriktYAML: %v", err)
	}
	if found != path {
		t.Fatalf("FindVeriktYAML = %q, want %q", found, path)
	}
}

func TestValidateComponentUniqueNames(t *testing.T) {
	cfg := DefaultVeriktConfig("go", "hexagonal")
	cfg.Components = append(cfg.Components, Component{Name: "domain", In: []string{"other/**"}, MayDependOn: []string{}})
	errs := ValidateVeriktYAML(cfg)
	if len(errs) == 0 {
		t.Fatal("expected duplicate name error")
	}
}

func TestValidateComponentNoSelfReference(t *testing.T) {
	cfg := DefaultVeriktConfig("go", "hexagonal")
	cfg.Components = []Component{
		{Name: "domain", In: []string{"domain/**"}, MayDependOn: []string{"domain"}},
	}
	errs := ValidateVeriktYAML(cfg)
	if len(errs) == 0 {
		t.Fatal("expected self-reference error")
	}
}

func TestValidateComponentUnknownDependency(t *testing.T) {
	cfg := DefaultVeriktConfig("go", "hexagonal")
	cfg.Components = []Component{
		{Name: "domain", In: []string{"domain/**"}, MayDependOn: []string{"nonexistent"}},
	}
	errs := ValidateVeriktYAML(cfg)
	if len(errs) == 0 {
		t.Fatal("expected unknown dependency error")
	}
}

func TestSeverityOverrides_ParsesValidYAML(t *testing.T) {
	yaml := `
language: go
architecture: hexagonal
severity_overrides:
  max_lines:
    - severity: ignore
      reason: "Generated code"
      paths: [generated/**]
    - severity: should
      reason: "Test helpers can be longer"
      paths: ["**/*_test.go"]
  orphan_package:
    - severity: ignore
      reason: "Scripts directory is intentionally outside components"
      paths: [scripts/**]
  arch/domain:
    - severity: should
      reason: "Legacy adapter, will be migrated"
      paths: [adapter/legacy/**]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "verikt.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cfg, err := LoadVeriktYAML(path)
	if err != nil {
		t.Fatalf("LoadVeriktYAML() error = %v", err)
	}
	if len(cfg.SeverityOverrides) != 3 {
		t.Fatalf("SeverityOverrides len = %d, want 3", len(cfg.SeverityOverrides))
	}
	maxLines := cfg.SeverityOverrides["max_lines"]
	if len(maxLines) != 2 {
		t.Fatalf("max_lines entries = %d, want 2", len(maxLines))
	}
	if maxLines[0].Severity != "ignore" {
		t.Fatalf("max_lines[0].Severity = %q, want ignore", maxLines[0].Severity)
	}
	if maxLines[0].Reason != "Generated code" {
		t.Fatalf("max_lines[0].Reason = %q, want 'Generated code'", maxLines[0].Reason)
	}
	if len(maxLines[0].Paths) != 1 || maxLines[0].Paths[0] != "generated/**" {
		t.Fatalf("max_lines[0].Paths = %v, want [generated/**]", maxLines[0].Paths)
	}
	archDomain := cfg.SeverityOverrides["arch/domain"]
	if len(archDomain) != 1 || archDomain[0].Severity != "should" {
		t.Fatalf("arch/domain entry unexpected: %+v", archDomain)
	}
}

func TestSeverityOverrides_RejectsEmptyReason(t *testing.T) {
	cfg := DefaultVeriktConfig("go", "hexagonal")
	cfg.SeverityOverrides = SeverityOverrides{
		"max_lines": []SeverityOverride{
			{Severity: "ignore", Reason: ""},
		},
	}
	errs := ValidateVeriktYAML(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error for empty reason")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "reason is required") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'reason is required' error, got: %v", errs)
	}
}

func TestSeverityOverrides_RejectsInvalidSeverity(t *testing.T) {
	cfg := DefaultVeriktConfig("go", "hexagonal")
	cfg.SeverityOverrides = SeverityOverrides{
		"max_lines": []SeverityOverride{
			{Severity: "critical", Reason: "some reason"},
		},
	}
	errs := ValidateVeriktYAML(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error for invalid severity")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "is invalid") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'is invalid' error, got: %v", errs)
	}
}

func TestSeverityOverrides_WarnsCatchAllNotLast(t *testing.T) {
	cfg := DefaultVeriktConfig("go", "hexagonal")
	cfg.SeverityOverrides = SeverityOverrides{
		"max_lines": []SeverityOverride{
			// catch-all (no paths) placed first — subsequent entry is unreachable
			{Severity: "ignore", Reason: "catch-all"},
			{Severity: "should", Reason: "test files", Paths: []string{"**/*_test.go"}},
		},
	}
	errs := ValidateVeriktYAML(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error for catch-all not last")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "unreachable") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'unreachable' error, got: %v", errs)
	}
}

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}
