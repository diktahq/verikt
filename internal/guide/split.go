package guide

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/scaffold"
)

// SplitContent holds the split guide content for Claude target.
type SplitContent struct {
	Index      string            // verikt-index.md content
	Categories map[string]string // category name -> content (e.g., "http" -> content)
}

// categoryCapabilities maps split categories to capability names.
var categoryCapabilities = map[string][]string{
	"http":       {"http-api", "grpc", "graphql", "websocket", "sse", "kafka-consumer", "nats", "api-versioning", "http-client", "bff"},
	"data":       {"mysql", "postgres", "redis", "mongodb", "sqlite", "dynamodb", "s3", "elasticsearch", "repository", "migrations"},
	"security":   {"auth-jwt", "cors", "multi-tenancy", "audit-log", "feature-flags", "oauth2", "encryption"},
	"resilience": {"circuit-breaker", "retry", "rate-limiting", "idempotency", "health", "timeout", "bulkhead"},
	"patterns":   {"cqrs", "ddd", "event-bus", "outbox", "saga", "scheduler", "worker", "email-gateway", "i18n"},
	"quality":    {"testing", "linting", "pre-commit", "validation", "uuid"},
	"frontend":   {"templ", "htmx", "static-assets"},
}

// categoryGlobs maps split categories to the file globs in each category
// file's `paths:` frontmatter.
var categoryGlobs = map[string][]string{
	// The bff gateway entries mirror each architecture's adapter/bffgateway
	// path mapping (hexagonal, clean, flat; layered's internal/handler/bff is
	// under internal/handler/**). split_bff_glob_test.go keeps them in sync.
	"http": {"internal/handler/**", "adapter/httphandler/**", "adapter/grpchandler/**", "adapter/graphql/**",
		"adapter/bffgateway/**", "internal/interface/bff/**", "handler/bff/**", "adapter/natshandler/**", "proto/**"},
	"data":       {"internal/repository/**", "adapter/*repo/**", "adapter/s3client/**", "adapter/esrepo/**", "migrations/**"},
	"security":   {"internal/middleware/**", "internal/auth/**", "adapter/httphandler/middleware/**"},
	"resilience": {"platform/resilience/**", "internal/circuit/**"},
	"patterns":   {"domain/**", "internal/event/**", "service/saga/**"},
	"quality":    {"**/*_test.go", "testdata/**"},
	// static-assets embeds static/ from the HTTP handler package, so the
	// static entries follow each architecture's adapter/httphandler mapping.
	"frontend": {"adapter/httphandler/views/**", "internal/views/**",
		"adapter/httphandler/static/**", "internal/interface/handler/static/**",
		"internal/handler/static/**", "handler/static/**"},
}

// categoryTitles maps split categories to human-readable titles.
var categoryTitles = map[string]string{
	"http":       "HTTP & Transport",
	"data":       "Data & Persistence",
	"security":   "Security & Auth",
	"resilience": "Resilience",
	"patterns":   "Patterns & Events",
	"quality":    "Quality & Testing",
	"frontend":   "Frontend & Views",
}

// buildSplitContent generates split guide content for Claude target.
func buildSplitContent(opts GenerateOptions) SplitContent {
	sc := SplitContent{
		Index:      buildIndexContent(opts),
		Categories: make(map[string]string),
	}

	for category := range categoryCapabilities {
		installed := installedInCategory(category, opts.Capabilities)
		if len(installed) == 0 {
			continue
		}
		sc.Categories[category] = buildCategoryContent(category, installed, opts)
	}

	return sc
}

// buildIndexContent generates the always-loaded index file: the writer rules
// shared by every target. It has no frontmatter, because Claude Code loads a
// rule without `paths:` frontmatter at launch, which is what the index needs.
func buildIndexContent(opts GenerateOptions) string {
	return claudeHeader + buildWriterRules(opts)
}

// writeArchitectureSummary writes a 2-3 sentence architecture summary.
func writeArchitectureSummary(b *strings.Builder, arch string) {
	switch arch {
	case "hexagonal":
		b.WriteString("Hexagonal (ports & adapters). Business logic in domain + service layers, isolated by ports (interfaces) and adapters (implementations).\n\n")
	case "layered":
		b.WriteString("Layered architecture. Dependencies flow strictly downward: handler → service → repository → model.\n\n")
	case "clean":
		b.WriteString("Clean Architecture. Dependencies point inward: infrastructure → interface → usecase → entity.\n\n")
	case "flat":
		b.WriteString("Flat architecture. All code in a single package, no layer restrictions.\n\n")
	default:
		fmt.Fprintf(b, "Architecture type: %s.\n\n", arch)
	}
}

// writeCriticalWarnings writes only critical warnings.
func writeCriticalWarnings(b *strings.Builder, capabilities []string) {
	warnings := scaffold.CapabilityWarnings(capabilities)
	var critical []string
	for _, w := range warnings {
		if w.Critical {
			critical = append(critical, w.Message)
		}
	}

	b.WriteString("## Critical Warnings\n\n")
	if len(critical) == 0 {
		b.WriteString("None.\n\n")
		return
	}
	for _, w := range critical {
		fmt.Fprintf(b, "- CRITICAL: %s\n", w)
	}
	b.WriteString("\n")
}

// buildCategoryContent generates a category-specific file.
func buildCategoryContent(category string, installed []string, opts GenerateOptions) string {
	var b strings.Builder

	// Claude Code scopes a rule with a `paths:` YAML list and loads it only
	// when the agent works on a matching file. It ignores Cursor's `globs:`.
	b.WriteString("---\npaths:\n")
	for _, g := range globsForCategory(category) {
		fmt.Fprintf(&b, "  - %q\n", g)
	}
	b.WriteString("---\n")
	b.WriteString(claudeHeader)

	title := categoryTitles[category]
	if title == "" {
		title = category
	}
	fmt.Fprintf(&b, "# verikt — %s\n\n", title)

	// Installed table.
	b.WriteString("## Installed\n\n")
	b.WriteString("| Capability | Directory |\n")
	b.WriteString("|-----------|----------|\n")
	for _, cap := range installed {
		fmt.Fprintf(&b, "| %s | %s |\n", cap, capabilityDir(cap))
	}
	b.WriteString("\n")

	// Adding code section relevant to this category.
	writeCategoryAddingCode(&b, category, opts.Architecture)

	// Category-relevant suggestions.
	writeCategorySuggestions(&b, category, installed, opts.Capabilities)

	// Category-relevant warnings.
	writeCategoryWarnings(&b, category, installed, opts.Capabilities, opts.SeverityOverrides)

	return b.String()
}

// writeCategoryAddingCode writes adding-code guidance relevant to the category.
func writeCategoryAddingCode(b *strings.Builder, category, arch string) {
	b.WriteString("## Adding Code\n\n")

	switch category {
	case "http":
		writeHTTPAddingCode(b, arch)
	case "data":
		writeDataAddingCode(b, arch)
	case "security":
		b.WriteString("- Add middleware in the middleware/auth package\n")
		b.WriteString("- Security checks run before handler logic\n")
		b.WriteString("- NEVER put auth logic in domain or business layer\n\n")
	case "resilience":
		b.WriteString("- Add resilience wrappers in the platform/resilience package\n")
		b.WriteString("- Apply via adapter constructors, not in domain logic\n\n")
	case "patterns":
		b.WriteString("- Domain events live in the domain layer\n")
		b.WriteString("- Event handlers are adapters that implement port interfaces\n")
		b.WriteString("- NEVER put event publishing logic in HTTP handlers\n\n")
	case "quality":
		b.WriteString("- Test files live next to the code they test\n")
		b.WriteString("- Use table-driven tests for multiple input/output combinations\n")
		b.WriteString("- Test helpers go in testdata/ or test utility packages\n\n")
	case "frontend":
		b.WriteString("- Templates live in adapter/httphandler/views/\n")
		b.WriteString("- Static assets in static/ next to the HTTP handler, embedded with embed.FS\n")
		b.WriteString("- NEVER put business logic in templates\n\n")
	}
}

func writeHTTPAddingCode(b *strings.Builder, arch string) {
	switch arch {
	case "hexagonal":
		b.WriteString("1. Add handler function in `adapter/httphandler/`\n")
		b.WriteString("2. Register route in the router\n")
		b.WriteString("3. Handler calls a service via its port interface\n\n")
	case "layered":
		b.WriteString("1. Add handler function in `internal/handler/`\n")
		b.WriteString("2. Register route in `internal/handler/router.go`\n")
		b.WriteString("3. Handler calls service; NEVER calls repository directly\n\n")
	case "clean":
		b.WriteString("1. Add handler function in `internal/interface/handler/`\n")
		b.WriteString("2. Register route in `internal/interface/handler/router.go`\n")
		b.WriteString("3. Handler calls a use case; NEVER calls entity or infrastructure directly\n\n")
	default:
		b.WriteString("Add HTTP handlers following your project's conventions.\n\n")
	}
}

func writeDataAddingCode(b *strings.Builder, arch string) {
	switch arch {
	case "hexagonal":
		b.WriteString("1. Define repository interface (port) in `port/`\n")
		b.WriteString("2. Create adapter in `adapter/<type>repo/`\n")
		b.WriteString("3. Adapter implements the port interface\n\n")
	case "layered":
		b.WriteString("1. Add to `internal/repository/`\n")
		b.WriteString("2. Repository may only import from `internal/model/`\n\n")
	case "clean":
		b.WriteString("1. Define gateway interface in `internal/usecase/`\n")
		b.WriteString("2. Implement in `internal/infrastructure/`\n\n")
	default:
		b.WriteString("Add data access code following your project's conventions.\n\n")
	}
}

// writeCategorySuggestions writes suggestions relevant to this category.
func writeCategorySuggestions(b *strings.Builder, category string, _, allInstalled []string) {
	suggestions := scaffold.ComputeSuggestions(allInstalled)
	catCaps := make(map[string]bool, len(categoryCapabilities[category]))
	for _, c := range categoryCapabilities[category] {
		catCaps[c] = true
	}

	var relevant []scaffold.Suggestion
	for _, s := range suggestions {
		if catCaps[s.Capability] {
			relevant = append(relevant, s)
		}
	}

	if len(relevant) == 0 {
		return
	}

	b.WriteString("## Suggestions\n\n")
	for i, s := range relevant {
		if i >= 3 {
			break
		}
		fmt.Fprintf(b, "- **Add %s** — %s\n", s.Capability, s.Reason)
	}
	b.WriteString("\n")
}

// writeCategoryWarnings writes warnings relevant to this category's capabilities.
// Warning messages start with the triggering capability name (e.g., "http-api without ..."),
// so we match by checking if the message starts with any capability in this category.
func writeCategoryWarnings(b *strings.Builder, category string, _ []string, allInstalled []string, overrides config.SeverityOverrides) {
	warnings := scaffold.CapabilityWarnings(allInstalled)
	if len(warnings) == 0 {
		return
	}

	catCaps := categoryCapabilities[category]

	var relevant []scaffold.CapabilityWarning
	for _, w := range warnings {
		for _, cap := range catCaps {
			if strings.HasPrefix(w.Message, cap+" ") || strings.Contains(w.Message, "**"+cap) {
				relevant = append(relevant, w)
				break
			}
		}
	}

	if len(relevant) == 0 {
		return
	}

	b.WriteString("## Warnings\n\n")
	for _, w := range relevant {
		if w.Critical {
			fmt.Fprintf(b, "- 🔴 MUST: %s\n", w.Message)
		} else {
			fmt.Fprintf(b, "- 🟡 SHOULD: %s\n", w.Message)
		}
		writeOverrideLines(b, overrides, w.Key)
	}
	b.WriteString("\n")
}

// installedInCategory returns capabilities from installed list that belong to category.
func installedInCategory(category string, installed []string) []string {
	caps, ok := categoryCapabilities[category]
	if !ok {
		return nil
	}

	catSet := make(map[string]bool, len(caps))
	for _, c := range caps {
		catSet[c] = true
	}

	var result []string
	for _, c := range installed {
		if catSet[c] {
			result = append(result, c)
		}
	}
	return result
}

// globsForCategory returns the globs for a category's `paths:` frontmatter.
func globsForCategory(category string) []string {
	if g, ok := categoryGlobs[category]; ok {
		return g
	}
	return []string{"**/*"}
}

// WriteSplit writes split guide files for the Claude target.
func (t *claudeTarget) WriteSplit(projectDir string, sc SplitContent) error {
	files := map[string]string{claudeIndexRule: sc.Index}
	for category, content := range sc.Categories {
		files["verikt-"+category+".md"] = content
	}
	if err := writeClaudeRules(projectDir, files); err != nil {
		return err
	}

	// Generate Claude Code hook scripts and update settings.json.
	if err := generateHooks(projectDir); err != nil {
		return fmt.Errorf("generate hooks: %w", err)
	}

	// Write guide hash for SessionStart staleness detection.
	if err := writeGuideHash(projectDir); err != nil {
		return fmt.Errorf("write guide hash: %w", err)
	}

	return nil
}

const (
	claudeIndexRule      = "verikt-index.md"
	claudeMonolithicRule = "verikt.md"
)

var claudeRulesDir = filepath.Join(".claude", "rules")

// writeClaudeRules writes files (name -> content) to .claude/rules, then
// removes every rule file an earlier verikt run generated that this run did
// not write: a category the project dropped, verikt.md after leaving
// --catalog-only, or the split files after entering it. Claude Code loads
// every file in .claude/rules, so a stale one keeps steering the agent.
func writeClaudeRules(projectDir string, files map[string]string) error {
	for name, content := range files {
		if err := WriteProjectFile(projectDir, filepath.Join(claudeRulesDir, name), []byte(content)); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return removeStaleClaudeRules(projectDir, files)
}

// removeStaleClaudeRules removes a .claude/rules file only when verikt owns
// both its name (verikt.md or verikt-*.md) and its content (the generated
// header in its first lines). A user's file is never removed.
func removeStaleClaudeRules(projectDir string, keep map[string]string) error {
	names, err := listProjectDir(projectDir, claudeRulesDir)
	if err != nil {
		return err
	}
	for _, name := range names {
		if _, ok := keep[name]; ok || !isVeriktRuleName(name) {
			continue
		}
		rel := filepath.Join(claudeRulesDir, name)
		data, err := readProjectFile(projectDir, rel)
		if err != nil {
			return fmt.Errorf("read %s: %w", filepath.ToSlash(rel), err)
		}
		if !hasGeneratedHeader(string(data)) {
			continue
		}
		if err := removeProjectFile(projectDir, rel); err != nil {
			return err
		}
	}
	return nil
}

func isVeriktRuleName(name string) bool {
	return name == claudeMonolithicRule || (strings.HasPrefix(name, "verikt-") && strings.HasSuffix(name, ".md"))
}

// hasGeneratedHeader reports whether content carries claudeHeader at its start
// or right after its frontmatter, where verikt writes it.
func hasGeneratedHeader(content string) bool {
	if strings.HasPrefix(content, claudeHeader) {
		return true
	}
	if !strings.HasPrefix(content, "---\n") {
		return false
	}
	end := strings.Index(content[4:], "\n---\n")
	return end >= 0 && strings.HasPrefix(content[4+end+5:], claudeHeader)
}
