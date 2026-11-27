package checker

import (
	"testing"

	"github.com/diktahq/verikt/internal/config"
)

// TestScriptGuards_VeriktRepoIsClean dogfoods silent_ci_swallow and
// mutable_install_url on this repository with default settings: verikt's own
// workflows, Makefile, lefthook.yml and scripts must pass the checks it ships.
func TestScriptGuards_VeriktRepoIsClean(t *testing.T) {
	cfg, err := config.LoadVeriktYAML("../../verikt.yaml")
	if err != nil {
		t.Fatal(err)
	}
	violations, waived := detectScriptGuards("../..", cfg.Check)
	for _, v := range violations {
		t.Errorf("%s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
	}
	for _, w := range waived {
		t.Logf("waived %s:%d [%s] %s", w.File, w.Line, w.Rule, w.Reason)
	}
}
