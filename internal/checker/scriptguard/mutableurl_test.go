package scriptguard

import "testing"

const mu = RuleMutableInstallURL

func TestMutableInstallURL_PipeToShell(t *testing.T) {
	runScanCases(t, []scanCase{
		{name: "curl bash", kind: KindShell, src: "curl -fsSL https://x.dev/install.sh | bash\n", want: []string{mu + "@1"}},
		{name: "env var before bash", kind: KindShell, src: "curl -fsSL https://verikt.dev/install.sh | VERIKT_VERSION=v0.1.1 bash\n", want: []string{mu + "@1"}},
		{name: "sudo sh", kind: KindShell, src: "wget -qO- https://x.dev/i | sudo -E sh -s -- -y\n", want: []string{mu + "@1"}},
		{name: "env and path", kind: KindShell, src: "curl https://x.dev/i | /usr/bin/env bash\n", want: []string{mu + "@1"}},
		{name: "zsh", kind: KindShell, src: "curl https://x.dev/i|zsh\n", want: []string{mu + "@1"}},
		{name: "process substitution", kind: KindShell, src: "bash <(curl -fsSL https://x.dev/i)\n", want: []string{mu + "@1"}},
		{name: "sh -c substitution", kind: KindShell, src: "sh -c \"$(curl -fsSL https://x.dev/i)\"\n", want: []string{mu + "@1"}},
		{name: "eval substitution", kind: KindShell, src: "eval \"$(curl -fsSL https://x.dev/i)\"\n", want: []string{mu + "@1"}},
		{name: "continuation", kind: KindDockerfile, src: "FROM alpine:3.20\nRUN curl -sSfL https://x.dev/install.sh \\\n    | sh -s -- -b /usr/local/bin\n", want: []string{mu + "@2"}},
		{name: "workflow", kind: KindWorkflow, src: "      - run: curl -sSL https://x.dev/i | bash\n", want: []string{mu + "@1"}},
		{name: "workflow quoted run", kind: KindWorkflow, src: "      - run: \"curl -sSL https://x.dev/i | bash\"\n", want: []string{mu + "@1"}},
		{name: "makefile", kind: KindMakefile, src: "tools:\n\tcurl -sSL https://x.dev/i | sh\n", want: []string{mu + "@2"}},
		{name: "lefthook", kind: KindLefthook, src: "run: curl https://x.dev/i | sh\n", want: []string{mu + "@1"}},
		{name: "ci config", kind: KindCIConfig, src: "  - curl https://x.dev/i | sh\n", want: []string{mu + "@1"}},

		{name: "checksum pipe", kind: KindShell, src: "curl -fsSL https://github.com/o/r/releases/download/v0.1.1/a.tgz | shasum -a 256\n"},
		{name: "sha256sum", kind: KindShell, src: "curl -fsSL https://x/a.tgz | sha256sum\n"},
		{name: "tar", kind: KindShell, src: "curl -fsSL https://x/a.tgz | tar xz\n"},
		{name: "shellcheck is not sh", kind: KindShell, src: "curl -fsSL https://x/a.sh | shellcheck -\n"},
		{name: "separate commands", kind: KindShell, src: "curl -o i.sh https://x/i.sh; cat i.sh | sh\n"},
		{name: "comment", kind: KindShell, src: "#   curl -fsSL https://verikt.dev/install.sh | bash\n"},
		{name: "quoted in echo", kind: KindShell, src: "echo \"install with: curl -fsSL https://x.dev/i | bash\"\n"},
		{name: "single-quoted in printf", kind: KindShell, src: "printf '%s\\n' 'curl https://x.dev/i | sh'\n"},
		{name: "allowed", kind: KindShell, src: "# verikt:allow mutable_install_url reason=\"vendor-documented installer, run in a throwaway container\"\ncurl https://x.dev/i | sh\n"},
	})
}

func TestMutableInstallURL_MutableRefs(t *testing.T) {
	runScanCases(t, []scanCase{
		{name: "latest download", kind: KindShell, src: "curl -LO https://github.com/o/r/releases/latest/download/r.tgz\n", want: []string{mu + "@1"}},
		{name: "raw main", kind: KindShell, src: "curl -O https://raw.githubusercontent.com/o/r/main/setup.sh\n", want: []string{mu + "@1"}},
		{name: "raw refs/heads/master", kind: KindShell, src: "wget https://raw.githubusercontent.com/o/r/refs/heads/master/x.sh\n", want: []string{mu + "@1"}},
		{name: "raw HEAD", kind: KindShell, src: "wget https://raw.githubusercontent.com/o/r/HEAD/x.sh\n", want: []string{mu + "@1"}},
		{name: "raw and pipe on one line report once", kind: KindCIConfig, src: "  - curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b bin\n", want: []string{mu + "@1"}},
		{name: "raw tag", kind: KindShell, src: "curl -O https://raw.githubusercontent.com/o/r/v1.2.3/setup.sh\n"},
		{name: "raw sha", kind: KindShell, src: "curl -O https://raw.githubusercontent.com/o/r/0123456789abcdef0123456789abcdef01234567/setup.sh\n"},
		{name: "raw image", kind: KindShell, src: "curl -O https://raw.githubusercontent.com/o/r/main/docs/logo.svg\n"},
		{name: "branch named mainline", kind: KindShell, src: "curl -O https://raw.githubusercontent.com/o/r/mainline/x.sh\n"},
		{name: "pinned release", kind: KindShell, src: "curl -LO https://github.com/o/r/releases/download/v1.0.0/r.tgz\n"},
		{name: "latest api is not a download", kind: KindShell, src: "curl -fsSL https://api.github.com/repos/o/r/releases/latest\n"},
	})
}

func TestMutableInstallURL_WorkflowUses(t *testing.T) {
	runScanCases(t, []scanCase{
		{name: "main", kind: KindWorkflow, src: "      - uses: org/deploy-action@main\n", want: []string{mu + "@1"}},
		{name: "master quoted", kind: KindWorkflow, src: "        uses: 'org/a/sub@master'\n", want: []string{mu + "@1"}},
		{name: "HEAD with comment", kind: KindWorkflow, src: "    uses: org/a@HEAD # tracking\n", want: []string{mu + "@1"}},
		{name: "reusable workflow", kind: KindWorkflow, src: "    uses: org/r/.github/workflows/ci.yml@main\n", want: []string{mu + "@1"}},
		{name: "version tag", kind: KindWorkflow, src: "      - uses: actions/checkout@v4\n"},
		{name: "sha", kind: KindWorkflow, src: "      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6.0.2\n"},
		{name: "local action", kind: KindWorkflow, src: "      - uses: ./.github/actions/setup\n"},
		{name: "branch named main-v2", kind: KindWorkflow, src: "      - uses: org/a@main-v2\n"},
		{name: "uses only in workflows", kind: KindLefthook, src: "uses: org/a@main\n"},
	})
}

func TestMutableInstallURL_DockerFrom(t *testing.T) {
	runScanCases(t, []scanCase{
		{name: "latest", kind: KindDockerfile, src: "FROM --platform=$BUILDPLATFORM alpine:latest\n", want: []string{mu + "@1"}},
		{name: "untagged", kind: KindDockerfile, src: "FROM ubuntu\n", want: []string{mu + "@1"}},
		{name: "registry port untagged", kind: KindDockerfile, src: "FROM localhost:5000/service\n", want: []string{mu + "@1"}},
		{name: "lowercase from", kind: KindDockerfile, src: "from node AS build\n", want: []string{mu + "@1"}},
		{name: "arg latest", kind: KindDockerfile, src: "FROM ${BASE}:latest\n", want: []string{mu + "@1"}},
		{name: "unknown stage name", kind: KindDockerfile, src: "FROM builder\n", want: []string{mu + "@1"}},

		{name: "multi-stage", kind: KindDockerfile, src: "FROM golang:1.23-alpine AS builder\nRUN go build\nFROM builder\nFROM scratch\n"},
		{name: "stage case-insensitive", kind: KindDockerfile, src: "FROM golang:1.23 AS Build\nFROM build\n"},
		{name: "registry port tagged", kind: KindDockerfile, src: "FROM localhost:5000/service:1.2\n"},
		{name: "digest", kind: KindDockerfile, src: "FROM alpine@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n"},
		{name: "latest pinned by digest", kind: KindDockerfile, src: "FROM alpine:latest@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n"},
		{name: "arg image", kind: KindDockerfile, src: "ARG BASE=alpine:3.20\nFROM $BASE\n"},
		{name: "template tag", kind: KindDockerfile, src: "FROM golang:{{.GoVersion}}-alpine AS builder\n"},
		{name: "commented", kind: KindDockerfile, src: "# FROM ubuntu\n"},
		{name: "from only in dockerfiles", kind: KindShell, src: "FROM ubuntu\n"},
	})
}

func TestMutableInstallURL_Markdown(t *testing.T) {
	src := "# Install\n\n" + // 1-2
		"Run curl https://x.dev/i | bash in prose.\n\n" + // 3-4: prose is ignored
		"```sh\n" + // 5
		"curl -fsSL https://x.dev/i | bash\n" + // 6
		"curl -LO https://github.com/o/r/releases/latest/download/r.tgz\n" + // 7
		"curl -O https://raw.githubusercontent.com/o/r/main/x.sh\n" + // 8: raw rule skipped in docs
		"```\n\n" + // 9-10
		"<!-- verikt:allow mutable_install_url reason=\"convenience script; pinned archive documented below\" -->\n" + // 11
		"```bash\n" + // 12
		"curl -fsSL https://x.dev/i | bash\n" + // 13: allowed via marker above the fence
		"```\n" + // 14
		"# verikt:allow is a heading here, not a marker\n" // 15
	vs := Scan("README.md", KindMarkdown, []byte(src), Options{})
	lines := make([]int, 0, len(vs))
	for _, v := range vs {
		if v.Rule != mu {
			t.Fatalf("unexpected rule %+v", v)
		}
		lines = append(lines, v.Line)
	}
	if len(lines) != 2 || lines[0] != 6 || lines[1] != 7 {
		t.Fatalf("markdown findings at lines %v, want [6 7]", lines)
	}
}
