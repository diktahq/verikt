package scriptguard

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// findings scans src as kind and returns "rule@line" strings, sorted.
func findings(t *testing.T, kind Kind, src string, opts Options) []string {
	t.Helper()
	vs := Scan("x", kind, []byte(src), opts)
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		if v.File != "x" || v.Message == "" {
			t.Errorf("bad violation %+v", v)
		}
		out = append(out, fmt.Sprintf("%s@%d", v.Rule, v.Line))
	}
	sort.Strings(out)
	return out
}

type scanCase struct {
	name string
	kind Kind
	src  string
	want []string // "rule@line"; nil = clean
	opts Options
}

func runScanCases(t *testing.T, cases []scanCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := findings(t, c.kind, c.src, c.opts)
			want := append([]string{}, c.want...)
			sort.Strings(want)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("findings = %v, want %v\nsource:\n%s", got, want, c.src)
			}
		})
	}
}

const sw = RuleSilentCISwallow

func TestSilentCISwallow_ContinueOnError(t *testing.T) {
	runScanCases(t, []scanCase{
		{name: "literal true on a step", kind: KindWorkflow, src: "steps:\n  - run: go test ./...\n    continue-on-error: true\n", want: []string{sw + "@3"}},
		{name: "on a job", kind: KindWorkflow, src: "jobs:\n  lint:\n    continue-on-error: true\n", want: []string{sw + "@3"}},
		{name: "list item form", kind: KindWorkflow, src: "- continue-on-error: true\n", want: []string{sw + "@1"}},
		{name: "quoted and capitalised", kind: KindWorkflow, src: "    continue-on-error: 'True'\n", want: []string{sw + "@1"}},
		{name: "trailing comment", kind: KindWorkflow, src: "    continue-on-error: true # flaky\n", want: []string{sw + "@1"}},
		{name: "expression is not flagged", kind: KindWorkflow, src: "    continue-on-error: ${{ matrix.experimental }}\n"},
		{name: "false is not flagged", kind: KindWorkflow, src: "    continue-on-error: false\n"},
		{name: "commented out", kind: KindWorkflow, src: "    # continue-on-error: true\n"},
		{name: "only in workflows", kind: KindLefthook, src: "continue-on-error: true\n"},
		{name: "allowed with marker above", kind: KindWorkflow, src: "  - name: notify\n    # verikt:allow silent_ci_swallow reason=\"Slack outage must not block release\"\n    continue-on-error: true\n"},
		{name: "allowed trailing", kind: KindWorkflow, src: "    continue-on-error: true # verikt:allow silent_ci_swallow reason=\"best effort\"\n"},
		{name: "invalid marker still reports", kind: KindWorkflow, src: "    continue-on-error: true # verikt:allow-continue-on-error reason=\"known flaky\"\n",
			want: []string{sw + "@1", RuleInvalidAllowDirective + "@1"}},
	})
}

func TestSilentCISwallow_ExitCodeRead(t *testing.T) {
	runScanCases(t, []scanCase{
		{name: "same line", kind: KindShell, src: "sync-schema || true; RC=$?\n", want: []string{sw + "@1"}},
		{name: "next line", kind: KindShell, src: "deploy-thing || true\nEXIT_CODE=$?\n", want: []string{sw + "@1"}},
		{name: "next line after comment", kind: KindShell, src: "deploy-thing || :\n# capture\nif [ $? -ne 0 ]; then exit 1; fi\n", want: []string{sw + "@1"}},
		{name: "make escaped", kind: KindMakefile, src: "x:\n\tdo-it || true; echo $$?\n", want: []string{sw + "@2"}},
		{name: "braced", kind: KindShell, src: "x || true\nexit ${?}\n", want: []string{sw + "@1"}},
		{name: "no read", kind: KindShell, src: "mkdir -p out || true\necho done\n"},
		{name: "read two lines later", kind: KindShell, src: "foo || true\necho hi\necho $?\n"},
		{name: "set +e capture is fine", kind: KindShell, src: "set +e\nRESULT=$(verikt check)\nEXIT_CODE=$?\nset -e\n"},
	})
}

func TestSilentCISwallow_VerificationCommands(t *testing.T) {
	runScanCases(t, []scanCase{
		// From the issue and the design report.
		{name: "migrate-up", kind: KindShell, src: "migrate-up || true\n", want: []string{sw + "@1"}},
		{name: "captured verikt check", kind: KindShell, src: "RESULT=$(verikt check) || true\n", want: []string{sw + "@1"}},
		{name: "captured with cd first", kind: KindShell, src: "OUT=$(cd svc && go test ./...) || true\n", want: []string{sw + "@1"}},
		{name: "export capture", kind: KindShell, src: "export OUT=$(golangci-lint run) || true\n", want: []string{sw + "@1"}},
		{name: "go test", kind: KindShell, src: "go test ./... || true\n", want: []string{sw + "@1"}},
		{name: "go vet colon", kind: KindShell, src: "go vet ./... || :\n", want: []string{sw + "@1"}},
		{name: "exit 0", kind: KindShell, src: "pytest -q || exit 0\n", want: []string{sw + "@1"}},
		{name: "after &&", kind: KindShell, src: "cd app && npm run lint || true\n", want: []string{sw + "@1"}},
		{name: "npm test", kind: KindShell, src: "npm test || true\n", want: []string{sw + "@1"}},
		{name: "pnpm run test:unit", kind: KindShell, src: "pnpm run test:unit || true\n", want: []string{sw + "@1"}},
		{name: "env prefix and sudo", kind: KindShell, src: "CI=1 sudo pytest || true\n", want: []string{sw + "@1"}},
		{name: "npx eslint", kind: KindShell, src: "npx eslint . || true\n", want: []string{sw + "@1"}},
		{name: "python -m mypy", kind: KindShell, src: "python3 -m mypy src || true\n", want: []string{sw + "@1"}},
		{name: "manage.py migrate", kind: KindShell, src: "python manage.py migrate || true\n", want: []string{sw + "@1"}},
		{name: "rails db:migrate", kind: KindShell, src: "bundle exec rails db:migrate || true\n", want: []string{sw + "@1"}},
		{name: "prisma migrate", kind: KindShell, src: "npx prisma migrate deploy || true\n", want: []string{sw + "@1"}},
		{name: "make target", kind: KindShell, src: "make -j4 test-short || true\n", want: []string{sw + "@1"}},
		{name: "gradle check", kind: KindShell, src: "./gradlew check || true\n", want: []string{sw + "@1"}},
		{name: "cargo clippy", kind: KindShell, src: "cargo clippy --all || true\n", want: []string{sw + "@1"}},
		{name: "script path", kind: KindShell, src: "./scripts/run-tests.sh || true\n", want: []string{sw + "@1"}},
		{name: "bash script", kind: KindShell, src: "bash scripts/lint.sh || true\n", want: []string{sw + "@1"}},
		{name: "pipeline stage", kind: KindShell, src: "go test ./... | tee out.txt || true\n", want: []string{sw + "@1"}},
		{name: "subshell", kind: KindShell, src: "( go test ./... || true )\n", want: []string{sw + "@1"}},
		{name: "then keyword", kind: KindShell, src: "if true; then go vet || true; fi\n", want: []string{sw + "@1"}},
		{name: "continuation keeps start line", kind: KindShell, src: "echo a\ngolangci-lint run \\\n  ./... \\\n  || true\n", want: []string{sw + "@2"}},
		{name: "workflow run line", kind: KindWorkflow, src: "      - run: go test ./... || true\n", want: []string{sw + "@1"}},
		{name: "workflow run block", kind: KindWorkflow, src: "      - run: |\n          go build ./...\n          go test ./... || true\n", want: []string{sw + "@3"}},
		{name: "lefthook run", kind: KindLefthook, src: "pre-commit:\n  commands:\n    lint:\n      run: golangci-lint run || true\n", want: []string{sw + "@4"}},
		{name: "makefile recipe prefix", kind: KindMakefile, src: "lint:\n\t-@golangci-lint run || true\n", want: []string{sw + "@2"}},
		{name: "lefthook quoted run", kind: KindLefthook, src: "pre-push:\n  commands:\n    test:\n      run: \"go test ./... || true\"\n", want: []string{sw + "@4"}},
		{name: "redirect then swallow", kind: KindWorkflow, src: "        run: |\n          /tmp/verikt check --output json > out.json || true\n", want: []string{sw + "@2"}},
		{name: "two swallows one line", kind: KindShell, src: "go vet || true; go test || true\n", want: []string{sw + "@1", sw + "@1"}},

		// Not verification commands, or exempt cleanup.
		{name: "rm with pytest arg", kind: KindMakefile, src: "clean:\n\trm -rf .pytest_cache || true\n"},
		{name: "docker rm", kind: KindShell, src: "docker rm -f test-db || true\n"},
		{name: "kill", kind: KindShell, src: "kill $TEST_PID || true\n"},
		{name: "grep", kind: KindShell, src: "grep -v x file > out || true\n"},
		{name: "command -v capture", kind: KindShell, src: "p=$(command -v curl 2>/dev/null || true)\n"},
		{name: "test builtin", kind: KindShell, src: "test -f out.txt || true\n"},
		{name: "bracket test", kind: KindShell, src: "[ -d build ] || true\n"},
		{name: "underscore script file", kind: KindShell, src: "./run_tests.sh || true\n", want: []string{sw + "@1"}},
		// From the corpus (obra/superpowers scripts/bump-version.sh): an audit
		// that runs its own check function and goes on to report. A bare
		// snake_case word is a shell function, not a verification tool.
		{name: "shell function with check in its name", kind: KindShell, src: "cmd_check || true\n"},
		{name: "cleanup script", kind: KindShell, src: "./scripts/clean-test-db.sh || true\n"},
		{name: "make clean target", kind: KindShell, src: "make clean-test || true\n"},
		{name: "quoted operator", kind: KindShell, src: "echo \"go test || true\"\n"},
		{name: "comment", kind: KindShell, src: "# go test || true\n"},
		{name: "|| false is not a swallow", kind: KindShell, src: "go test || false\n"},
		{name: "|| exit 1 is not a swallow", kind: KindShell, src: "go test || exit 1\n"},
		{name: "|| truecolor word", kind: KindShell, src: "go test || trueish\n"},
		{name: "dockerfile not a target", kind: KindDockerfile, src: "RUN go test ./... || true\n"},
		{name: "markdown not a target", kind: KindMarkdown, src: "```sh\ngo test || true\n```\n"},
		{name: "allowed above", kind: KindShell, src: "# verikt:allow silent_ci_swallow reason=\"advisory during Q2 lint migration (#412)\"\ngolangci-lint run ./... || true\n"},
	})
}

func TestSilentCISwallow_ExtraCommands(t *testing.T) {
	opts := Options{ExtraCommands: []string{"db-up", "deno test"}}
	runScanCases(t, []scanCase{
		{name: "extra name", kind: KindShell, src: "db-up --wait || true\n", opts: opts, want: []string{sw + "@1"}},
		{name: "extra name as path", kind: KindShell, src: "./bin/db-up || true\n", opts: opts, want: []string{sw + "@1"}},
		{name: "extra prefix", kind: KindShell, src: "deno test -A || true\n", opts: opts, want: []string{sw + "@1"}},
		{name: "extra prefix other subcommand", kind: KindShell, src: "deno run x.ts || true\n", opts: opts},
		{name: "not configured", kind: KindShell, src: "db-up || true\n"},
	})
}
