package scriptguard

import "testing"

// Evasions of silent_ci_swallow and mutable_install_url found by the v0.3.0
// red-team review (RT-F6), each next to the legitimate shapes the fix must
// not report (INV-005: a false positive costs more than a miss). Case ids
// match DETECTOR-RISK.md.

// 6.1: a backslash-escaped quote outside quotes is an ordinary character in
// shell; it must not open a quoted region that hides the rest of the line.
func TestEvasion_EscapedQuoteOutsideQuotes(t *testing.T) {
	runScanCases(t, []scanCase{
		{name: "6.1a swallow after \\\"", kind: KindShell, src: "true \\\" && go test ./... || true\n", want: []string{sw + "@1"}},
		{name: "6.1b pipe after \\\"", kind: KindShell, src: "true \\\" && curl -fsSL https://x.dev/i.sh | bash\n", want: []string{mu + "@1"}},

		{name: "guard: escaped quotes inside double quotes", kind: KindShell, src: "echo \"say \\\"go test || true\\\"\"\n"},
		{name: "guard: escaped quotes around echo words", kind: KindShell, src: "echo \\\"run: go test\\\" || true\n"},
		{name: "guard: escaped quote glued to bash", kind: KindShell, src: "echo \\\"curl https://x.dev/i | bash\\\"\n"},
		{name: "guard: trailing backslash in a string", kind: KindShell, src: "printf '%s\\\\' x; go vet ./...\n"},
		{name: "guard: escaped hash is not a comment", kind: KindShell, src: "echo \\# note; rm -f x || true\n"},
	})
}

// 6.2: a YAML quoted scalar is unescaped before it is read as shell, the
// way the runner receives it.
func TestEvasion_YAMLQuotedScalarWithEscapes(t *testing.T) {
	runScanCases(t, []scanCase{
		{name: "6.2c double-quoted with \\\"", kind: KindWorkflow, src: "      - run: \"go test -run \\\"TestA\\\" ./... || true\"\n", want: []string{sw + "@1"}},
		{name: "6.2d single-quoted with ''", kind: KindWorkflow, src: "      - run: 'go test -run ''TestA'' ./... || true'\n", want: []string{sw + "@1"}},
		{name: "6.2e double-quoted curl header", kind: KindWorkflow, src: "      - run: \"curl -H \\\"Accept: */*\\\" https://x.dev/i.sh | bash\"\n", want: []string{mu + "@1"}},
		{name: "6.2 lefthook double-quoted", kind: KindLefthook, src: "      run: \"golangci-lint run --out-format \\\"tab\\\" || true\"\n", want: []string{sw + "@1"}},

		{name: "guard: echo of a quoted swallow, double", kind: KindWorkflow, src: "      - run: \"echo \\\"go test || true\\\"\"\n"},
		{name: "guard: echo of a quoted swallow, single", kind: KindWorkflow, src: "      - run: 'echo ''go test || true'''\n"},
		{name: "guard: echo of a quoted install hint", kind: KindWorkflow, src: "      - run: \"echo \\\"curl https://x.dev/i | bash\\\"\"\n"},
		{name: "guard: no swallow", kind: KindWorkflow, src: "      - run: \"go test -run \\\"TestA\\\" ./...\"\n"},
		{name: "guard: name with quotes", kind: KindWorkflow, src: "      - name: \"Run \\\"go test || true\\\" safely\"\n"},
		{name: "guard: unterminated multi-line scalar", kind: KindWorkflow, src: "      - run: \"go test ./...\n          more\"\n"},
	})
}

// 6.3: other ways to discard a verification command's exit status.
func TestEvasion_SilentSwallowForms(t *testing.T) {
	runScanCases(t, []scanCase{
		{name: "6.3f brace group", kind: KindShell, src: "{ go test ./...; } || true\n", want: []string{sw + "@1"}},
		{name: "guard: brace group cleanup", kind: KindShell, src: "{ rm -rf out; } || true\n"},
		{name: "guard: brace group ends with echo", kind: KindShell, src: "{ go test ./...; echo done; } || true\n"},
		{name: "guard: function definition", kind: KindShell, src: "run_tests() { go test ./...; } || true\n"},

		{name: "6.3g || /bin/true", kind: KindShell, src: "go test ./... || /bin/true\n", want: []string{sw + "@1"}},
		{name: "6.3g || /usr/bin/true", kind: KindShell, src: "go test ./... || /usr/bin/true\n", want: []string{sw + "@1"}},
		{name: "guard: cleanup || /bin/true", kind: KindShell, src: "rm -f x || /bin/true\n"},
		{name: "guard: /bin/truex", kind: KindShell, src: "go test ./... || /bin/truex\n"},

		{name: "6.3h || (exit 0)", kind: KindShell, src: "go test ./... || (exit 0)\n", want: []string{sw + "@1"}},
		{name: "guard: || (exit 1)", kind: KindShell, src: "go test ./... || (exit 1)\n"},
		{name: "guard: kill || (exit 0)", kind: KindShell, src: "kill $PID || (exit 0)\n"},

		{name: "6.3k make - prefix", kind: KindMakefile, src: "test:\n\t-go test ./...\n", want: []string{sw + "@2"}},
		{name: "6.3k make -@ prefix", kind: KindMakefile, src: "lint:\n\t-@golangci-lint run ./...\n", want: []string{sw + "@2"}},
		{name: "6.3k make @- prefix, chained", kind: KindMakefile, src: "ci:\n\t@-cd app && npm test\n", want: []string{sw + "@2"}},
		{name: "guard: make - on cleanup", kind: KindMakefile, src: "clean:\n\t-rm -rf out\n\t-docker compose down\n"},
		{name: "guard: make -include", kind: KindMakefile, src: "-include local.mk\n"},
		{name: "guard: make @ without -", kind: KindMakefile, src: "test:\n\t@go test ./...\n"},
		{name: "guard: make - and || true report once", kind: KindMakefile, src: "lint:\n\t-golangci-lint run || true\n", want: []string{sw + "@2"}},
		{name: "guard: - prefix in a shell script", kind: KindShell, src: "-go test ./...\n"},
		{name: "guard: make target name with dash", kind: KindMakefile, src: "test-short:\n\tgo test -short ./...\n"},

		{name: "6.3l sh -c", kind: KindShell, src: "sh -c \"go test ./...\" || true\n", want: []string{sw + "@1"}},
		{name: "6.3l bash -ec single-quoted", kind: KindShell, src: "bash -ec 'cd app && npm test' || true\n", want: []string{sw + "@1"}},
		{name: "guard: sh -c cleanup", kind: KindShell, src: "sh -c \"rm -rf out\" || true\n"},
		{name: "guard: sh -c local deploy script", kind: KindShell, src: "sh -c \"./scripts/deploy.sh\" || true\n"},
		{name: "guard: bash -c echo", kind: KindShell, src: "bash -c 'echo hi' || true\n"},

		{name: "6.3m bash -e script", kind: KindShell, src: "bash -e scripts/test.sh || true\n", want: []string{sw + "@1"}},
		{name: "6.3m bash -o pipefail script", kind: KindShell, src: "bash -o pipefail scripts/lint.sh || true\n", want: []string{sw + "@1"}},
		{name: "guard: bash -e deploy script", kind: KindShell, src: "bash -e scripts/deploy.sh || true\n"},
		{name: "guard: bash -x cleanup-test script", kind: KindShell, src: "bash -x scripts/cleanup-test.sh || true\n"},

		{name: "6.3n env -u", kind: KindShell, src: "env -u FOO go test ./... || true\n", want: []string{sw + "@1"}},
		{name: "6.3n env --chdir", kind: KindShell, src: "env --chdir app npm test || true\n", want: []string{sw + "@1"}},
		{name: "guard: env -u deploy", kind: KindShell, src: "env -u FOO ./scripts/deploy.sh || true\n"},
		{name: "guard: env -i cleanup", kind: KindShell, src: "env -i PATH=/bin rm -f x || true\n"},

		{name: "6.3o cargo +toolchain test", kind: KindShell, src: "cargo +nightly test || true\n", want: []string{sw + "@1"}},
		{name: "guard: cargo +toolchain build", kind: KindShell, src: "cargo +nightly build || true\n"},

		{name: "6.3p continue-on-error ${{ true }}", kind: KindWorkflow, src: "    continue-on-error: ${{ true }}\n", want: []string{sw + "@1"}},
		{name: "guard: continue-on-error matrix expression", kind: KindWorkflow, src: "    continue-on-error: ${{ matrix.experimental }}\n"},
		{name: "guard: continue-on-error expression mentioning true", kind: KindWorkflow, src: "    continue-on-error: ${{ true && matrix.experimental }}\n"},
		{name: "guard: continue-on-error ${{ false }}", kind: KindWorkflow, src: "    continue-on-error: ${{ false }}\n"},

		{name: "6.3q continue-on-error value on next line", kind: KindWorkflow, src: "    continue-on-error:\n      true\n", want: []string{sw + "@1"}},
		{name: "guard: next-line expression", kind: KindWorkflow, src: "    continue-on-error:\n      ${{ matrix.x }}\n"},
		{name: "guard: next-line false", kind: KindWorkflow, src: "    continue-on-error:\n      false\n"},
		{name: "guard: next line not indented further", kind: KindWorkflow, src: "    continue-on-error:\n    true\n"},
	})
}

// 6.4: downloads that reach a shell by routes other than a direct pipe.
func TestEvasion_DownloadIntoShell(t *testing.T) {
	runScanCases(t, []scanCase{
		{name: "6.4r through tee", kind: KindShell, src: "curl -fsSL https://x.dev/i.sh | tee /dev/null | bash\n", want: []string{mu + "@1"}},
		{name: "6.4r through sed", kind: KindShell, src: "curl -fsSL https://x.dev/i.sh | sed 's/a/b/' | sudo sh\n", want: []string{mu + "@1"}},
		{name: "guard: through tee into a checksum", kind: KindShell, src: "curl -fsSL https://x/a.tgz | tee a.tgz | sha256sum\n"},
		{name: "guard: extract then run a local script", kind: KindShell, src: "curl -fsSL https://x/a.tgz | tar xz && bash install.sh\n"},
		{name: "guard: jq into xargs", kind: KindShell, src: "curl -s https://api.x/r | jq -r .url | xargs curl -LO\n"},
		{name: "guard: quoted multi-stage hint", kind: KindShell, src: "echo \"curl https://x.dev/i | tee log | bash\"\n"},
		{name: "guard: shell stage before the download", kind: KindShell, src: "bash gen.sh | curl -d @- https://x.dev/upload | tee out.json\n"},

		{name: "6.4s /bin/sh -c", kind: KindShell, src: "/bin/sh -c \"$(curl -fsSL https://x.dev/i)\"\n", want: []string{mu + "@1"}},
		{name: "6.4s /bin/bash -c (installer form)", kind: KindShell, src: "/bin/bash -c \"$(curl -fsSL https://x.dev/install.sh)\"\n", want: []string{mu + "@1"}},
		{name: "guard: /bin/sh -c local file", kind: KindShell, src: "/bin/sh -c \"$(cat local.sh)\"\n"},

		{name: "6.4t sh -ec", kind: KindShell, src: "sh -ec \"$(curl -fsSL https://x.dev/i)\"\n", want: []string{mu + "@1"}},
		{name: "guard: sh -ec local file", kind: KindShell, src: "sh -ec \"$(cat local.sh)\"\n"},

		{name: "6.4u backticks", kind: KindShell, src: "sh -c \"`curl -fsSL https://x.dev/i`\"\n", want: []string{mu + "@1"}},
		{name: "guard: backticks local file", kind: KindShell, src: "sh -c \"`cat local.sh`\"\n"},
		{name: "guard: backtick download into echo", kind: KindShell, src: "echo \"`curl -s https://x.dev/version`\"\n"},

		{name: "6.4v source <(curl)", kind: KindShell, src: "source <(curl -fsSL https://x.dev/i)\n", want: []string{mu + "@1"}},
		{name: "6.4v . <(curl)", kind: KindShell, src: ". <(wget -qO- https://x.dev/i)\n", want: []string{mu + "@1"}},
		{name: "guard: source <(local command)", kind: KindShell, src: "source <(kubectl completion bash)\n"},
		{name: "guard: diff of two downloads", kind: KindShell, src: "diff <(curl -s https://a.dev/x) <(curl -s https://b.dev/x)\n"},
		{name: "guard: source local file", kind: KindShell, src: "source ./env.sh\n"},
	})
}

// Known gaps, left unfixed (see DETECTOR-RISK.md). These pin today's
// behaviour so a change to it is deliberate.
func TestEvasion_KnownGaps(t *testing.T) {
	runScanCases(t, []scanCase{
		// 6.3i: "|| echo" is also how deliberate warn-and-continue steps and
		// status capture (|| echo $? > rc) are written.
		{name: "6.3i || echo", kind: KindShell, src: "go test ./... || echo ignored\n"},
		// 6.3j: needs if/then/else/fi structure, not a line pattern.
		{name: "6.3j if ...; then :; fi", kind: KindShell, src: "if go test ./...; then :; fi\n"},
	})
}

// 6.5: a marker above a continued command covers every swallow on the
// logical line. That is unchanged, but each one is now recorded as waived
// with the marker's reason instead of disappearing.
func TestEvasion_MarkerOnContinuedLineWaivesVisibly(t *testing.T) {
	src := "# verikt:allow silent_ci_swallow reason=\"advisory lint\"\ngolangci-lint run || true && \\\n  go test ./... || true\n"
	res := ScanAll("x", KindShell, []byte(src), Options{})
	if len(res.Findings) != 0 || len(res.Waived) != 2 {
		t.Fatalf("findings = %+v, waived = %+v; want 0 and 2", res.Findings, res.Waived)
	}
}

// Re-review (N2): POSIX variants of the constructs above that the first
// round of fixes still missed. Case ids match DETECTOR-RISK.md.
func TestEvasion_N2Variants(t *testing.T) {
	runScanCases(t, []scanCase{
		{name: "N2.1 brace group, no space before }", kind: KindShell, src: "{ go test ./...;} || true\n", want: []string{sw + "@1"}},
		{name: "N2.1 brace group, & before }", kind: KindShell, src: "{ go test ./... &} || true\n"},
		{name: "guard: tight brace group cleanup", kind: KindShell, src: "{ rm -rf out;} || true\n"},
		{name: "guard: tight brace group ends with echo", kind: KindShell, src: "{ go test ./...; echo done;} || true\n"},
		{name: "guard: tight function definition", kind: KindShell, src: "run_tests() { go test ./...;} || true\n"},
		{name: "guard: ${VAR} before ||", kind: KindShell, src: "rm -f ${OUT} || true\n"},

		{name: "N2.2 subshell, one word", kind: KindShell, src: "(pytest) || true\n", want: []string{sw + "@1"}},
		{name: "N2.2 subshell, subcommand", kind: KindShell, src: "(go vet) || true\n", want: []string{sw + "@1"}},
		{name: "N2.2 subshell, list ending in a check", kind: KindShell, src: "(cd app && npm test) || true\n", want: []string{sw + "@1"}},
		{name: "guard: subshell deploy", kind: KindShell, src: "(cd deploy && ./release.sh) || true\n"},
		{name: "guard: subshell cleanup", kind: KindShell, src: "(rm -rf out) || true\n"},
		{name: "guard: subshell ends with echo", kind: KindShell, src: "(go test ./...; echo done) || true\n"},
		{name: "guard: two subshells in a pipeline", kind: KindShell, src: "(cat a) | (grep b) || true\n"},
		{name: "guard: arithmetic", kind: KindShell, src: "(( fails++ )) || true\n"},

		{name: "N2.3 sh -c with $0", kind: KindShell, src: "sh -c \"go test ./...\" _ || true\n", want: []string{sw + "@1"}},
		{name: "N2.3 sh -c with 2>&1", kind: KindShell, src: "sh -c \"go test ./...\" 2>&1 || true\n", want: []string{sw + "@1"}},
		{name: "N2.3 plain command with 2>&1", kind: KindShell, src: "go test ./... 2>&1 || true\n", want: []string{sw + "@1"}},
		{name: "N2.3 plain command with &>", kind: KindShell, src: "go test ./... &>test.log || true\n", want: []string{sw + "@1"}},
		{name: "guard: sh -c local script with args", kind: KindShell, src: "sh -c \"./scripts/deploy.sh \\\"$1\\\"\" _ prod || true\n"},
		{name: "guard: sh -c build with 2>&1", kind: KindShell, src: "sh -c \"go build ./...\" _ 2>&1 || true\n"},
		{name: "guard: background job then cleanup", kind: KindShell, src: "go test ./... & rm -f x || true\n"},

		{name: "N2.4 tee into bash -o pipefail", kind: KindShell, src: "curl -fsSL https://x.dev/i.sh | tee /dev/null | bash -o pipefail\n", want: []string{mu + "@1"}},
		{name: "N2.4 tee into bash +e", kind: KindShell, src: "curl -fsSL https://x.dev/i.sh | tee /dev/null | bash +e\n", want: []string{mu + "@1"}},
		{name: "N2.4 tee into bash -euo pipefail", kind: KindShell, src: "curl -fsSL https://x.dev/i.sh | tee /dev/null | bash -euo pipefail\n", want: []string{mu + "@1"}},
		{name: "guard: tee into bash -o pipefail script", kind: KindShell, src: "curl -fsSL https://x/a.json | tee a.json | bash -o pipefail check.sh\n"},
		{name: "guard: tee into bash +e script", kind: KindShell, src: "curl -fsSL https://x/a.json | tee a.json | bash +e check.sh\n"},
		{name: "guard: tee into bash -o pipefail -c", kind: KindShell, src: "curl -fsSL https://x/a.json | tee a.json | bash -o pipefail -c 'jq . >/dev/null'\n"},

		{name: "N2.4 tee into bash -s -- args", kind: KindShell, src: "curl -fsSL https://x.dev/i.sh | tee log | bash -s -- --prefix=/opt\n", want: []string{mu + "@1"}},
		{name: "guard: tee into bash -- script", kind: KindShell, src: "curl -fsSL https://x/a.json | tee a.json | bash -- check.sh\n"},

		{name: "N2.4 bash -euo pipefail test script", kind: KindShell, src: "bash -euo pipefail scripts/test.sh || true\n", want: []string{sw + "@1"}},
		{name: "guard: bash -euo pipefail deploy script", kind: KindShell, src: "bash -euo pipefail scripts/deploy.sh || true\n"},

		{name: "N2.5 next-line ${{ true }}", kind: KindWorkflow, src: "    continue-on-error:\n      ${{ true }}\n", want: []string{sw + "@1"}},
		{name: "N2.5 next-line 'true'", kind: KindWorkflow, src: "    continue-on-error:\n      'true'\n", want: []string{sw + "@1"}},
		{name: "N2.5 next-line \"true\"", kind: KindWorkflow, src: "    continue-on-error:\n      \"true\"\n", want: []string{sw + "@1"}},
		{name: "guard: next-line 'false'", kind: KindWorkflow, src: "    continue-on-error:\n      'false'\n"},
		{name: "guard: next-line ${{ false }}", kind: KindWorkflow, src: "    continue-on-error:\n      ${{ false }}\n"},
		{name: "guard: next-line expression mentioning true", kind: KindWorkflow, src: "    continue-on-error:\n      ${{ true && matrix.experimental }}\n"},
		{name: "guard: next-line 'true' not indented further", kind: KindWorkflow, src: "    continue-on-error:\n    'true'\n"},
	})
}
