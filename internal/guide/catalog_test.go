package guide

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dcsg/archway/internal/config"
	"github.com/dcsg/archway/internal/scaffold"
)

func testCapFS() fstest.MapFS {
	return fstest.MapFS{
		"templates/capabilities/http-api/capability.yaml": &fstest.MapFile{
			Data: []byte("name: http-api\ndescription: \"HTTP API with Chi router\"\nrequires: []\nsuggests: [rate-limiting, auth-jwt, cors]\nconflicts: []\n"),
		},
		"templates/capabilities/mysql/capability.yaml": &fstest.MapFile{
			Data: []byte("name: mysql\ndescription: \"MySQL database adapter\"\nrequires: []\nsuggests: [uuid, migrations]\nconflicts: []\n"),
		},
		"templates/capabilities/docker/capability.yaml": &fstest.MapFile{
			Data: []byte("name: docker\ndescription: \"Dockerfile and docker-compose\"\nrequires: []\nsuggests: []\nconflicts: []\n"),
		},
		"templates/capabilities/circuit-breaker/capability.yaml": &fstest.MapFile{
			Data: []byte("name: circuit-breaker\ndescription: \"Circuit breaker for resilient calls\"\nrequires: []\nsuggests: [observability]\nconflicts: []\n"),
		},
		"templates/capabilities/saga/capability.yaml": &fstest.MapFile{
			Data: []byte("name: saga\ndescription: \"Saga orchestrator\"\nrequires: [event-bus]\nsuggests: [observability]\nconflicts: []\n"),
		},
		"templates/capabilities/rate-limiting/capability.yaml": &fstest.MapFile{
			Data: []byte("name: rate-limiting\ndescription: \"Rate limiter middleware\"\nrequires: []\nsuggests: []\nconflicts: []\n"),
		},
		"templates/capabilities/cors/capability.yaml": &fstest.MapFile{
			Data: []byte("name: cors\ndescription: \"CORS middleware\"\nrequires: []\nsuggests: []\nconflicts: []\n"),
		},
		"templates/capabilities/health/capability.yaml": &fstest.MapFile{
			Data: []byte("name: health\ndescription: \"Health check endpoints\"\nrequires: []\nsuggests: []\nconflicts: []\n"),
		},
		"templates/capabilities/multi-tenancy/capability.yaml": &fstest.MapFile{
			Data: []byte("name: multi-tenancy\ndescription: \"Multi-tenant isolation middleware\"\nrequires: []\nsuggests: [auth-jwt]\nconflicts: []\n"),
		},
		"templates/capabilities/auth-jwt/capability.yaml": &fstest.MapFile{
			Data: []byte("name: auth-jwt\ndescription: \"JWT authentication middleware\"\nrequires: []\nsuggests: []\nconflicts: []\n"),
		},
	}
}

func TestBuildCatalog_WithFS(t *testing.T) {
	installed := []string{"http-api", "mysql"}
	catalog, err := BuildCatalog(testCapFS(), installed)
	if err != nil {
		t.Fatalf("BuildCatalog() error = %v", err)
	}
	if len(catalog) != 10 {
		t.Fatalf("expected 10 entries, got %d", len(catalog))
	}

	installedCount := 0
	for _, e := range catalog {
		if e.Installed {
			installedCount++
		}
		if e.Category == "" {
			t.Errorf("entry %q has empty category", e.Name)
		}
	}
	if installedCount != 2 {
		t.Errorf("expected 2 installed, got %d", installedCount)
	}
}

func TestBuildCatalog_NilFS(t *testing.T) {
	catalog, err := BuildCatalog(nil, []string{"http-api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if catalog != nil {
		t.Error("expected nil catalog for nil FS")
	}
}

func TestBuildCatalog_EmptyInstalled(t *testing.T) {
	catalog, err := BuildCatalog(testCapFS(), nil)
	if err != nil {
		t.Fatalf("BuildCatalog() error = %v", err)
	}
	for _, e := range catalog {
		if e.Installed {
			t.Errorf("entry %q should not be installed", e.Name)
		}
	}
}

func TestBuildCatalog_SortedByCategoryThenName(t *testing.T) {
	catalog, err := BuildCatalog(testCapFS(), nil)
	if err != nil {
		t.Fatalf("BuildCatalog() error = %v", err)
	}
	for i := 1; i < len(catalog); i++ {
		prev, curr := catalog[i-1], catalog[i]
		if prev.Category > curr.Category {
			t.Errorf("not sorted by category: %s/%s before %s/%s",
				prev.Category, prev.Name, curr.Category, curr.Name)
		}
		if prev.Category == curr.Category && prev.Name > curr.Name {
			t.Errorf("not sorted by name: %s before %s in %s",
				prev.Name, curr.Name, prev.Category)
		}
	}
}

func TestWriteCatalog_Output(t *testing.T) {
	catalog, _ := BuildCatalog(testCapFS(), []string{"http-api", "mysql"})
	var b strings.Builder
	writeCatalog(&b, catalog, []string{"http-api", "mysql"})
	output := b.String()

	checks := []string{
		"## Capability Catalog",
		"### Installed",
		"### Available (not installed)",
		"http-api",
		"mysql",
		"circuit-breaker",
	}
	for _, want := range checks {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q in output", want)
		}
	}
}

func TestWriteCatalog_EmptyCatalog(t *testing.T) {
	var b strings.Builder
	writeCatalog(&b, nil, nil)
	if b.Len() != 0 {
		t.Error("expected empty output for nil catalog")
	}
}

func TestWriteCatalog_SuggestionsMaxFive(t *testing.T) {
	catalog, _ := BuildCatalog(testCapFS(), []string{"http-api"})
	var b strings.Builder
	writeCatalog(&b, catalog, []string{"http-api"})
	output := b.String()
	count := strings.Count(output, "- Consider **")
	if count > 5 {
		t.Errorf("suggestions capped at 5, got %d", count)
	}
}

func TestCategoryFor_Known(t *testing.T) {
	if got := categoryFor("http-api"); got != "transport" {
		t.Errorf("categoryFor(http-api) = %q, want transport", got)
	}
}

func TestCategoryFor_BFF(t *testing.T) {
	if got := categoryFor("bff"); got != "transport" {
		t.Errorf("categoryFor(bff) = %q, want transport", got)
	}
}

func TestCategoryFor_Unknown(t *testing.T) {
	if got := categoryFor("nonexistent"); got != "other" {
		t.Errorf("categoryFor(nonexistent) = %q, want other", got)
	}
}

func TestWhenToUseFor_Known(t *testing.T) {
	if got := whenToUseFor("saga"); got == "" {
		t.Error("expected non-empty whenToUse for saga")
	}
}

func TestWhenToUseFor_Unknown(t *testing.T) {
	if got := whenToUseFor("nonexistent"); got != "" {
		t.Errorf("whenToUseFor(nonexistent) = %q, want empty", got)
	}
}

func TestCatalogInBuildContent(t *testing.T) {
	opts := GenerateOptions{
		Architecture: "hexagonal",
		Capabilities: []string{"http-api", "mysql"},
		TemplateFS:   testCapFS(),
	}
	content := buildContent(opts)
	if !strings.Contains(content, "## Capability Catalog") {
		t.Error("buildContent should include capability catalog")
	}
}

func TestWriteWarnings_DangerousCombination(t *testing.T) {
	var b strings.Builder
	writeWarnings(&b, []string{"http-api"}, nil)
	output := b.String()
	if !strings.Contains(output, "## Your Stack: Specific Warnings") {
		t.Error("expected Your Stack: Specific Warnings section")
	}
	if !strings.Contains(output, "rate-limiting") {
		t.Error("expected warning about rate-limiting")
	}
}

func TestWriteWarnings_SafeCombination(t *testing.T) {
	var b strings.Builder
	writeWarnings(&b, []string{"http-api", "rate-limiting", "cors", "health", "observability", "request-id"}, nil)
	output := b.String()
	if strings.Contains(output, "## Interaction Warnings") {
		t.Errorf("expected no warnings for safe combination, got: %s", output)
	}
}

func TestWriteWarnings_CriticalFirst(t *testing.T) {
	var b strings.Builder
	// multi-tenancy without auth triggers critical; http-api without health triggers regular.
	writeWarnings(&b, []string{"multi-tenancy", "http-api"}, nil)
	output := b.String()

	mustIdx := strings.Index(output, "🔴 MUST")
	shouldIdx := strings.Index(output, "🟡 SHOULD")
	if mustIdx < 0 {
		t.Fatal("expected 🔴 MUST warning")
	}
	if shouldIdx < 0 {
		t.Fatal("expected 🟡 SHOULD warning")
	}
	if mustIdx > shouldIdx {
		t.Error("🔴 MUST warnings should appear before 🟡 SHOULD")
	}
}

func TestWriteSuggestions_ReflectsActualMissing(t *testing.T) {
	var b strings.Builder
	writeSuggestions(&b, []string{"http-api"})
	output := b.String()
	if !strings.Contains(output, "## Architecture Suggestions") {
		t.Error("expected Architecture Suggestions section")
	}
	if !strings.Contains(output, "**Add ") {
		t.Error("expected numbered suggestion format")
	}
	// rate-limiting is a suggestion for http-api
	if !strings.Contains(output, "rate-limiting") {
		t.Error("expected rate-limiting suggestion for http-api")
	}
}

func TestWriteSuggestions_EmptyWhenAllInstalled(t *testing.T) {
	var b strings.Builder
	// Provide capabilities that satisfy all suggestion rules triggered by http-api.
	writeSuggestions(&b, []string{
		"http-api", "platform", "bootstrap", "rate-limiting", "auth-jwt",
		"testing", "docker", "ci-github", "linting", "cors", "health",
		"observability", "request-id",
	})
	output := b.String()
	if strings.Contains(output, "## Architecture Suggestions") {
		t.Errorf("expected no suggestions, got: %s", output)
	}
}

// Tests for severity tiers (A improvement) — writeCriticalInteractionWarnings.

func TestWriteCriticalInteractionWarnings_HasSeverityLegend(t *testing.T) {
	var b strings.Builder
	writeCriticalInteractionWarnings(&b)
	output := b.String()
	if !strings.Contains(output, "🔴 MUST") {
		t.Error("expected severity legend with 🔴 MUST")
	}
	if !strings.Contains(output, "🟡 SHOULD") {
		t.Error("expected severity legend with 🟡 SHOULD")
	}
}

func TestWriteCriticalInteractionWarnings_HasMUSTSubsection(t *testing.T) {
	var b strings.Builder
	writeCriticalInteractionWarnings(&b)
	output := b.String()
	if !strings.Contains(output, "### 🔴 MUST") {
		t.Error("expected ### 🔴 MUST subsection")
	}
}

func TestWriteCriticalInteractionWarnings_HasSHOULDSubsection(t *testing.T) {
	var b strings.Builder
	writeCriticalInteractionWarnings(&b)
	output := b.String()
	if !strings.Contains(output, "### 🟡 SHOULD") {
		t.Error("expected ### 🟡 SHOULD subsection")
	}
}

func TestWriteCriticalInteractionWarnings_MUSTBeforesSHOULD(t *testing.T) {
	var b strings.Builder
	writeCriticalInteractionWarnings(&b)
	output := b.String()
	mustIdx := strings.Index(output, "### 🔴 MUST")
	shouldIdx := strings.Index(output, "### 🟡 SHOULD")
	if mustIdx < 0 || shouldIdx < 0 {
		t.Fatal("missing subsections")
	}
	if mustIdx > shouldIdx {
		t.Error("🔴 MUST subsection should appear before 🟡 SHOULD")
	}
}

func TestWriteCriticalInteractionWarnings_AllFiveMUSTWarnings(t *testing.T) {
	var b strings.Builder
	writeCriticalInteractionWarnings(&b)
	output := b.String()
	mustWarnings := []string{
		"retry without idempotency",
		"event-bus without outbox",
		"http-client without circuit-breaker",
		"multi-tenant without auth",
		"financial operation without audit-log",
	}
	for _, w := range mustWarnings {
		if !strings.Contains(output, w) {
			t.Errorf("missing MUST warning: %q", w)
		}
	}
}

func TestWriteCriticalInteractionWarnings_AllFiveSHOULDWarnings(t *testing.T) {
	var b strings.Builder
	writeCriticalInteractionWarnings(&b)
	output := b.String()
	shouldWarnings := []string{
		"http-api without health",
		"postgres without uuid",
		"kafka-consumer without graceful shutdown",
		"any service without observability",
		"external call without timeout",
	}
	for _, w := range shouldWarnings {
		if !strings.Contains(output, w) {
			t.Errorf("missing SHOULD warning: %q", w)
		}
	}
}

// Tests for context-aware warnings (C improvement) — writeWarnings with Critical field.

func TestWriteWarnings_CriticalCapProducesMUSTSection(t *testing.T) {
	var b strings.Builder
	// http-client without circuit-breaker is Critical=true.
	writeWarnings(&b, []string{"http-client"}, nil)
	output := b.String()
	if !strings.Contains(output, "### 🔴 MUST") {
		t.Error("expected ### 🔴 MUST section for critical cap combination")
	}
	if !strings.Contains(output, "circuit-breaker") {
		t.Error("expected circuit-breaker in output")
	}
}

func TestWriteWarnings_NonCriticalCapProducesSHOULDSection(t *testing.T) {
	var b strings.Builder
	// http-api without health is Critical=false.
	writeWarnings(&b, []string{"http-api", "rate-limiting", "cors", "request-id", "observability"}, nil)
	output := b.String()
	if !strings.Contains(output, "### 🟡 SHOULD") {
		t.Error("expected ### 🟡 SHOULD section for non-critical cap combination")
	}
}

func TestWriteWarnings_NoOutputWhenAllSatisfied(t *testing.T) {
	var b strings.Builder
	// Satisfy all warning rules that fire for http-client and http-api.
	writeWarnings(&b, []string{
		"http-client", "circuit-breaker", "retry",
		"http-api", "health", "rate-limiting", "cors", "observability", "request-id",
	}, nil)
	output := b.String()
	if strings.Contains(output, "Your Stack: Specific Warnings") {
		t.Errorf("expected no warnings when all caps satisfied, got: %s", output)
	}
}

func TestWriteWarnings_SectionTitle(t *testing.T) {
	var b strings.Builder
	writeWarnings(&b, []string{"http-client"}, nil)
	output := b.String()
	if !strings.Contains(output, "## Your Stack: Specific Warnings") {
		t.Error("expected section title '## Your Stack: Specific Warnings'")
	}
}

func TestWriteWarnings_MixedCriticalAndShouldBothPresent(t *testing.T) {
	var b strings.Builder
	// multi-tenancy without auth = Critical; http-api without health = non-critical.
	writeWarnings(&b, []string{"multi-tenancy", "http-api"}, nil)
	output := b.String()
	if !strings.Contains(output, "### 🔴 MUST") {
		t.Error("expected ### 🔴 MUST section")
	}
	if !strings.Contains(output, "### 🟡 SHOULD") {
		t.Error("expected ### 🟡 SHOULD section")
	}
}

// Tests for severity overrides in writeWarnings.

func TestGuideOutput_WithSeverityOverrides_ShowsPathQualifiedSeverity(t *testing.T) {
	overrides := config.SeverityOverrides{
		"http-client": []config.SeverityOverride{
			{Severity: "should", Reason: "Istio handles this", Paths: []string{"adapter/grpc/**"}},
			{Severity: "ignore", Reason: "Test doubles", Paths: []string{"adapter/mock/**"}},
		},
	}
	var b strings.Builder
	writeWarnings(&b, []string{"http-client"}, overrides)
	output := b.String()

	if !strings.Contains(output, "🟡 SHOULD") {
		t.Error("expected 🟡 SHOULD override line")
	}
	if !strings.Contains(output, "adapter/grpc/**") {
		t.Error("expected adapter/grpc/** glob in output")
	}
	if !strings.Contains(output, "Istio handles this") {
		t.Error("expected reason 'Istio handles this' in output")
	}
	if !strings.Contains(output, "⚪ IGNORE") {
		t.Error("expected ⚪ IGNORE override line")
	}
	if !strings.Contains(output, "adapter/mock/**") {
		t.Error("expected adapter/mock/** glob in output")
	}
	if !strings.Contains(output, "Test doubles") {
		t.Error("expected reason 'Test doubles' in output")
	}
}

func TestGuideOutput_NoOverrides_UnchangedOutput(t *testing.T) {
	var bWith, bWithout strings.Builder
	writeWarnings(&bWith, []string{"http-client"}, nil)
	writeWarnings(&bWithout, []string{"http-client"}, config.SeverityOverrides{})

	if bWith.String() != bWithout.String() {
		t.Error("empty overrides should produce identical output to nil overrides")
	}
}

// Tests for CapabilityWarnings Critical field in scaffold package.

func TestCapabilityWarnings_HttpClientWithoutCircuitBreaker_IsCritical(t *testing.T) {
	warnings := testCapabilityWarningsFor([]string{"http-client"}, "circuit-breaker")
	if len(warnings) == 0 {
		t.Fatal("expected warning for http-client without circuit-breaker")
	}
	for _, w := range warnings {
		if strings.Contains(w.Message, "circuit-breaker") && !w.Critical {
			t.Error("http-client without circuit-breaker should be Critical=true")
		}
	}
}

func TestCapabilityWarnings_MultiTenancyWithoutAuth_IsCritical(t *testing.T) {
	warnings := testCapabilityWarningsFor([]string{"multi-tenancy"}, "auth-jwt")
	if len(warnings) == 0 {
		t.Fatal("expected warning for multi-tenancy without auth-jwt")
	}
	for _, w := range warnings {
		if strings.Contains(w.Message, "multi-tenancy") && !w.Critical {
			t.Error("multi-tenancy without auth should be Critical=true")
		}
	}
}

func TestCapabilityWarnings_EventBusWithoutOutbox_IsCritical(t *testing.T) {
	warnings := testCapabilityWarningsFor([]string{"event-bus"}, "outbox")
	if len(warnings) == 0 {
		t.Fatal("expected warning for event-bus without outbox")
	}
	for _, w := range warnings {
		if strings.Contains(w.Message, "event-bus") && !w.Critical {
			t.Error("event-bus without outbox should be Critical=true")
		}
	}
}

func TestCapabilityWarnings_HttpApiWithoutHealth_IsNotCritical(t *testing.T) {
	warnings := testCapabilityWarningsFor([]string{"http-api"}, "health")
	if len(warnings) == 0 {
		t.Fatal("expected warning for http-api without health")
	}
	for _, w := range warnings {
		if strings.Contains(w.Message, "health") && w.Critical {
			t.Error("http-api without health should be Critical=false")
		}
	}
}

func TestCapabilityWarnings_PostgresWithoutUUID_IsNotCritical(t *testing.T) {
	warnings := testCapabilityWarningsFor([]string{"postgres"}, "uuid")
	if len(warnings) == 0 {
		t.Fatal("expected warning for postgres without uuid")
	}
	for _, w := range warnings {
		if strings.Contains(w.Message, "uuid") && w.Critical {
			t.Error("postgres without uuid should be Critical=false")
		}
	}
}

// testCapabilityWarningsFor returns warnings triggered by caps, excluding those
// that would be suppressed when suppressed cap is NOT in the list.
func testCapabilityWarningsFor(caps []string, missingCap string) []scaffoldWarning {
	from := scaffold.CapabilityWarnings(caps)
	var relevant []scaffoldWarning
	for _, w := range from {
		if strings.Contains(w.Message, missingCap) {
			relevant = append(relevant, w)
		}
	}
	return relevant
}

// scaffoldWarning aliases the scaffold type for use in catalog_test.
type scaffoldWarning = scaffold.CapabilityWarning

func TestBuildContent_IncludesWarningsAndSuggestions(t *testing.T) {
	opts := GenerateOptions{
		Architecture: "hexagonal",
		Capabilities: []string{"http-api", "mysql"},
		TemplateFS:   testCapFS(),
	}
	content := buildContent(opts)
	if !strings.Contains(content, "## Interaction Warnings") {
		t.Error("buildContent should include Interaction Warnings")
	}
	if !strings.Contains(content, "## Architecture Suggestions") {
		t.Error("buildContent should include Architecture Suggestions")
	}
}
