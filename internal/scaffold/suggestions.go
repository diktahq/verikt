package scaffold

// Suggestion represents a capability the user might want to add.
type Suggestion struct {
	Capability string
	Reason     string
}

// SuggestionRule defines when to suggest a capability.
type SuggestionRule struct {
	// IfAny triggers the rule when any of these capabilities are selected.
	IfAny []string
	// Missing is the capability to suggest (skipped if already selected).
	Missing string
	// Reason explains why the suggestion is relevant.
	Reason string
}

var defaultSuggestionRules = []SuggestionRule{
	{IfAny: []string{"http-api", "grpc", "kafka-consumer", "mysql", "redis"}, Missing: "platform", Reason: "Production services need config, logging, and lifecycle management"},
	{IfAny: []string{"platform"}, Missing: "bootstrap", Reason: "Bootstrap pattern provides testable dependency wiring with thin main.go"},
	{IfAny: []string{"http-api"}, Missing: "rate-limiting", Reason: "HTTP APIs benefit from rate limiting to prevent abuse"},
	{IfAny: []string{"http-api"}, Missing: "auth-jwt", Reason: "HTTP APIs typically need authentication"},
	{IfAny: []string{"http-api"}, Missing: "testing", Reason: "APIs need handler tests for reliability"},
	{IfAny: []string{"mysql", "redis"}, Missing: "docker", Reason: "docker-compose simplifies local development with external dependencies"},
	{IfAny: []string{"http-api", "grpc", "kafka-consumer"}, Missing: "ci-github", Reason: "CI/CD catches issues before they reach production"},
	{IfAny: []string{"http-api", "grpc", "kafka-consumer"}, Missing: "linting", Reason: "Linting catches code quality issues early"},
	{IfAny: []string{"http-api", "grpc"}, Missing: "docker", Reason: "Docker simplifies deployment and local development"},
	{IfAny: []string{"postgres", "mysql"}, Missing: "uuid", Reason: "UUIDv7 provides database-friendly primary keys without index fragmentation"},
	{IfAny: []string{"postgres", "mysql"}, Missing: "migrations", Reason: "Database schema changes should be versioned and reproducible"},
	{IfAny: []string{"event-bus"}, Missing: "outbox", Reason: "Transactional outbox prevents event loss on process crash"},
	{IfAny: []string{"http-client"}, Missing: "circuit-breaker", Reason: "Circuit breakers prevent cascade failures from flaky external services"},
	{IfAny: []string{"http-client"}, Missing: "retry", Reason: "Retry with backoff handles transient failures gracefully"},
	{IfAny: []string{"http-api"}, Missing: "cors", Reason: "Browser-facing APIs require CORS headers"},
	{IfAny: []string{"http-api"}, Missing: "health", Reason: "Health endpoints enable orchestrator readiness probes"},
	{IfAny: []string{"http-api", "grpc", "kafka-consumer"}, Missing: "observability", Reason: "Distributed tracing and metrics for production debugging"},
	{IfAny: []string{"http-api", "grpc"}, Missing: "request-id", Reason: "Request ID propagation enables end-to-end request tracing"},
}

// ComputeSuggestions returns capabilities the user might want based on their selections.
func ComputeSuggestions(selected []string) []Suggestion {
	selectedSet := make(map[string]bool, len(selected))
	for _, s := range selected {
		selectedSet[s] = true
	}

	seen := map[string]bool{}
	var suggestions []Suggestion

	for _, rule := range defaultSuggestionRules {
		if selectedSet[rule.Missing] || seen[rule.Missing] {
			continue
		}
		for _, trigger := range rule.IfAny {
			if selectedSet[trigger] {
				seen[rule.Missing] = true
				suggestions = append(suggestions, Suggestion{
					Capability: rule.Missing,
					Reason:     rule.Reason,
				})
				break
			}
		}
	}
	return suggestions
}

// CapabilityWarning represents a potential issue with the selected capability combination.
type CapabilityWarning struct {
	Key      string // the triggering capability (ifHas), used as override key
	Message  string
	Critical bool // true = 🔴 MUST, false = 🟡 SHOULD
}

// warningRule defines when to warn about a capability combination.
type warningRule struct {
	ifHas    string   // capability that triggers the check
	missing  []string // warn if NONE of these are selected
	message  string
	critical bool // true = 🔴 MUST, false = 🟡 SHOULD
}

var defaultWarningRules = []warningRule{
	// 🔴 MUST — skipping causes data loss, duplicates, or security breaches.
	{ifHas: "http-client", missing: []string{"circuit-breaker"}, message: "**http-client without circuit-breaker** → A slow/failing dependency exhausts your connection pool and cascades failure to your callers. Add circuit-breaker.", critical: true},
	{ifHas: "multi-tenancy", missing: []string{"auth-jwt"}, message: "**multi-tenancy without auth** → Any caller can impersonate any tenant by setting the header. Add auth-jwt to verify identity before trusting tenant context.", critical: true},
	{ifHas: "event-bus", missing: []string{"outbox"}, message: "**event-bus without outbox** → Events published after DB commit can be lost on crash. Add outbox for guaranteed delivery.", critical: true},

	// 🟡 SHOULD — skipping creates operational blind spots or reliability gaps.
	{ifHas: "postgres", missing: []string{"uuid"}, message: "**postgres without uuid** → Auto-increment IDs leak row counts. Random UUIDs fragment indexes. Use UUIDv7 (time-sortable, index-friendly).", critical: false},
	{ifHas: "mysql", missing: []string{"uuid"}, message: "**mysql without uuid** → Auto-increment IDs leak row counts. Random UUIDs fragment indexes. Use UUIDv7 (time-sortable, index-friendly).", critical: false},
	{ifHas: "http-api", missing: []string{"health"}, message: "**http-api without health** → Load balancers and Kubernetes need /healthz and /readyz to route traffic correctly.", critical: false},
	{ifHas: "kafka-consumer", missing: []string{"graceful"}, message: "**kafka-consumer without graceful shutdown** → Abrupt exit causes consumer group rebalancing storms and duplicate message processing.", critical: false},
	{ifHas: "grpc", missing: []string{"health"}, message: "**grpc without health** → gRPC services should implement the health checking protocol for load balancer integration.", critical: false},
	{ifHas: "http-api", missing: []string{"observability", "request-id"}, message: "**http-api without request-id** → Without request ID propagation, you cannot trace a request across logs. Add request-id at minimum.", critical: false},
	{ifHas: "http-api", missing: []string{"rate-limiting"}, message: "**http-api without rate-limiting** → Public APIs are vulnerable to abuse and thundering herd. Add rate-limiting.", critical: false},
	{ifHas: "http-client", missing: []string{"retry"}, message: "**http-client without retry** → Transient failures silently propagate. Add retry with exponential backoff for idempotent calls.", critical: false},
	{ifHas: "http-api", missing: []string{"cors"}, message: "**http-api without cors** → Browser clients will be blocked by CORS policy. Add cors if this API serves browser traffic.", critical: false},
}

// CapabilityWarnings returns warnings about potentially problematic capability combinations.
func CapabilityWarnings(selected []string) []CapabilityWarning {
	selectedSet := make(map[string]bool, len(selected))
	for _, s := range selected {
		selectedSet[s] = true
	}

	var warnings []CapabilityWarning
	for _, rule := range defaultWarningRules {
		if !selectedSet[rule.ifHas] {
			continue
		}
		// Warn only if NONE of the missing capabilities are selected.
		hasSome := false
		for _, m := range rule.missing {
			if selectedSet[m] {
				hasSome = true
				break
			}
		}
		if !hasSome {
			warnings = append(warnings, CapabilityWarning{Key: rule.ifHas, Message: rule.message, Critical: rule.critical})
		}
	}
	return warnings
}
