package cli

import (
	"strings"
	"testing"
)

func TestGlobalRulesContent_ContainsSentinel(t *testing.T) {
	content := GlobalRulesContent("0.2.0")
	sentinel := "<!-- verikt:global:v0.2.0 -->"
	if !strings.Contains(content, sentinel) {
		t.Errorf("expected sentinel %q in content", sentinel)
	}
}

func TestGlobalRulesContent_ContainsVersion(t *testing.T) {
	content := GlobalRulesContent("1.3.5")
	if !strings.Contains(content, "1.3.5") {
		t.Error("expected version string to appear in content")
	}
}

func TestGlobalRulesContent_ContainsInterview(t *testing.T) {
	content := GlobalRulesContent("0.2.0")
	if !strings.Contains(content, "## AI Interview:") {
		t.Error("expected interview protocol section in content")
	}
	// Verify the interview body includes the key steps.
	if !strings.Contains(content, "--name") {
		t.Error("expected --name flag (service name step) in interview protocol")
	}
	if !strings.Contains(content, "--no-wizard") {
		t.Error("expected scaffold command in interview protocol")
	}
}

func TestGlobalRulesContent_Under800Tokens(t *testing.T) {
	content := GlobalRulesContent("0.2.0")
	// Approximate token count: 1 token ≈ 4 characters (conservative estimate).
	// 800 tokens × 4 chars = 3200 chars.
	approxTokens := len(content) / 4
	if approxTokens > 800 {
		t.Errorf("content exceeds 800 tokens (approx %d tokens, %d chars)", approxTokens, len(content))
	}
}

func TestGlobalRulesContent_SentinelIsFirstLine(t *testing.T) {
	content := GlobalRulesContent("0.2.0")
	firstLine := strings.SplitN(content, "\n", 2)[0]
	expected := "<!-- verikt:global:v0.2.0 -->"
	if firstLine != expected {
		t.Errorf("expected sentinel as first line, got: %q", firstLine)
	}
}

func TestGlobalRulesContent_ContainsCommands(t *testing.T) {
	content := GlobalRulesContent("0.2.0")
	requiredCommands := []string{
		"verikt new",
		"verikt guide",
		"verikt check",
		"verikt init",
		"verikt analyze",
	}
	for _, cmd := range requiredCommands {
		if !strings.Contains(content, cmd) {
			t.Errorf("expected command %q in content", cmd)
		}
	}
}
