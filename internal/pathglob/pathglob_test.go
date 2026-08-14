package pathglob

import "testing"

func TestMatch(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		// The reported failure: check.exclude ["gen/**"] dropped a
		// sql_concatenation finding in internal/agent/db.go, because "agent"
		// contains "gen". The check reported "✓ All checks pass", exit 0.
		{"unrelated path sharing a substring", "gen/**", "internal/agent/db.go", false},
		{"directory contents", "gen/**", "gen/proto.go", true},
		{"the directory itself", "gen/**", "gen", true},
		{"nested below the directory", "gen/**", "gen/pb/engine.go", true},
		{"same name deeper in the tree is not matched", "gen/**", "internal/gen/proto.go", false},
		{"multi-segment prefix", "internal/engineclient/pb/**", "internal/engineclient/pb/engine.pb.go", true},
		{"partial segment is not a prefix", "internal/engine/**", "internal/engineclient/pb/x.go", false},

		{"basename pattern", "**/*_test.go", "internal/cli/check_test.go", true},
		{"basename pattern misses non-tests", "**/*_test.go", "internal/cli/check.go", false},
		{"multi-segment doublestar tail", "**/adapter/*.go", "src/internal/adapter/db.go", true},

		// "at any depth" has to be spelled out. This is the difference between
		// excluding the project's own testdata/ and excluding every testdata
		// directory in the tree, and the two should not be the same pattern.
		{"nested directory at any depth", "**/testdata/**", "internal/checker/testdata/fixture/bad.go", true},
		{"nested directory at root too", "**/testdata/**", "testdata/top.go", true},
		{"anchored form does not reach nested dirs", "testdata/**", "internal/checker/testdata/bad.go", false},
		{"any-depth form still needs the segment", "**/testdata/**", "internal/checker/data/bad.go", false},

		{"plain name in root", "*.go", "main.go", true},
		{"plain name does not cross directories", "*.go", "internal/main.go", false},
		{"exact path", "Makefile", "Makefile", true},

		{"everything", "**", "any/path/at/all.go", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Match(tt.pattern, tt.path); got != tt.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchAny(t *testing.T) {
	patterns := []string{"experiments/**", "internal/engineclient/pb/**"}

	if !MatchAny("experiments/EXP-04/main.go", patterns) {
		t.Error("expected a path under experiments/ to match")
	}
	if MatchAny("internal/checker/checker.go", patterns) {
		t.Error("a path matching no pattern must not be excluded")
	}
	if MatchAny("internal/checker/checker.go", nil) {
		t.Error("an empty pattern list matches nothing")
	}
}
