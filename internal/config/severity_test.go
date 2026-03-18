package config

import (
	"testing"
)

func makeOverrides() SeverityOverrides {
	return SeverityOverrides{
		"max_lines": {
			{Severity: "ignore", Reason: "Generated code", Paths: []string{"generated/**"}},
			{Severity: "should", Reason: "Test helpers can be longer", Paths: []string{"**/*_test.go"}},
			{Severity: "must", Reason: "Default strict", Paths: nil},
		},
		"orphan_package": {
			{Severity: "ignore", Reason: "Scripts directory", Paths: []string{"scripts/**"}},
		},
	}
}

func TestResolveSeverity_NoOverrides_ReturnsDefault(t *testing.T) {
	got := ResolveSeverity(SeverityOverrides{}, "max_lines", "internal/foo.go", "error")
	if got != "error" {
		t.Errorf("got %q, want %q", got, "error")
	}
}

func TestResolveSeverity_UnknownKey_ReturnsDefault(t *testing.T) {
	got := ResolveSeverity(makeOverrides(), "unknown_rule", "internal/foo.go", "warning")
	if got != "warning" {
		t.Errorf("got %q, want %q", got, "warning")
	}
}

func TestResolveSeverity_ExactPathMatch(t *testing.T) {
	overrides := SeverityOverrides{
		"required_dir": {
			{Severity: "ignore", Reason: "Optional for this project", Paths: []string{"cmd/"}},
		},
	}
	got := ResolveSeverity(overrides, "required_dir", "cmd/", "error")
	if got != "ignore" {
		t.Errorf("got %q, want %q", got, "ignore")
	}
}

func TestResolveSeverity_GlobMatch(t *testing.T) {
	got := ResolveSeverity(makeOverrides(), "orphan_package", "scripts/migrate.go", "error")
	if got != "ignore" {
		t.Errorf("got %q, want %q", got, "ignore")
	}
}

func TestResolveSeverity_GlobNoMatch_ReturnsDefault(t *testing.T) {
	got := ResolveSeverity(makeOverrides(), "orphan_package", "internal/migrate.go", "error")
	if got != "error" {
		t.Errorf("got %q, want %q", got, "error")
	}
}

func TestResolveSeverity_FirstMatchWins(t *testing.T) {
	// "generated/**" should match before the catch-all "must" entry.
	got := ResolveSeverity(makeOverrides(), "max_lines", "generated/proto.go", "error")
	if got != "ignore" {
		t.Errorf("got %q, want %q", got, "ignore")
	}
}

func TestResolveSeverity_CatchAllFallback(t *testing.T) {
	// A file not in generated/** or a test file — hits the catch-all.
	got := ResolveSeverity(makeOverrides(), "max_lines", "internal/service.go", "error")
	if got != "must" {
		t.Errorf("got %q, want %q", got, "must")
	}
}

func TestResolveSeverity_TestFilePattern(t *testing.T) {
	got := ResolveSeverity(makeOverrides(), "max_lines", "internal/service_test.go", "error")
	if got != "should" {
		t.Errorf("got %q, want %q", got, "should")
	}
}

func TestMapSeverity_MustToError(t *testing.T) {
	if got := MapSeverity("must"); got != "error" {
		t.Errorf("got %q, want %q", got, "error")
	}
}

func TestMapSeverity_ShouldToWarning(t *testing.T) {
	if got := MapSeverity("should"); got != "warning" {
		t.Errorf("got %q, want %q", got, "warning")
	}
}

func TestMapSeverity_IgnoreToIgnore(t *testing.T) {
	if got := MapSeverity("ignore"); got != "ignore" {
		t.Errorf("got %q, want %q", got, "ignore")
	}
}

func TestMapSeverity_UnknownPassthrough(t *testing.T) {
	if got := MapSeverity("error"); got != "error" {
		t.Errorf("got %q, want %q", got, "error")
	}
}

func TestResolveReason_ReturnsMatchedReason(t *testing.T) {
	got := ResolveReason(makeOverrides(), "orphan_package", "scripts/migrate.go")
	if got != "Scripts directory" {
		t.Errorf("got %q, want %q", got, "Scripts directory")
	}
}

func TestResolveReason_NoMatch_ReturnsEmpty(t *testing.T) {
	got := ResolveReason(makeOverrides(), "orphan_package", "internal/service.go")
	if got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}
