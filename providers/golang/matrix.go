package golang

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diktahq/verikt/internal/config"
)

// capabilityMeta holds display metadata for the "What's In This Project" table
// in the docs/PROJECT.md file generated for each scaffolded project. It is not
// the capability registry: the set of Go capabilities is whatever lives under
// templates/capabilities/ in the embedded FS. TestCapabilityMetaCoversTemplates
// fails if an architecture or capability template has no entry here, because a
// missing entry silently drops that capability from the generated doc.
var capabilityMeta = map[string]matrixEntry{
	// Architecture
	"hexagonal": {Category: "Architecture", Pattern: "Hexagonal (Ports & Adapters)", Description: "Domain-centric with strict dependency direction: domain → ports → adapters"},
	"clean":     {Category: "Architecture", Pattern: "Clean Architecture", Description: "Entity, usecase, interface, and infrastructure layers"},
	"layered":   {Category: "Architecture", Pattern: "Layered", Description: "Handler, service, and repository layers"},
	"flat":      {Category: "Architecture", Pattern: "Flat", Description: "Single-package structure for simple CLIs and scripts"},

	// Design Patterns
	"bootstrap":  {Category: "Design Patterns", Pattern: "Bootstrap / Composition Root", Description: "Thin main.go delegates to internal/bootstrap for testable dependency wiring"},
	"platform":   {Category: "Design Patterns", Pattern: "Platform Layer", Description: "Config, lifecycle management, structured logging, OpenTelemetry, PII redaction"},
	"ddd":        {Category: "Design Patterns", Pattern: "DDD Building Blocks", Description: "AggregateRoot and DomainEvent base types"},
	"cqrs":       {Category: "Design Patterns", Pattern: "CQRS", Description: "Command bus and query bus with typed handlers"},
	"event-bus":  {Category: "Design Patterns", Pattern: "Event Bus", Description: "In-process domain event publish/subscribe"},
	"outbox":     {Category: "Design Patterns", Pattern: "Transactional Outbox", Description: "Reliable event publishing through an outbox table and relay"},
	"saga":       {Category: "Design Patterns", Pattern: "Saga Orchestrator", Description: "Step-based orchestration with compensation for distributed transactions"},
	"repository": {Category: "Design Patterns", Pattern: "Generic Repository", Description: "Repository interface using Go generics"},
	"worker":     {Category: "Design Patterns", Pattern: "Background Worker", Description: "Background job processor with graceful shutdown"},
	"scheduler":  {Category: "Design Patterns", Pattern: "Scheduler", Description: "Cron-style periodic task scheduler"},
	"i18n":       {Category: "Design Patterns", Pattern: "Internationalization", Description: "Message catalogs and locale middleware"},

	// Transport
	"http-api":       {Category: "Transport", Pattern: "REST API (Chi)", Description: "Chi router, middleware chain, RFC 7807 error responses, OpenAPI spec"},
	"grpc":           {Category: "Transport", Pattern: "gRPC", Description: "Protocol Buffers, buf tooling, unary/stream interceptors"},
	"graphql":        {Category: "Transport", Pattern: "GraphQL (gqlgen)", Description: "Schema-first GraphQL API with gqlgen code generation"},
	"websocket":      {Category: "Transport", Pattern: "WebSocket", Description: "WebSocket handler with connection management"},
	"sse":            {Category: "Transport", Pattern: "Server-Sent Events", Description: "Server-to-client event streaming over HTTP"},
	"bff":            {Category: "Transport", Pattern: "Backend-for-Frontend", Description: "Gateway that aggregates and shapes backend service responses"},
	"api-versioning": {Category: "Transport", Pattern: "API Versioning", Description: "URL prefix or header-based version selection"},
	"timeout":        {Category: "Transport", Pattern: "Timeout Middleware", Description: "Context deadlines for HTTP handlers and service calls"},

	// Messaging
	"kafka-consumer": {Category: "Messaging", Pattern: "Kafka Consumer", Description: "Consumer group with handler pattern, graceful shutdown"},
	"nats":           {Category: "Messaging", Pattern: "NATS", Description: "Publisher and subscriber with request-reply"},

	// Data
	"postgres":      {Category: "Data", Pattern: "PostgreSQL Repository", Description: "pgx driver, connection pooling, repository pattern"},
	"mysql":         {Category: "Data", Pattern: "MySQL Repository", Description: "Connection pooling, health checks, repository pattern"},
	"sqlite":        {Category: "Data", Pattern: "SQLite Repository", Description: "Pure-Go SQLite (modernc.org/sqlite), no CGO"},
	"mongodb":       {Category: "Data", Pattern: "MongoDB Repository", Description: "Official Go driver, base repository per collection"},
	"dynamodb":      {Category: "Data", Pattern: "DynamoDB Repository", Description: "AWS SDK v2 client and repository"},
	"redis":         {Category: "Data", Pattern: "Redis Repository", Description: "Connection management, repository pattern"},
	"elasticsearch": {Category: "Data", Pattern: "Elasticsearch", Description: "go-elasticsearch v8 client with index and search helpers"},
	"s3":            {Category: "Data", Pattern: "S3 Object Storage", Description: "AWS SDK v2 S3 client"},
	"migrations":    {Category: "Data", Pattern: "Migrations", Description: "Versioned SQL migrations with golang-migrate"},
	"uuid":          {Category: "Data", Pattern: "UUIDv7", Description: "Time-sortable IDs for database-friendly primary keys"},
	"squirrel":      {Category: "Data", Pattern: "Squirrel Query Builder", Description: "Fluent query construction, dynamic WHERE clauses, complex joins"},
	"sqlc":          {Category: "Data", Pattern: "sqlc", Description: "Type-safe database access from SQL files, compile-time query validation"},

	// Resilience
	"circuit-breaker": {Category: "Resilience", Pattern: "Circuit Breaker", Description: "Closed/open/half-open breaker for external calls"},
	"retry":           {Category: "Resilience", Pattern: "Retry", Description: "Exponential backoff with jitter"},
	"idempotency":     {Category: "Resilience", Pattern: "Idempotency Keys", Description: "Idempotency key middleware for safe request retries"},
	"bulkhead":        {Category: "Resilience", Pattern: "Bulkhead", Description: "Semaphore-based concurrency limiter for load isolation"},
	"graceful":        {Category: "Resilience", Pattern: "Graceful Shutdown", Description: "Standalone shutdown manager for services without the platform capability"},

	// Observability
	"observability": {Category: "Observability", Pattern: "OpenTelemetry + Prometheus", Description: "Distributed tracing and metrics"},
	"request-id":    {Category: "Observability", Pattern: "Request ID", Description: "Request ID middleware with context propagation"},
	"health":        {Category: "Observability", Pattern: "Health Probes", Description: "/healthz and /readyz endpoints"},
	"audit-log":     {Category: "Observability", Pattern: "Audit Log", Description: "Structured audit trail for compliance and debugging"},

	// Security
	"auth-jwt":      {Category: "Security", Pattern: "JWT Authentication", Description: "Middleware-based JWT validation with claims extraction"},
	"oauth2":        {Category: "Security", Pattern: "OAuth2", Description: "Client credentials and authorization code flow support"},
	"rate-limiting": {Category: "Security", Pattern: "Rate Limiting", Description: "Token bucket rate limiter middleware for HTTP endpoints"},
	"cors":          {Category: "Security", Pattern: "CORS", Description: "CORS middleware with configurable origins, methods, headers"},
	"validation":    {Category: "Security", Pattern: "Request Validation", Description: "go-playground/validator with structured field errors"},
	"encryption":    {Category: "Security", Pattern: "Encryption", Description: "AES-256-GCM at-rest encryption helpers"},
	"multi-tenancy": {Category: "Security", Pattern: "Multi-Tenancy", Description: "Tenant resolver, context propagation, and HTTP middleware"},
	"feature-flags": {Category: "Security", Pattern: "Feature Flags", Description: "Flag evaluation with local config fallback"},

	// Frontend
	"templ":         {Category: "Frontend", Pattern: "templ", Description: "Type-safe server-rendered HTML components"},
	"htmx":          {Category: "Frontend", Pattern: "HTMX", Description: "Hypermedia-driven partials served from Go handlers"},
	"static-assets": {Category: "Frontend", Pattern: "Static Assets", Description: "Embedded static files with cache headers"},

	// Integration
	"email-gateway": {Category: "Integration", Pattern: "Email Gateway", Description: "Adapter pattern for email sending with provider abstraction"},
	"http-client":   {Category: "Integration", Pattern: "HTTP Client", Description: "Resilient HTTP client with retry, timeout, and observability"},
	"mailpit":       {Category: "Integration", Pattern: "Mailpit", Description: "Local SMTP capture with web UI for email testing"},

	// Testing
	"testing": {Category: "Testing", Pattern: "Test Helpers", Description: "Test utilities, fixtures, and example table-driven tests"},

	// DevOps / CI
	"ci-github":    {Category: "DevOps", Pattern: "GitHub CI", Description: "Issue templates, PR template for standardized workflows"},
	"ci-gitlab":    {Category: "DevOps", Pattern: "GitLab CI", Description: "Lint, test, build, and docker pipeline stages"},
	"ci-bitbucket": {Category: "DevOps", Pattern: "Bitbucket Pipelines", Description: "Bitbucket Pipelines CI configuration"},
	"docker":       {Category: "DevOps", Pattern: "Docker Compose", Description: "Local development environment with service dependencies"},
	"devcontainer": {Category: "DevOps", Pattern: "Dev Container", Description: "VS Code Dev Container with Go tooling"},
	"makefile":     {Category: "DevOps", Pattern: "Makefile", Description: "build, test, lint, run, docker-build, clean targets"},
	"linting":      {Category: "DevOps", Pattern: "golangci-lint", Description: "Comprehensive Go linting with curated rule set"},
	"pre-commit":   {Category: "DevOps", Pattern: "Pre-commit Hooks", Description: "Automated code quality checks before every commit"},

	// Language
	"go": {Category: "Language", Pattern: "Go", Description: "Idiomatic Go with modules, standard project layout"},
}

type matrixEntry struct {
	Category    string
	Pattern     string
	Description string
}

func generateProjectMatrix(outputDir, architecture string, capabilities []string, veriktCfg *config.VeriktConfig) (string, error) {
	if err := os.MkdirAll(filepath.Join(outputDir, "docs"), 0o755); err != nil {
		return "", fmt.Errorf("create docs dir: %w", err)
	}

	// Build ordered entries by category.
	categoryOrder := []string{"Architecture", "Design Patterns", "Language", "Transport", "Messaging", "Data", "Resilience", "Observability", "Security", "Frontend", "Integration", "Testing", "DevOps"}
	categories := map[string][]matrixEntry{}

	// Add architecture.
	if entry, ok := capabilityMeta[architecture]; ok {
		categories[entry.Category] = append(categories[entry.Category], entry)
	}

	// Add language.
	categories["Language"] = append(categories["Language"], capabilityMeta["go"])

	// Add capabilities.
	for _, cap := range capabilities {
		if entry, ok := capabilityMeta[cap]; ok {
			categories[entry.Category] = append(categories[entry.Category], entry)
		}
	}

	var b strings.Builder
	b.WriteString("# Project Anatomy\n\n")
	fmt.Fprintf(&b, "> Generated by [verikt](https://github.com/diktahq/verikt) on %s\n\n", time.Now().Format("2006-01-02"))

	// Summary matrix table.
	b.WriteString("## What's In This Project\n\n")
	b.WriteString("| Category | Pattern | Description |\n")
	b.WriteString("|----------|---------|-------------|\n")

	for _, cat := range categoryOrder {
		entries, ok := categories[cat]
		if !ok {
			continue
		}
		for _, entry := range entries {
			fmt.Fprintf(&b, "| **%s** | %s | %s |\n", entry.Category, entry.Pattern, entry.Description)
		}
	}

	// Capability set for quick lookup.
	capSet := map[string]bool{}
	for _, c := range capabilities {
		capSet[c] = true
	}

	// Architecture components.
	if veriktCfg != nil && len(veriktCfg.Components) > 0 {
		b.WriteString("\n## Architecture Components\n\n")
		b.WriteString("| Component | Location | May Depend On |\n")
		b.WriteString("|-----------|----------|---------------|\n")
		for _, comp := range veriktCfg.Components {
			deps := "—"
			if len(comp.MayDependOn) > 0 {
				deps = strings.Join(comp.MayDependOn, ", ")
			}
			locs := strings.Join(comp.In, ", ")
			fmt.Fprintf(&b, "| %s | `%s` | %s |\n", comp.Name, locs, deps)
		}
		b.WriteString("\nDependency violations are checked by `verikt check`.\n")
	}

	// Middleware stack.
	if capSet["http-api"] {
		b.WriteString("\n## HTTP Middleware Stack\n\n")
		b.WriteString("Applied in order (outermost first):\n\n")
		b.WriteString("| # | Middleware | Source |\n")
		b.WriteString("|---|-----------|--------|\n")
		middlewares := []struct{ name, source string }{
			{"RequestID", "chi/middleware"},
			{"RealIP", "chi/middleware"},
			{"Recoverer", "chi/middleware"},
			{"otelhttp", "OpenTelemetry"},
		}
		if capSet["rate-limiting"] {
			middlewares = append(middlewares, struct{ name, source string }{"RateLimiter", "rate-limiting capability"})
		}
		if capSet["auth-jwt"] {
			middlewares = append(middlewares, struct{ name, source string }{"JWTAuth", "auth-jwt capability"})
		}
		for i, mw := range middlewares {
			fmt.Fprintf(&b, "| %d | %s | %s |\n", i+1, mw.name, mw.source)
		}
	}

	// Observability.
	if capSet["platform"] {
		b.WriteString("\n## Observability\n\n")
		b.WriteString("| Signal | Implementation | Notes |\n")
		b.WriteString("|--------|---------------|-------|\n")
		b.WriteString("| Logging | `slog` (structured) | JSON in production, text in dev |\n")
		b.WriteString("| Traces | OpenTelemetry → OTLP/gRPC | Auto-instrumented HTTP/gRPC |\n")
		b.WriteString("| Metrics | OpenTelemetry → OTLP/gRPC | Runtime + custom metrics |\n")
		b.WriteString("| PII Redaction | RedactHandler | Strips secrets from log output |\n")
		if capSet["bootstrap"] {
			b.WriteString("| Lifecycle | lifecycle.App | Graceful shutdown with ordered hooks |\n")
		}
	}

	// Error handling.
	if capSet["http-api"] {
		b.WriteString("\n## Error Handling\n\n")
		b.WriteString("| Layer | Pattern | Format |\n")
		b.WriteString("|-------|---------|--------|\n")
		b.WriteString("| Domain | Sentinel errors + typed validation errors | `domain.ErrNotFound`, `domain.ValidationError` |\n")
		b.WriteString("| HTTP | RFC 7807 Problem Detail | `application/problem+json` |\n")
		b.WriteString("| Mapping | `MapError()` auto-maps domain → HTTP status | 404, 409, 401, 403, 429, 500 |\n")
	}

	// Data layer.
	if capSet["mysql"] || capSet["redis"] {
		b.WriteString("\n## Data Layer\n\n")
		b.WriteString("| Store | Connection | Pattern |\n")
		b.WriteString("|-------|-----------|--------|\n")
		if capSet["mysql"] {
			b.WriteString("| MySQL | Pooled (`sql.DB`) | Repository per aggregate |\n")
		}
		if capSet["redis"] {
			b.WriteString("| Redis | Single connection | Repository pattern |\n")
		}
	}

	// DevOps checklist.
	b.WriteString("\n## DevOps Checklist\n\n")
	checks := []struct {
		item, status string
	}{
		{"Dockerfile", boolToCheck(fileExists(outputDir, "Dockerfile"))},
		{"Docker Compose", boolToCheck(capSet["docker"])},
		{"CI Pipeline", boolToCheck(capSet["ci-github"] || capSet["ci-gitlab"] || capSet["ci-bitbucket"])},
		{"Linter Config", boolToCheck(capSet["linting"])},
		{"Pre-commit Hooks", boolToCheck(capSet["pre-commit"])},
		{"OpenAPI Spec", boolToCheck(capSet["http-api"])},
		{"Live Reload (.air)", boolToCheck(capSet["platform"])},
		{"Makefile", boolToCheck(fileExists(outputDir, "Makefile"))},
		{"Dependency Graph (`verikt check`)", boolToCheck(true)},
	}
	for _, c := range checks {
		fmt.Fprintf(&b, "- %s %s\n", c.status, c.item)
	}

	// Key commands.
	b.WriteString("\n## Key Commands\n\n")
	b.WriteString("```bash\n")
	if architecture == "flat" {
		b.WriteString("# Run\ngo run .\n\n")
	} else {
		b.WriteString("# Run\ngo run ./cmd/<service-name>\n\n")
	}
	if capSet["platform"] {
		b.WriteString("# Live reload\nair\n\n")
	}
	b.WriteString("# Test\ngo test ./...\n\n")
	if capSet["linting"] {
		b.WriteString("# Lint\ngolangci-lint run\n\n")
	}
	b.WriteString("# Check architecture\nverikt check\n")
	if capSet["docker"] {
		b.WriteString("\n# Start dependencies\ndocker compose up -d\n")
	}
	b.WriteString("```\n")

	docPath := filepath.Join(outputDir, "docs", "PROJECT.md")
	if err := os.WriteFile(docPath, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("write project matrix: %w", err)
	}

	return docPath, nil
}

func boolToCheck(v bool) string {
	if v {
		return "[x]"
	}
	return "[ ]"
}

func fileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}
