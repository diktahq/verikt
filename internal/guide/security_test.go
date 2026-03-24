package guide

import (
	"strings"
	"testing"
)

func TestHookScriptCheck_HasStrictMode(t *testing.T) {
	if !strings.Contains(hookScriptCheck, "set -euo pipefail") {
		t.Error("hookScriptCheck: missing 'set -euo pipefail' — scripts must fail on errors")
	}
}

func TestHookScriptGuideRefresh_HasStrictMode(t *testing.T) {
	if !strings.Contains(hookScriptGuideRefresh, "set -euo pipefail") {
		t.Error("hookScriptGuideRefresh: missing 'set -euo pipefail' — scripts must fail on errors")
	}
}

func TestHookScriptCheck_QuotesExitCode(t *testing.T) {
	if strings.Contains(hookScriptCheck, "[ $EXIT_CODE") {
		t.Error("hookScriptCheck: unquoted $EXIT_CODE in test expression — use \"$EXIT_CODE\"")
	}
}

func TestHookScriptGuideRefresh_AtomicHashWrite(t *testing.T) {
	// The hash file should be written atomically via mktemp + mv, not direct > redirect.
	if strings.Contains(hookScriptGuideRefresh, "> \"$HASH_FILE\"") && !strings.Contains(hookScriptGuideRefresh, "mktemp") {
		t.Error("hookScriptGuideRefresh: hash file should be written atomically via mktemp + mv")
	}
}
