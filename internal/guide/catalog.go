package guide

import (
	"cmp"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/dcsg/archway/internal/scaffold"
)

// CatalogEntry describes a single capability in the catalog.
type CatalogEntry struct {
	Name        string
	Category    string
	Description string
	WhenToUse   string
	Installed   bool
	Suggests    []string
}

// capabilityCategories maps each capability to its category.
var capabilityCategories = map[string]string{
	// Transport
	"http-api":       "transport",
	"grpc":           "transport",
	"graphql":        "transport",
	"websocket":      "transport",
	"kafka-consumer": "transport",
	"nats":           "transport",
	"sse":            "transport",
	"bff":            "transport",
	// Data
	"mysql":         "data",
	"postgres":      "data",
	"redis":         "data",
	"mongodb":       "data",
	"sqlite":        "data",
	"dynamodb":      "data",
	"s3":            "data",
	"elasticsearch": "data",
	"repository":    "data",
	"migrations":    "data",
	// Resilience
	"circuit-breaker": "resilience",
	"retry":           "resilience",
	"rate-limiting":   "resilience",
	"idempotency":     "resilience",
	"timeout":         "resilience",
	"bulkhead":        "resilience",
	// Patterns
	"event-bus": "patterns",
	"cqrs":      "patterns",
	"ddd":       "patterns",
	"outbox":    "patterns",
	"saga":      "patterns",
	"worker":    "patterns",
	"scheduler": "patterns",
	// Security
	"auth-jwt":      "security",
	"cors":          "security",
	"audit-log":     "security",
	"multi-tenancy": "security",
	"feature-flags": "security",
	"oauth2":        "security",
	"encryption":    "security",
	// Observability
	"observability": "observability",
	"health":        "observability",
	"request-id":    "observability",
	// Infrastructure
	"docker":       "infrastructure",
	"ci-bitbucket": "infrastructure",
	"ci-github":    "infrastructure",
	"ci-gitlab":    "infrastructure",
	"graceful":     "infrastructure",
	"makefile":     "infrastructure",
	"devcontainer": "infrastructure",
	"pre-commit":   "infrastructure",
	// Quality
	"linting":    "quality",
	"testing":    "quality",
	"validation": "quality",
	"uuid":       "quality",
	// Platform
	"platform":  "platform",
	"bootstrap": "platform",
	// Frontend
	"templ":         "frontend",
	"htmx":          "frontend",
	"static-assets": "frontend",
	// Other
	"http-client":    "transport",
	"email-gateway":  "patterns",
	"mailpit":        "infrastructure",
	"i18n":           "patterns",
	"api-versioning": "transport",
}

// whenToUse provides guidance on when each capability is appropriate.
var whenToUse = map[string]string{
	"http-api":        "REST APIs, webhooks, web applications",
	"grpc":            "Internal microservice communication, streaming, high-performance RPC",
	"graphql":         "Client-facing APIs with complex querying needs, mobile apps",
	"websocket":       "Real-time bidirectional communication (chat, live updates)",
	"kafka-consumer":  "Event-driven architectures, async message processing",
	"nats":            "Lightweight pub/sub, request-reply, JetStream persistent messaging",
	"sse":             "Server-to-client streaming (live feeds, notifications)",
	"bff":             "Backend-for-Frontend gateway (service aggregation, response shaping)",
	"mysql":           "Relational data with SQL queries, transactional workloads",
	"postgres":        "Advanced relational data, JSONB, full-text search",
	"redis":           "Caching, session storage, pub/sub, rate limiting backend",
	"mongodb":         "Document-oriented data, flexible schemas, rapid prototyping",
	"sqlite":          "Embedded databases, single-binary deployments, testing",
	"dynamodb":        "Serverless workloads, key-value at scale, AWS-native apps",
	"s3":              "Object storage, file uploads, static asset hosting",
	"elasticsearch":   "Full-text search, log analytics, document indexing",
	"repository":      "Data access abstraction, testable persistence layer",
	"migrations":      "Versioned database schema changes, reproducible deployments",
	"circuit-breaker": "External service calls that may fail or slow down",
	"retry":           "Transient failures from external services or networks",
	"rate-limiting":   "Public APIs needing abuse prevention",
	"idempotency":     "Operations that must be safe to retry (payments, webhooks)",
	"event-bus":       "Decoupled components communicating via domain events",
	"ci-bitbucket":    "Bitbucket Pipelines CI/CD configuration",
	"cqrs":            "Separate read/write models for complex query patterns",
	"ddd":             "Domain-Driven Design building blocks (AggregateRoot, ValueObject, DomainEvent)",
	"outbox":          "Guaranteed event delivery with transactional consistency",
	"saga":            "Multi-service transactions that need rollback capability",
	"worker":          "Background job processing, async task queues",
	"scheduler":       "Periodic tasks, cron-like scheduling",
	"auth-jwt":        "JWT-based authentication for stateless APIs",
	"cors":            "Browser-facing APIs needing cross-origin access",
	"audit-log":       "Compliance, change tracking, security forensics",
	"multi-tenancy":   "SaaS applications serving multiple organizations",
	"feature-flags":   "Gradual rollouts, A/B testing, kill switches",
	"observability":   "Distributed tracing, metrics, structured logging",
	"graceful":        "Standalone graceful shutdown manager for services without platform capability",
	"health":          "Orchestrator readiness/liveness probes",
	"request-id":      "End-to-end request tracing across services",
	"docker":          "Containerized deployments, local dev with dependencies",
	"ci-github":       "GitHub Actions CI/CD pipeline",
	"ci-gitlab":       "GitLab CI/CD pipeline",
	"makefile":        "Standard build targets (build, test, lint, run)",
	"devcontainer":    "VS Code Dev Containers for reproducible environments",
	"pre-commit":      "Git hooks for code quality checks before commit",
	"linting":         "Static analysis and code style enforcement",
	"testing":         "Test infrastructure and helper utilities",
	"validation":      "Input validation for API request payloads",
	"uuid":            "UUIDv7 primary keys (time-sortable, index-friendly)",
	"platform":        "Config, logging, and lifecycle management",
	"bootstrap":       "Testable dependency wiring with thin main.go",
	"templ":           "Type-safe HTML templates with Go (templ)",
	"htmx":            "Hypermedia-driven UI with HTMX partials",
	"static-assets":   "Embedded static files served via embed.FS",
	"http-client":     "Outbound HTTP calls to external services",
	"email-gateway":   "Transactional email sending",
	"mailpit":         "Local email testing (dev-only SMTP)",
	"i18n":            "Internationalization and localization",
	"api-versioning":  "API version management (URL or header-based)",
	"timeout":         "Context-based timeout middleware for HTTP handlers and service calls",
	"bulkhead":        "Concurrency limiter using semaphore pattern for load isolation",
	"oauth2":          "OAuth2 client credentials and authorization code flow support",
	"encryption":      "At-rest encryption helpers using AES-256-GCM",
}

// BuildCatalog reads all capability manifests and returns an organized catalog.
func BuildCatalog(templateFS fs.FS, installedCaps []string) ([]CatalogEntry, error) {
	if templateFS == nil {
		return nil, nil
	}

	installedSet := make(map[string]bool, len(installedCaps))
	for _, c := range installedCaps {
		installedSet[c] = true
	}

	capDir := path.Join("templates", "capabilities")
	entries, err := fs.ReadDir(templateFS, capDir)
	if err != nil {
		return nil, fmt.Errorf("read capabilities dir: %w", err)
	}

	catalog := make([]CatalogEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		data, err := fs.ReadFile(templateFS, path.Join(capDir, name, "capability.yaml"))
		if err != nil {
			continue
		}
		cm, err := scaffold.ParseCapabilityManifest(data)
		if err != nil {
			continue
		}

		catalog = append(catalog, CatalogEntry{
			Name:        name,
			Category:    categoryFor(name),
			Description: cm.Description,
			WhenToUse:   whenToUseFor(name),
			Installed:   installedSet[name],
			Suggests:    cm.Suggests,
		})
	}

	slices.SortFunc(catalog, func(a, b CatalogEntry) int {
		if c := cmp.Compare(a.Category, b.Category); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})

	return catalog, nil
}

func categoryFor(name string) string {
	if c, ok := capabilityCategories[name]; ok {
		return c
	}
	return "other"
}

func whenToUseFor(name string) string {
	if w, ok := whenToUse[name]; ok {
		return w
	}
	return ""
}

// writeCatalog writes the capability catalog section to the guide.
func writeCatalog(b *strings.Builder, catalog []CatalogEntry, installedCaps []string) {
	if len(catalog) == 0 {
		return
	}

	b.WriteString("## Capability Catalog\n\n")

	// Installed capabilities.
	var installed, available []CatalogEntry
	for _, e := range catalog {
		if e.Installed {
			installed = append(installed, e)
		} else {
			available = append(available, e)
		}
	}

	if len(installed) > 0 {
		b.WriteString("### Installed\n")
		b.WriteString("| Capability | Category | Purpose |\n")
		b.WriteString("|-----------|----------|--------|\n")
		for _, e := range installed {
			purpose := e.WhenToUse
			if purpose == "" {
				purpose = e.Description
			}
			fmt.Fprintf(b, "| %s | %s | %s |\n", e.Name, e.Category, purpose)
		}
		b.WriteString("\n")
	}

	if len(available) > 0 {
		b.WriteString("### Available (not installed)\n")
		b.WriteString("| Capability | Category | When to Consider |\n")
		b.WriteString("|-----------|----------|------------------|\n")
		for _, e := range available {
			when := e.WhenToUse
			if when == "" {
				when = e.Description
			}
			fmt.Fprintf(b, "| %s | %s | %s |\n", e.Name, e.Category, when)
		}
		b.WriteString("\n")
	}

}

// writeSmartSuggestions writes the "if you have X, you probably need Y" table.
// This is always included — it's the highest-value content for AI agents.
func writeSmartSuggestions(b *strings.Builder) {
	b.WriteString("## Smart Suggestions\n\n")
	b.WriteString("When you have certain capabilities, you almost certainly need others:\n\n")
	b.WriteString("| If You Have | You Probably Need | Why |\n")
	b.WriteString("|-------------|------------------|-----|\n")
	b.WriteString("| Any transport (http-api, grpc, kafka-consumer) | **platform** | Production services need config, logging, and lifecycle management |\n")
	b.WriteString("| http-api | **health** | Health endpoints enable orchestrator readiness probes |\n")
	b.WriteString("| http-api | **request-id** | Request ID propagation enables end-to-end request tracing |\n")
	b.WriteString("| http-api | **validation** | APIs need input validation for reliability |\n")
	b.WriteString("| http-api serving browsers | **cors** | Browser-facing APIs require CORS headers |\n")
	b.WriteString("| http-api, grpc | **graceful** | Clean shutdown prevents request drops during deploys |\n")
	b.WriteString("| http-api, grpc, kafka-consumer | **observability** | Distributed tracing and metrics for production debugging |\n")
	b.WriteString("| postgres, mysql | **uuid** | UUIDv7 provides database-friendly primary keys without index fragmentation |\n")
	b.WriteString("| postgres, mysql | **migrations** | Database schema changes should be versioned and reproducible |\n")
	b.WriteString("| http-client | **circuit-breaker** | Circuit breakers prevent cascade failures from flaky external services |\n")
	b.WriteString("| http-client | **retry** | Retry with backoff handles transient failures gracefully |\n")
	b.WriteString("| event-bus | **outbox** | Transactional outbox prevents event loss on process crash |\n")
	b.WriteString("| Any write operation that can be retried | **idempotency** | Idempotency keys prevent duplicate side effects |\n")
	b.WriteString("\n")
}

// writeCriticalInteractionWarnings writes dangerous capability combinations with severity tiers.
// 🔴 MUST = implement this or you will have a production incident.
// 🟡 SHOULD = implement this for production readiness.
// These are always included regardless of installed capabilities.
func writeCriticalInteractionWarnings(b *strings.Builder) {
	b.WriteString("## Interaction Warnings\n\n")
	b.WriteString("Severity: 🔴 MUST implement (production incident risk) · 🟡 SHOULD implement (production readiness)\n\n")

	// 🔴 MUST — skipping these causes data loss, duplicates, or security breaches.
	b.WriteString("### 🔴 MUST\n\n")
	b.WriteString("- **retry without idempotency** → Retrying non-idempotent operations (payments, order creation) causes DUPLICATE operations. Always pair retry with idempotency.\n")
	b.WriteString("- **event-bus without outbox** → If your process crashes between committing to DB and publishing the event, the event is LOST. Transactional outbox guarantees delivery.\n")
	b.WriteString("- **http-client without circuit-breaker** → A slow/failing external service will exhaust your connection pool and cascade failure to YOUR callers.\n")
	b.WriteString("- **multi-tenant without auth** → Tenant ID from headers without JWT/auth verification means any caller can impersonate any tenant.\n")
	b.WriteString("- **financial operation without audit-log** → Payments, refunds, and order mutations must have an append-only audit trail for compliance and dispute resolution.\n")
	b.WriteString("\n")

	// 🟡 SHOULD — skipping these creates operational blind spots or reliability gaps.
	b.WriteString("### 🟡 SHOULD\n\n")
	b.WriteString("- **http-api without health** → Production APIs behind load balancers or Kubernetes MUST have /healthz and /readyz endpoints.\n")
	b.WriteString("- **postgres without uuid** → Auto-increment IDs leak business data. Random UUIDs fragment B-tree indexes. Use UUIDv7 (time-sortable).\n")
	b.WriteString("- **kafka-consumer without graceful shutdown** → Ungraceful shutdown causes rebalancing storms and duplicate processing.\n")
	b.WriteString("- **any service without observability** → You cannot debug production without distributed tracing and structured metrics. Add at minimum: request-id + structured logging.\n")
	b.WriteString("- **external call without timeout** → Without context deadlines, a hung external call blocks your goroutine forever and exhausts your connection pool under load.\n")
	b.WriteString("\n")
}

// writeDesignQuestions writes prompts that trigger architectural reasoning.
func writeDesignQuestions(b *strings.Builder) {
	b.WriteString("## Design Questions\n\n")
	b.WriteString("When designing a new feature or service, ask yourself:\n\n")
	b.WriteString("1. **Does this feature call an external service?** → You need http-client + circuit-breaker + retry. If the call mutates state, add idempotency.\n")
	b.WriteString("2. **Does this feature write data that must not be duplicated?** → Add idempotency keys to the endpoint.\n")
	b.WriteString("3. **Does this feature serve public/browser traffic?** → Add rate-limiting + cors + auth.\n")
	b.WriteString("4. **Does this feature handle sensitive data or financial operations?** → Add audit-log.\n")
	b.WriteString("5. **Does this feature run as a background job or consumer?** → Ensure graceful shutdown handles in-flight work. Add health checks.\n")
	b.WriteString("6. **Is this a multi-tenant application?** → Every query must be scoped by tenant. Every endpoint must validate tenant ownership via authenticated identity (not just a header).\n")
	b.WriteString("7. **Will this be deployed to Kubernetes or behind a load balancer?** → Add health + readiness endpoints.\n")
	b.WriteString("8. **Does this feature publish domain events?** → Use outbox pattern if events must be reliable.\n")
	b.WriteString("9. **Does this feature persist data?** → Use UUIDv7 for primary keys. Add database migrations. Use transactions for multi-table writes.\n")
	b.WriteString("10. **Does the implementation order matter?** → Persist before calling external services (safer crash recovery). Or use saga/outbox for distributed operations.\n")
	b.WriteString("\n")
}

// writeWarnings writes context-specific capability interaction warnings based on installed caps.
// Uses the same 🔴/🟡 severity format as writeCriticalInteractionWarnings.
func writeWarnings(b *strings.Builder, installedCaps []string) {
	warnings := scaffold.CapabilityWarnings(installedCaps)
	if len(warnings) == 0 {
		return
	}

	var must, should []string
	for _, w := range warnings {
		if w.Critical {
			must = append(must, w.Message)
		} else {
			should = append(should, w.Message)
		}
	}

	b.WriteString("## Your Stack: Specific Warnings\n\n")
	b.WriteString("Based on your installed capabilities:\n\n")
	if len(must) > 0 {
		b.WriteString("### 🔴 MUST\n\n")
		for _, w := range must {
			fmt.Fprintf(b, "- %s\n", w)
		}
		b.WriteString("\n")
	}
	if len(should) > 0 {
		b.WriteString("### 🟡 SHOULD\n\n")
		for _, w := range should {
			fmt.Fprintf(b, "- %s\n", w)
		}
		b.WriteString("\n")
	}
}

// writeSuggestions writes architecture suggestions to the guide.
func writeSuggestions(b *strings.Builder, installedCaps []string) {
	suggestions := scaffold.ComputeSuggestions(installedCaps)
	if len(suggestions) > 5 {
		suggestions = suggestions[:5]
	}
	if len(suggestions) == 0 {
		return
	}

	b.WriteString("## Architecture Suggestions\n\n")
	for i, s := range suggestions {
		fmt.Fprintf(b, "%d. **Add %s** — %s\n", i+1, s.Capability, s.Reason)
	}
	b.WriteString("\n")
}
