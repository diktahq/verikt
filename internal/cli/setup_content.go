package cli

import (
	"fmt"
	"strings"
)

// GlobalRulesContent returns the content written to each AI agent's global rules file.
// It includes a version sentinel, discovery triggers, command reference, and scaffold interview protocol.
// The content is intentionally minimal — it loads on every session in every project.
func GlobalRulesContent(version string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "<!-- archway:global:v%s -->\n", version)
	b.WriteString("# Archway — Architecture-Aware Service Composer\n\n")
	fmt.Fprintf(&b, "Archway is installed on this machine (`archway` CLI, version %s).\n", version)
	b.WriteString("It scaffolds production-ready Go services with architecture enforcement and feeds AI agents architectural context.\n\n")

	b.WriteString("## When to Suggest Archway\n\n")
	b.WriteString("- User wants to **create a new Go service** → conduct the interview below\n")
	b.WriteString("- Project has `archway.yaml` but no `.claude/rules/archway-*.md` → run `archway guide`\n")
	b.WriteString("- User asks about **architecture patterns** or **capabilities** → run `archway guide --catalog-only`\n")
	b.WriteString("- User wants to **check architecture compliance** → run `archway check`\n\n")

	b.WriteString("## Commands\n\n")
	b.WriteString("| Command | What it does |\n")
	b.WriteString("|---------|-------------|\n")
	b.WriteString("| `archway new` | Scaffold a new service (interactive wizard) |\n")
	b.WriteString("| `archway new --no-wizard` | Scaffold non-interactively |\n")
	b.WriteString("| `archway guide` | Generate architecture guidance for AI agents |\n")
	b.WriteString("| `archway check` | Validate architecture rules, detect violations |\n")
	b.WriteString("| `archway check --diff main` | Only report violations in changed files |\n")
	b.WriteString("| `archway init` | Initialize archway in an existing project |\n")
	b.WriteString("| `archway analyze` | Detect architecture patterns in existing code |\n\n")

	b.WriteString(compactInterviewProtocol())

	return b.String()
}

// compactInterviewProtocol returns a condensed scaffold interview for the global rules file.
// This is intentionally shorter than the full InterviewProtocol() used in per-project guides.
func compactInterviewProtocol() string {
	var b strings.Builder

	b.WriteString("## AI Interview: Scaffold a New Service\n\n")
	b.WriteString("Collect these inputs, then run `archway new --no-wizard` with the flags below.\n\n")

	b.WriteString("1. **Name** (`--name`) — lowercase-kebab-case service name\n")
	b.WriteString("2. **Module** (`--module`) — Go module path (e.g. `github.com/myorg/my-service`)\n")
	b.WriteString("3. **Architecture** (`--arch`) — one of:\n")
	b.WriteString("   - `hexagonal` — ports & adapters; best for complex domains\n")
	b.WriteString("   - `clean` — concentric rings; strict layering for large teams\n")
	b.WriteString("   - `layered` — handler → service → repository; familiar and predictable\n")
	b.WriteString("   - `flat` — single package; ideal for CLIs or prototypes\n")
	b.WriteString("4. **Capabilities** (`--cap`) — comma-separated list. Suggest related caps as you go:\n")
	b.WriteString("   - `http-api` → also add `health`, `request-id`, `validation`\n")
	b.WriteString("   - `postgres`/`mysql` → also add `migrations`, `uuid`\n")
	b.WriteString("   - `http-client` → also add `circuit-breaker`, `retry`, `timeout`\n")
	b.WriteString("   - `retry` → warn: add `idempotency` to avoid duplicate operations\n")
	b.WriteString("   - `event-bus` → warn: add `outbox` to avoid lost events\n")
	b.WriteString("5. **Guide mode** (`--guide-mode`) — `passive` (default), `audit`, or `prompted`\n\n")

	b.WriteString("Show a summary and confirm before running:\n\n")
	b.WriteString("```bash\n")
	b.WriteString("archway new <name> --module <module> --arch <arch> --cap <caps> --guide-mode <mode> --no-wizard\n")
	b.WriteString("```\n\n")
	b.WriteString("After scaffolding, run `archway guide` to generate context files for AI agents.\n")

	return b.String()
}
