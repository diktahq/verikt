package guide

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/rules"
)

// GenerateOptions holds the inputs for guide generation.
type GenerateOptions struct {
	ProjectDir        string
	Target            string
	Architecture      string
	Capabilities      []string
	Components        []config.Component
	TemplateFS        fs.FS // optional: embedded FS for pattern extraction
	CatalogOnly       bool  // when true, only output catalog-related sections
	Decisions         []config.Decision
	LanguageVersion   string                   // e.g., "Go 1.26"
	Features          map[string]bool          // detected language features; only true entries are shown
	GuideMode         string                   // "passive" | "audit" | "prompted" (default: "passive")
	SeverityOverrides config.SeverityOverrides // path-scoped severity overrides from verikt.yaml
}

// Generate produces guide files for the specified target(s).
func Generate(opts GenerateOptions) error {
	targets, err := resolveTargets(opts.Target)
	if err != nil {
		return err
	}

	// Monolithic content for non-Claude targets (built lazily).
	var monolithic string
	monolithicBuilt := false

	for _, t := range targets {
		if ct, ok := t.(*claudeTarget); ok && !opts.CatalogOnly {
			sc := buildSplitContent(opts)
			if err := ct.WriteSplit(opts.ProjectDir, sc); err != nil {
				return fmt.Errorf("write split guide: %w", err)
			}
		} else {
			if !monolithicBuilt {
				monolithic = buildContent(opts)
				monolithicBuilt = true
			}
			if err := t.Write(opts.ProjectDir, monolithic); err != nil {
				return fmt.Errorf("write guide for %s: %w", t.Name(), err)
			}
		}
	}
	return nil
}

// GenerateFromConfig reads an VeriktConfig and generates guides.
// An optional fs.FS can be passed to enable template pattern extraction.
func GenerateFromConfig(projectDir string, cfg *config.VeriktConfig, target string, templateFS ...fs.FS) error {
	opts := GenerateOptions{
		ProjectDir:        projectDir,
		Target:            target,
		Architecture:      cfg.Architecture,
		Capabilities:      cfg.Capabilities,
		Components:        cfg.Components,
		Decisions:         cfg.Decisions,
		GuideMode:         cfg.Guide.GuideMode(),
		SeverityOverrides: cfg.SeverityOverrides,
	}
	if len(templateFS) > 0 {
		opts.TemplateFS = templateFS[0]
	}
	return Generate(opts)
}

// BuildContent generates the full markdown guide content.
// Exported for use by integration experiments and external tooling.
func BuildContent(opts GenerateOptions) string {
	return buildContent(opts)
}

// buildContent generates the full markdown guide content.
func buildContent(opts GenerateOptions) string {
	var b strings.Builder

	writeHeader(&b)
	writeAgentInstructions(&b, opts.GuideMode)
	writeGovernanceCheckpoint(&b)

	if !opts.CatalogOnly {
		writeArchitecture(&b, opts.Architecture)
		writeLanguageVersion(&b, opts.LanguageVersion, opts.Features)
		writeLayerRules(&b, opts.Architecture, opts.Components)
		writeDependencyDirection(&b, opts.Architecture, opts.Components)
		writeAddingCode(&b, opts.Architecture)
		writeCapabilities(&b, opts.Capabilities)
		if patterns := ExtractPatterns(opts.TemplateFS, opts.Capabilities); patterns != "" {
			b.WriteString(patterns)
		}
	}

	if catalog, err := BuildCatalog(opts.TemplateFS, opts.Capabilities); err == nil && len(catalog) > 0 {
		writeCatalog(&b, catalog, opts.Capabilities)
	}

	// Always write the high-value design guidance sections.
	// These are what transform agent output from "list gaps" to "implement fixes."
	writeSmartSuggestions(&b)
	writeCriticalInteractionWarnings(&b)
	writeDesignQuestions(&b)

	// Write context-specific warnings and suggestions (based on installed caps).
	writeWarnings(&b, opts.Capabilities, opts.SeverityOverrides)
	writeSuggestions(&b, opts.Capabilities)

	if len(opts.Decisions) > 0 {
		writeDecisionStatus(&b, opts.Decisions)
	}

	if !opts.CatalogOnly {
		writeAntiPatterns(&b, opts.Architecture)
	}

	writeRuleSummaries(&b, opts.ProjectDir)
	writeVerificationChecklist(&b, opts.Capabilities)

	writeInterviewProtocol(&b)

	if opts.GuideMode == "prompted" {
		writeSuggestedPrompts(&b)
	}

	return b.String()
}

// writeVerificationChecklist writes a checklist the agent must confirm before
// finishing. Generic items always appear; capability-specific items appear when
// relevant capabilities are installed.
func writeVerificationChecklist(b *strings.Builder, caps []string) {
	b.WriteString("## Verification Checklist\n\n")
	b.WriteString("Before you finish, confirm each item. For any item you have NOT implemented, state why.\n\n")

	// Universal checks — always apply.
	b.WriteString("### Always\n\n")
	b.WriteString("- [ ] All external HTTP calls have a context deadline or timeout\n")
	b.WriteString("- [ ] Multi-tenant: tenant identity comes from verified auth, not a trusted header\n")
	b.WriteString("- [ ] Errors are mapped to specific HTTP status codes (not all-500)\n")
	b.WriteString("- [ ] No secrets or sensitive data in logs\n\n")

	// Capability-specific checks.
	capSet := make(map[string]bool, len(caps))
	for _, c := range caps {
		capSet[c] = true
	}

	var specific []string
	if capSet["http-client"] {
		specific = append(specific, "- [ ] **http-client** — circuit breaker implemented on the payment/external HTTP client")
	}
	if capSet["http-api"] || capSet["grpc"] {
		specific = append(specific, "- [ ] **transport** — /healthz and /readyz endpoints registered")
		specific = append(specific, "- [ ] **transport** — graceful shutdown handles SIGTERM/SIGINT")
		specific = append(specific, "- [ ] **transport** — request-id middleware propagates correlation ID")
	}
	if capSet["multi-tenancy"] {
		specific = append(specific, "- [ ] **multi-tenancy** — JWT/auth middleware validates tenant identity before trusting tenant context")
	}
	if capSet["event-bus"] {
		specific = append(specific, "- [ ] **event-bus** — outbox pattern used (or dual-write risk is explicitly accepted and documented)")
	}
	if capSet["mysql"] || capSet["postgres"] {
		specific = append(specific, "- [ ] **database** — UUIDv7 used for primary keys (not auto-increment or random UUID)")
		specific = append(specific, "- [ ] **database** — migration files included")
	}

	// If no verikt.yaml, emit the universal financial checklist.
	if len(caps) == 0 {
		specific = append(specific,
			"- [ ] **payments** — idempotency key required on all payment/order operations",
			"- [ ] **payments** — append-only audit log written on every financial mutation",
			"- [ ] **payments** — circuit breaker on the payment HTTP client",
		)
	}

	if len(specific) > 0 {
		b.WriteString("### Your Stack\n\n")
		for _, item := range specific {
			b.WriteString(item + "\n")
		}
		b.WriteString("\n")
	}
}

func writeHeader(b *strings.Builder) {
	b.WriteString("# verikt -- Architecture Guide\n\n")
	b.WriteString("> Auto-generated by `verikt guide`. Do not edit manually.\n")
	b.WriteString("> Re-run `verikt guide` to update.\n\n")
}

// writeAgentInstructions writes the usage preamble for the given guide mode.
// mode: "passive" | "audit" | "prompted" — defaults to "passive".
func writeAgentInstructions(b *strings.Builder, mode ...string) {
	m := "passive"
	if len(mode) > 0 && mode[0] != "" {
		m = mode[0]
	}

	b.WriteString("## How to Use This Guide\n\n")

	switch m {
	case "audit":
		b.WriteString("**On every session start:**\n\n")
		b.WriteString("1. **Read the codebase** — scan the project files to understand current state.\n")
		b.WriteString("2. **Cross-reference `verikt.yaml`** — compare what's declared against what's implemented.\n")
		b.WriteString("3. **Lead with a gap report** — before answering any question, output an `## Architecture Audit` section listing violations, missing capabilities, and drift.\n\n")
		b.WriteString("This guide reflects your *declared* architecture. Treat deviations as bugs to fix, not style choices.\n\n")
	case "prompted":
		b.WriteString("**Respond in this order:**\n\n")
		b.WriteString("1. **Research first** — search the web for current docs, APIs, and best practices. Read the codebase to understand what's already there. Answer the user's question fully.\n")
		b.WriteString("2. **Architecture Notes last** — after your full answer, append an `## Architecture Notes` section with any capability gaps, warnings, or boundary violations. Omit this section if nothing applies.\n\n")
		b.WriteString("This guide reflects your *declared* architecture (`verikt.yaml`). Always read the code first — use this guide to check alignment, not replace research.\n\n")
		b.WriteString("See **Suggested Prompts** at the bottom of this guide for queries that unlock the most value from verikt.\n\n")
	default: // passive
		b.WriteString("**Respond in this order:**\n\n")
		b.WriteString("1. **Research first** — search the web for current docs, APIs, and best practices. Read the codebase to understand what's already there. Answer the user's question fully.\n")
		b.WriteString("2. **Architecture Notes last** — after your full answer, append an `## Architecture Notes` section with any capability gaps, warnings, or boundary violations. Omit this section if nothing applies.\n\n")
		b.WriteString("This guide reflects your *declared* architecture (`verikt.yaml`). Always read the code first — use this guide to check alignment, not replace research.\n\n")
	}

	b.WriteString("This guide contains two types of content:\n\n")
	b.WriteString("- **🔴 MUST** — Implementation requirements. If a MUST applies to what you are building, implement it. The only valid reason to skip is if it genuinely does not apply — state that explicitly.\n")
	b.WriteString("- **🟡 SHOULD** — Production readiness recommendations. Implement if scope allows; otherwise flag as a known gap.\n\n")
	b.WriteString("**Before you finish:** run through the Verification Checklist at the bottom of this guide.\n\n")
}

// writeGovernanceCheckpoint writes a pre-action compliance checkpoint for the agent.
// It forces the agent to verify rule applicability before and after each tool call,
// preventing cascading violations from unverified corrections.
func writeGovernanceCheckpoint(b *strings.Builder) {
	b.WriteString("## Governance Checkpoint\n\n")
	b.WriteString("Before modifying any file, pause and verify:\n")
	b.WriteString("1. List which architecture rules from this guide apply to the change you are about to make.\n")
	b.WriteString("2. Check if the change introduces any pattern these rules explicitly prohibit.\n")
	b.WriteString("3. If multiple rules conflict, state the conflict before proceeding.\n\n")
	b.WriteString("After receiving tool results (test output, lint output, build errors), re-check compliance before taking the next action. Do not chain corrections without verifying each step against these rules.\n\n")
}

// InterviewProtocol returns the standalone AI interview protocol as markdown.
// Used by `verikt init --ai` to print the protocol to stdout so an AI agent
// can conduct the setup interview conversationally.
//
// The protocol matches the /verikt:init skill — both detect project state
// (greenfield vs brownfield) and route to the appropriate flow.
func InterviewProtocol() string {
	var b strings.Builder
	b.WriteString("# verikt — AI Setup Protocol\n\n")
	b.WriteString("> You are the onboarding wizard for verikt. Detect the project state and route to the right flow.\n\n")
	b.WriteString("**IMPORTANT:** Do NOT run `verikt init` — it opens a TUI that doesn't work in agent environments. You are replacing the TUI. Detect, interview, then run the appropriate command.\n\n")

	b.WriteString("## Step 1 — Detect Project State\n\n")
	b.WriteString("Check the current directory:\n")
	b.WriteString("- Run `ls` to see what files exist\n")
	b.WriteString("- Check for `verikt.yaml` (already initialized)\n")
	b.WriteString("- Check for `go.mod` or `package.json` (existing codebase)\n")
	b.WriteString("- Empty or only has README/LICENSE → greenfield\n\n")
	b.WriteString("Tell the user what you found, then follow the matching flow below.\n\n")

	b.WriteString("## Greenfield Flow (empty project)\n\n")
	b.WriteString("No code exists. Full scaffold interview.\n\n")
	writeInterviewProtocol(&b)

	b.WriteString("## Brownfield Flow (existing code)\n\n")
	b.WriteString("Existing code found. Analyze first, then ask what to do.\n\n")
	b.WriteString("### Analyze\n\n")
	b.WriteString("Run `verikt analyze --path . --output json` to detect language, architecture, framework, and libraries. Present the findings.\n\n")
	b.WriteString("### Choose Strategy\n\n")
	b.WriteString("Ask: \"What would you like to do?\"\n\n")
	b.WriteString("**Option A: Map existing architecture** — govern what's already here.\n")
	b.WriteString("- Confirm/adjust detected language and architecture\n")
	b.WriteString("- Select capabilities that match what's installed\n")
	b.WriteString("- Run: `verikt init --language <lang> --architecture <arch> --cap <caps> --guide-mode <mode> --no-wizard --force`\n")
	b.WriteString("- Then run: `verikt guide`\n\n")
	b.WriteString("**Option B: Bubble context** — start a clean new service inside this project.\n")
	b.WriteString("- Strangler fig pattern: new service gets proper architecture from day one\n")
	b.WriteString("- Ask for a service name, then follow the Greenfield Flow above\n")
	b.WriteString("- Run: `verikt new <name> --language <lang> --arch <arch> --cap <caps> --no-wizard`\n\n")

	b.WriteString("## After Setup\n\n")
	b.WriteString("Always:\n")
	b.WriteString("1. Run `verikt guide` to generate AI agent context files\n")
	b.WriteString("2. Tell the user what was created\n")
	b.WriteString("3. Show them: `verikt check` to validate, `verikt add <cap>` to add more\n\n")

	return b.String()
}

// writeInterviewProtocol appends an AI interview protocol section so agents can
// scaffold new services conversationally without the TUI wizard.
func writeInterviewProtocol(b *strings.Builder) {
	b.WriteString("## AI Interview: Scaffold a New Service\n\n")
	b.WriteString("If the user says they need a new service, want to scaffold something, or asks how to start — conduct this interview. Do NOT run the command until all questions are answered and the user has confirmed.\n\n")

	b.WriteString("### Step 1 — Service name\n\n")
	b.WriteString("Ask: \"What's the name of your service?\"\n\n")
	b.WriteString("Collect as `--name <value>`. Use lowercase-kebab-case.\n\n")

	b.WriteString("### Step 2 — Language\n\n")
	b.WriteString("Ask: \"Go or TypeScript?\"\n\n")
	b.WriteString("- **go** — Go module. Also ask for the module path (e.g. github.com/myorg/my-service). Collect as `--language go --module <value>`.\n")
	b.WriteString("- **typescript** — TypeScript/Node.js. Also ask for the HTTP framework: Express (default), Fastify, or Hono. Collect as `--language typescript`. If not Express, add `--set HttpFramework=<value>`.\n\n")

	b.WriteString("### Step 3 — Architecture\n\n")
	b.WriteString("Present the options available for the chosen language:\n\n")
	b.WriteString("**Go architectures:**\n")
	b.WriteString("- **hexagonal** — ports & adapters; business logic isolated from infrastructure. Best for complex domains, long-lived services.\n")
	b.WriteString("- **layered** — handler → service → repository → model. Familiar, predictable, lower ceremony.\n")
	b.WriteString("- **clean** — concentric rings (entity → usecase → interface → infrastructure). Good for large teams with strict layering.\n")
	b.WriteString("- **flat** — single package, no layer rules. Ideal for simple tools, CLIs, or prototypes.\n\n")
	b.WriteString("**TypeScript architectures:**\n")
	b.WriteString("- **hexagonal** — domain/ → application/ → infrastructure/ → transport/. Same boundaries as Go, TypeScript conventions.\n")
	b.WriteString("- **flat** — single src/ directory. Good for simple services or prototypes.\n\n")
	b.WriteString("Collect as `--arch <value>`.\n\n")

	b.WriteString("### Step 4 — Capabilities\n\n")
	b.WriteString("Ask: \"What does this service need?\" Guide the conversation:\n\n")
	b.WriteString("- Start broad: \"Will it expose an HTTP API? Connect to a database? Consume events?\"\n")
	b.WriteString("- As they answer, proactively suggest related capabilities:\n")
	b.WriteString("  - `http-api` → suggest `health`, `request-id`, `cors` (browser), `rate-limiting` (public)\n")
	b.WriteString("  - `postgres` or `mysql` → suggest `migrations`, `uuid`. For TypeScript, ask: \"Prisma (default) or Drizzle?\" If Drizzle, add `--set OrmLibrary=drizzle`.\n")
	b.WriteString("  - `http-client` → suggest `circuit-breaker`, `retry`, `timeout`\n")
	b.WriteString("  - `retry` → warn: \"also add `idempotency` — retrying without it causes duplicates\"\n")
	b.WriteString("  - `event-bus` → warn: \"also add `outbox` — events can be lost without it\"\n")
	b.WriteString("- Note: some capabilities are Go-only (grpc, graphql, ddd, templ, htmx). If the user chose TypeScript, do not suggest these.\n")
	b.WriteString("- Confirm final capability list before moving on.\n\n")
	b.WriteString("Collect as `--cap <comma-separated>`.\n\n")

	b.WriteString("### Step 5 — Guide mode\n\n")
	b.WriteString("Ask: \"How should I use the architecture guide in this project?\"\n\n")
	b.WriteString("- **passive** (default) — answer questions fully, architecture notes at the end. Low friction for day-to-day work.\n")
	b.WriteString("- **audit** — on every session start, read the codebase and lead with a gap report. Best for onboarding or pre-release reviews.\n")
	b.WriteString("- **prompted** — like passive, but with suggested prompts appended so the team knows what to ask.\n\n")
	b.WriteString("Collect as `--guide-mode <value>`.\n\n")

	b.WriteString("### Step 6 — Confirm and scaffold\n\n")
	b.WriteString("Show a summary of all choices. Ask: \"Ready to scaffold?\"\n\n")
	b.WriteString("On confirmation, run:\n\n")
	b.WriteString("```bash\n")
	b.WriteString("verikt new <name> \\\n")
	b.WriteString("  --language <language> \\\n")
	b.WriteString("  --arch <arch> \\\n")
	b.WriteString("  --cap <capabilities> \\\n")
	b.WriteString("  --guide-mode <mode> \\\n")
	b.WriteString("  --no-wizard\n")
	b.WriteString("```\n\n")
	b.WriteString("Add `--module <path>` for Go. Add `--set HttpFramework=<value>` or `--set OrmLibrary=drizzle` if non-default choices were made.\n\n")
	b.WriteString("After scaffolding, run `verikt guide` to generate updated context files, then tell the user what was created.\n\n")
}

// writeSuggestedPrompts appends copy-paste prompts that unlock the most value from verikt.
func writeSuggestedPrompts(b *strings.Builder) {
	b.WriteString("## Suggested Prompts\n\n")
	b.WriteString("Copy any of these into your AI agent to get the most from verikt:\n\n")
	b.WriteString("```\n")
	b.WriteString("Audit this codebase against verikt.yaml and list all violations\n")
	b.WriteString("What capabilities am I missing before going to production?\n")
	b.WriteString("I need to add [feature] — what capabilities do I need and how should I wire them?\n")
	b.WriteString("Research best practices for [capability] and implement it following my architecture\n")
	b.WriteString("What dangerous capability combinations do I have that need fixing?\n")
	b.WriteString("```\n\n")
}

func writeArchitecture(b *strings.Builder, arch string) {
	b.WriteString("## Architecture: " + arch + "\n\n")
	switch arch {
	case "hexagonal":
		b.WriteString("This project uses hexagonal (ports & adapters) architecture.\n")
		b.WriteString("Business logic lives in the center (domain + service layers),\n")
		b.WriteString("isolated from infrastructure by ports (interfaces) and adapters (implementations).\n\n")
	case "layered":
		b.WriteString("This project uses layered architecture.\n")
		b.WriteString("Code is organized into four layers: handler, service, repository, and model.\n")
		b.WriteString("Dependencies flow strictly downward: handler → service → repository → model.\n\n")
	case "clean":
		b.WriteString("This project uses Clean Architecture (Uncle Bob).\n")
		b.WriteString("Code is organized into four layers: entity, usecase, interface, and infrastructure.\n")
		b.WriteString("Dependencies point inward: infrastructure → interface → usecase → entity.\n")
		b.WriteString("The entity layer has no external dependencies — it is the innermost ring.\n\n")
	case "flat":
		b.WriteString("This project uses a flat architecture.\n")
		b.WriteString("All code lives in a single package with no layer restrictions.\n\n")
	default:
		b.WriteString("Architecture type: " + arch + "\n\n")
	}
}

func writeLayerRules(b *strings.Builder, arch string, components []config.Component) {
	b.WriteString("## Layer Rules\n\n")

	if arch == "flat" {
		b.WriteString("No layer restrictions. All code lives in the root package.\n\n")
		return
	}

	if len(components) == 0 {
		return
	}

	for _, c := range components {
		b.WriteString("### " + c.Name + "\n")
		b.WriteString("- Directories: " + strings.Join(c.In, ", ") + "\n")
		if len(c.MayDependOn) == 0 {
			b.WriteString("- Dependencies: none (innermost layer)\n")
		} else {
			b.WriteString("- May depend on: " + strings.Join(c.MayDependOn, ", ") + "\n")
		}
		b.WriteString("\n")
	}

	writeCodebaseMapping(b, components)
}

// writeCodebaseMapping emits a directory-to-layer mapping table derived from component globs.
// This is static (from verikt.yaml), not dynamic (no filesystem scan). See ADR-008.
func writeCodebaseMapping(b *strings.Builder, components []config.Component) {
	if len(components) == 0 {
		return
	}

	b.WriteString("## Codebase Mapping\n\n")
	b.WriteString("When reading or writing code, use this table to know which layer a directory belongs to:\n\n")
	b.WriteString("| Directory | Layer | Role |\n")
	b.WriteString("|-----------|-------|------|\n")

	for _, c := range components {
		for _, glob := range c.In {
			dir := globToDir(glob)
			role := layerRole(c.Name, c.MayDependOn)
			fmt.Fprintf(b, "| `%s` | %s | %s |\n", dir, c.Name, role)
		}
	}

	b.WriteString("\nDirectories not listed here are outside the declared architecture. Run `verikt check` to identify unmapped directories.\n\n")
}

// globToDir converts a component glob to a human-readable directory path.
// "domain/**" → "domain/", "adapter/http/**" → "adapter/http/"
func globToDir(glob string) string {
	dir := strings.TrimSuffix(glob, "/**")
	dir = strings.TrimSuffix(dir, "**")
	if !strings.HasSuffix(dir, "/") && dir != "" {
		dir += "/"
	}
	if dir == "" {
		dir = "(root)"
	}
	return dir
}

// layerRole returns a short description of a component's role based on its position.
func layerRole(name string, deps []string) string {
	if len(deps) == 0 {
		return "Innermost layer — no outward dependencies"
	}
	switch name {
	case "domain":
		return "Business entities and value objects"
	case "ports", "port":
		return "Interfaces defining boundaries"
	case "service":
		return "Application logic and use cases"
	case "adapter", "adapters":
		return "Infrastructure implementations"
	case "handler":
		return "HTTP/transport handlers"
	case "repository":
		return "Data access implementations"
	case "model":
		return "Shared data structures"
	case "internal":
		return "Private implementation details"
	case "entity":
		return "Enterprise business rules"
	case "usecase":
		return "Application business rules"
	case "interface":
		return "Interface adapters"
	case "infrastructure":
		return "Frameworks and drivers"
	default:
		return "Depends on: " + strings.Join(deps, ", ")
	}
}

func writeDependencyDirection(b *strings.Builder, arch string, components []config.Component) {
	b.WriteString("## Dependency Direction\n\n")

	if arch == "flat" {
		b.WriteString("No dependency restrictions in flat architecture.\n\n")
		return
	}

	b.WriteString("Dependencies point **inward** toward the domain.\n\n")
	b.WriteString("**NEVER rules:**\n\n")

	for _, c := range components {
		forbidden := forbiddenDeps(c, components)
		for _, f := range forbidden {
			fmt.Fprintf(b, "- `%s` NEVER imports from `%s`\n", c.Name, f)
		}
	}
	b.WriteString("\n")
}

// forbiddenDeps returns component names that c must NOT depend on.
func forbiddenDeps(c config.Component, all []config.Component) []string {
	allowed := map[string]bool{c.Name: true}
	for _, dep := range c.MayDependOn {
		allowed[dep] = true
	}

	var forbidden []string
	for _, other := range all {
		if !allowed[other.Name] {
			forbidden = append(forbidden, other.Name)
		}
	}
	return forbidden
}

func writeAddingCode(b *strings.Builder, arch string) {
	b.WriteString("## Adding Code\n\n")

	if arch == "flat" {
		b.WriteString("Add new files to the root package. No special placement rules.\n\n")
		return
	}

	if arch == "clean" {
		b.WriteString("### New entity (enterprise business rule)\n")
		b.WriteString("1. Add to `internal/entity/`\n")
		b.WriteString("2. Entity MUST NOT import from usecase, interface, or infrastructure\n\n")

		b.WriteString("### New use case (application business rule)\n")
		b.WriteString("1. Add to `internal/usecase/`\n")
		b.WriteString("2. Use case may only import from `internal/entity/`\n")
		b.WriteString("3. Use case MUST NOT import from infrastructure\n\n")

		b.WriteString("### New interface adapter (handler, presenter, gateway)\n")
		b.WriteString("1. Add to `internal/interface/`\n")
		b.WriteString("2. Interface adapters may import from `internal/usecase/` and `internal/entity/`\n\n")

		b.WriteString("### New HTTP endpoint\n")
		b.WriteString("1. Add handler function in `internal/interface/handler/`\n")
		b.WriteString("2. Register route in `internal/interface/handler/router.go`\n")
		b.WriteString("3. Handler calls a use case; NEVER calls entity or infrastructure directly\n\n")

		b.WriteString("### New infrastructure component (DB, web, config)\n")
		b.WriteString("1. Add to `internal/infrastructure/`\n")
		b.WriteString("2. Infrastructure may import from all inner layers\n\n")
		return
	}

	if arch == "layered" {
		b.WriteString("### New model (shared entity)\n")
		b.WriteString("1. Add to `internal/model/`\n")
		b.WriteString("2. Model MUST NOT import from handler, service, or repository\n\n")

		b.WriteString("### New repository\n")
		b.WriteString("1. Add to `internal/repository/`\n")
		b.WriteString("2. Repository may only import from `internal/model/`\n\n")

		b.WriteString("### New service\n")
		b.WriteString("1. Add to `internal/service/`\n")
		b.WriteString("2. Service may import from `internal/repository/` and `internal/model/`\n\n")

		b.WriteString("### New HTTP endpoint\n")
		b.WriteString("1. Add handler function in `internal/handler/`\n")
		b.WriteString("2. Register route in `internal/handler/router.go`\n")
		b.WriteString("3. Handler calls service; NEVER calls repository directly\n\n")
		return
	}

	b.WriteString("### New domain entity\n")
	b.WriteString("1. Create the entity in `domain/`\n")
	b.WriteString("2. Define value objects and domain errors alongside it\n")
	b.WriteString("3. Domain MUST NOT import from any other layer\n\n")

	b.WriteString("### New port (interface)\n")
	b.WriteString("1. Define the interface in `port/`\n")
	b.WriteString("2. Ports may only import from `domain/`\n\n")

	b.WriteString("### New adapter (implementation)\n")
	b.WriteString("1. Create in `adapter/<type>/` (e.g., `adapter/httphandler/`, `adapter/mysqlrepo/`)\n")
	b.WriteString("2. Implement a port interface\n")
	b.WriteString("3. Adapters may import from `port/` and `domain/` only\n\n")

	b.WriteString("### New service / use case\n")
	b.WriteString("1. Add to `service/`\n")
	b.WriteString("2. Depend on ports (interfaces), not adapters (implementations)\n")
	b.WriteString("3. Services may import from `domain/` and `port/`\n\n")

	b.WriteString("### New HTTP endpoint\n")
	b.WriteString("1. Add handler function in `adapter/httphandler/`\n")
	b.WriteString("2. Register route in the router\n")
	b.WriteString("3. Handler calls a service via its port interface\n\n")
}

func writeCapabilities(b *strings.Builder, capabilities []string) {
	b.WriteString("## Capabilities\n\n")

	if len(capabilities) == 0 {
		b.WriteString("No capabilities configured.\n\n")
		return
	}

	b.WriteString("Installed capabilities:\n\n")
	for _, cap := range capabilities {
		dir := capabilityDir(cap)
		fmt.Fprintf(b, "- **%s** -- %s\n", cap, dir)
	}
	b.WriteString("\n")
}

// featureDescriptions maps feature keys to human-readable descriptions with minimum version.
var featureDescriptions = map[string]struct {
	minVersion  string
	description string
}{
	"slices_package":       {"1.21+", "use slices.SortFunc, slices.Contains instead of sort.Slice"},
	"log_slog":             {"1.21+", "use log/slog for structured logging"},
	"maps_package":         {"1.21+", "use maps.Keys, maps.Values from stdlib"},
	"range_over_int":       {"1.22+", "use `for i := range n` syntax"},
	"range_over_func":      {"1.23+", "use iterator functions with range"},
	"os_root":              {"1.24+", "use os.OpenRoot for kernel-level path safety"},
	"weak_pointers":        {"1.24+", "use weak package for weak references"},
	"os_root_fs":           {"1.25+", "os.Root implements fs.FS interface"},
	"synctest":             {"1.24+", "use testing/synctest for concurrent test control"},
	"generic_type_aliases": {"1.24+", "fully generic type aliases supported"},
}

// featureOrder defines the display order for features.
var featureOrder = []string{
	"slices_package",
	"log_slog",
	"maps_package",
	"range_over_int",
	"range_over_func",
	"os_root",
	"weak_pointers",
	"os_root_fs",
	"synctest",
	"generic_type_aliases",
}

func writeLanguageVersion(b *strings.Builder, version string, features map[string]bool) {
	if version == "" || len(features) == 0 {
		return
	}

	// Check if any feature is active.
	hasActive := false
	for _, active := range features {
		if active {
			hasActive = true
			break
		}
	}
	if !hasActive {
		return
	}

	b.WriteString("## Language Version\n\n")
	fmt.Fprintf(b, "%s detected. Available modern APIs:\n", version)

	for _, key := range featureOrder {
		if !features[key] {
			continue
		}
		desc, ok := featureDescriptions[key]
		if !ok {
			continue
		}
		fmt.Fprintf(b, "- %s (%s) — %s\n", key, desc.minVersion, desc.description)
	}

	b.WriteString("\nWhen writing code, prefer these modern APIs over legacy alternatives.\n\n")
}

// capabilityDir returns the typical directory for a capability.
func capabilityDir(cap string) string {
	dirs := map[string]string{
		"http-api":      "adapter/httphandler/",
		"grpc":          "adapter/grpchandler/, proto/",
		"graphql":       "adapter/graphql/",
		"sse":           "adapter/httphandler/",
		"mysql":         "adapter/mysqlrepo/",
		"postgres":      "adapter/postgresrepo/",
		"redis":         "adapter/redisrepo/",
		"mongodb":       "adapter/mongorepo/",
		"sqlite":        "adapter/sqliterepo/",
		"kafka":         "adapter/kafkahandler/",
		"s3":            "adapter/s3client/",
		"dynamodb":      "adapter/dynamorepo/",
		"observability": "platform/observability/",
		"config":        "config/",
		"docker":        "Dockerfile, docker-compose.yml",
		"ci-github":     ".github/workflows/",
		"makefile":      "Makefile",
		"saga":          "service/saga/",
		"feature-flags": "platform/featureflags/",
		"multi-tenancy": "adapter/httphandler/middleware/",
		"ci-gitlab":     ".gitlab-ci.yml",
		"devcontainer":  ".devcontainer/",
		"templ":         "adapter/httphandler/views/",
		"htmx":          "adapter/httphandler/",
		"static-assets": "static/",
	}
	if d, ok := dirs[cap]; ok {
		return d
	}
	return "see project structure"
}

func writeRuleSummaries(b *strings.Builder, projectDir string) {
	if projectDir == "" {
		return
	}

	rulesDir := filepath.Join(projectDir, ".verikt", "rules")
	if _, err := os.Stat(rulesDir); errors.Is(err, fs.ErrNotExist) {
		return
	}

	loadedRules, statuses, err := rules.LoadRules(rulesDir, projectDir)
	if err != nil {
		return
	}

	// Filter to valid rules only.
	var validRules []rules.Rule
	statusMap := make(map[string]string, len(statuses))
	for _, s := range statuses {
		statusMap[s.Rule.ID] = s.Status
	}
	for _, r := range loadedRules {
		if statusMap[r.ID] == "valid" {
			validRules = append(validRules, r)
		}
	}

	if len(validRules) == 0 {
		return
	}

	fmt.Fprintf(b, "## Active Rules\n\n")
	fmt.Fprintf(b, "%d proxy rules enforced by `verikt check`:\n\n", len(validRules))
	b.WriteString("| Rule | Engine | Severity | Scope |\n")
	b.WriteString("|------|--------|----------|-------|\n")
	for _, r := range validRules {
		scope := strings.Join(r.Scope, ", ")
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", r.ID, r.Engine, r.Severity, scope)
	}
	b.WriteString("\nRun `verikt check` to validate. Run `verikt check --staged` as pre-commit hook.\n\n")
}

func writeAntiPatterns(b *strings.Builder, arch string) {
	b.WriteString("## Anti-patterns to Avoid\n\n")

	// Common anti-patterns: each rule has a positive alternative.
	b.WriteString("- NEVER create `utils/`, `helpers/`, `common/`, or `shared/` packages — place functions in the package they serve\n")
	b.WriteString("- NEVER use init() for business logic — use explicit constructors (`NewService(deps)`) from `main()`\n")
	b.WriteString("- NEVER ignore errors with `_` — handle, wrap, or return: `return fmt.Errorf(\"op: %w\", err)`\n")
	b.WriteString("- NEVER use `uuid.New()` for entity IDs (B-tree fragmentation) — use UUIDv7: `uuid.Must(uuid.NewV7())`\n")
	b.WriteString("- NEVER start a naked goroutine — use `errgroup.Go()` or `context.Context` + `sync.WaitGroup` for lifecycle control\n")
	b.WriteString("- NEVER use `context.Background()` in handlers — propagate `r.Context()` through the call chain\n")
	b.WriteString("- NEVER concatenate SQL strings (injection risk) — use parameterized queries: `db.QueryContext(ctx, \"SELECT ... WHERE id = $1\", id)`\n")
	b.WriteString("- NEVER use package-level mutable `var` (data races) — inject state through struct constructors\n")

	if arch == "hexagonal" {
		b.WriteString("- NEVER import infrastructure from `domain/` — define interfaces in `port/`, implement in `adapter/`\n")
		b.WriteString("- NEVER put business logic in handlers or adapters — implement in `service/`, handlers only parse/respond\n")
		b.WriteString("- NEVER depend on concrete types — accept port interfaces: `NewOrderService(repo port.OrderRepository)`\n")
		b.WriteString("- NEVER put SQL/JSON tags in domain types — create separate DTOs in `adapter/`\n")
		b.WriteString("- NEVER bypass the service layer — route: `handler → service → port → adapter`\n")
	}

	if arch == "clean" {
		b.WriteString("- NEVER let `entity/` import from usecase, interface, or infrastructure — define interfaces in `entity/`\n")
		b.WriteString("- NEVER let `usecase/` import from infrastructure — accept repository interfaces as constructor params\n")
		b.WriteString("- NEVER put business logic in `interface/` adapters — parse request, call use case, format response\n")
		b.WriteString("- NEVER bypass use cases — route: `adapter → usecase → entity`\n")
		b.WriteString("- NEVER let SQL/HTTP concerns leak into entities or use cases — map at the adapter boundary\n")
	}

	if arch == "layered" {
		b.WriteString("- NEVER put business logic in `handler/` — parse request, call service, format response\n")
		b.WriteString("- NEVER import `handler/` or `service/` from `repository/` — dependencies point downward only\n")
		b.WriteString("- NEVER let handler call repository directly — mediate through the service layer\n")
		b.WriteString("- NEVER put data access in `service/` — inject a repository interface, implement in `repository/`\n")
	}

	b.WriteString("\n")
}
