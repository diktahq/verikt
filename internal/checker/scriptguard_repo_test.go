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
	for _, v := range detectScriptGuards("../..", cfg.Check) {
		t.Errorf("%s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
	}
}
