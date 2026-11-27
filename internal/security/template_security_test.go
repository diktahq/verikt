package security_test

import (
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/checker/scriptguard"
	"github.com/diktahq/verikt/internal/provider"

	// Register providers so provider.Get works in tests.
	_ "github.com/diktahq/verikt/providers/golang"
	_ "github.com/diktahq/verikt/providers/typescript"
)

// denyPatterns are regex patterns that must never appear in template output.
// This test catches security regressions across all providers and templates.
var denyPatterns = []struct {
	pattern *regexp.Regexp
	reason  string
	// fileGlob limits which files this pattern applies to (empty = all .tmpl files).
	fileGlob string
}{
	{
		pattern: regexp.MustCompile(`change-me-in-production|change.me.in.prod`),
		reason:  "hardcoded fallback secret — service starts with forgeable tokens if env var unset",
	},
	{
		pattern: regexp.MustCompile(`image:\s*\S+:latest`),
		reason:  "unpinned Docker image tag — builds are not reproducible",
	},
	{
		// INV-003 item 3: an image must be pinned to a version. No tag, or a tag
		// that doesn't start with a version (dind, alpine), floats. A templated
		// tag ({{...}}) is resolved at scaffold time and allowed.
		pattern:  regexp.MustCompile(`(?m)image:\s*[\w./-]+(?::(?:[^\d{v\s]\S*|v\D\S*))?\s*$`),
		reason:   "Docker image without a version tag — it floats to whatever is published next",
		fileGlob: "*.yml.tmpl",
	},
	{
		// The same rule for images listed as YAML items (GitLab CI services).
		// A bare item with no tag (`- docker`) is a runner tag, not an image, so
		// only items that carry a non-version tag are reported. A tag never
		// contains "/", which keeps volume mounts (`- pg_data:/var/lib/...`) out.
		pattern:  regexp.MustCompile(`(?m)^\s*-\s*[\w./-]+:(?:[^\d{v\s"'/][^\s/]*|v[^\d\s/][^\s/]*)\s*$`),
		reason:   "Docker service image without a version tag — it floats to whatever is published next",
		fileGlob: "*.yml.tmpl",
	},
	{
		pattern:  regexp.MustCompile(`AllowedOrigins:\s*\[\]string\{"\*"\}`),
		reason:   "CORS wildcard origin in Go templates",
		fileGlob: "*cors*",
	},
	{
		pattern:  regexp.MustCompile(`\?\?\s*\['\*'\]`),
		reason:   "CORS wildcard origin fallback in TypeScript templates",
		fileGlob: "*cors*",
	},
	{
		pattern:  regexp.MustCompile(`fmt\.Sprintf\([^)]*grant_type=`),
		reason:   "OAuth2 token exchange must use url.Values, not fmt.Sprintf — prevents parameter injection",
		fileGlob: "*oauth2*",
	},
	{
		pattern:  regexp.MustCompile(`reflection:\s*true`),
		reason:   "gRPC reflection enabled by default — exposes service schema in production",
		fileGlob: "*config*example*",
	},
	{
		pattern: regexp.MustCompile(`CheckOrigin:\s*func\s*\([^)]*\)\s*bool\s*\{(?:\s*//[^\n]*)*\s*return\s+true\s*\}`),
		reason:  "WebSocket CheckOrigin accepts every origin — enables cross-site WebSocket hijacking; leave it unset (same-origin) or check an allow-list",
	},
	{
		pattern: regexp.MustCompile(`middleware\.RealIP\b`),
		reason:  "chi middleware.RealIP trusts client-supplied X-Forwarded-For/X-Real-IP/True-Client-IP (deprecated in chi v5.5.0, GHSA-3fxj-6jh8-hvhx) — spoofed client IPs bypass rate limits",
	},
	{
		pattern:  regexp.MustCompile(`Header\.Get\([^)]*[Tt]enant`),
		reason:   "tenant identity read from a request header — any caller can impersonate any tenant; resolve it from verified auth",
		fileGlob: "*tenant*",
	},
	{
		pattern:  regexp.MustCompile(`strings\.Join\(cfg\.AllowedOrigins`),
		reason:   "CORS Access-Control-Allow-Origin built from the whole allow-list — echo only the matched request Origin",
		fileGlob: "*cors*",
	},
	{
		pattern:  regexp.MustCompile(`sha256\.Sum256\(\[\]byte\(key\)\)`),
		reason:   "idempotency cache keyed by the Idempotency-Key alone — another caller with the same key receives this caller's response",
		fileGlob: "*idempotency*",
	},
}

// insecureFixtures are template snippets each deny pattern exists to catch.
// TestDenyPatterns_CatchInsecureFixtures proves the patterns fire, so a
// pattern that stops matching fails here instead of passing every template.
var insecureFixtures = []struct {
	name    string
	file    string
	content string
}{
	{
		name:    "websocket CheckOrigin accepting every origin, one line",
		file:    "websocket.go.tmpl",
		content: "var upgrader = websocket.Upgrader{\n\tCheckOrigin: func(r *http.Request) bool { return true },\n}\n",
	},
	{
		name:    "websocket CheckOrigin accepting every origin, block body",
		file:    "ws.go.tmpl",
		content: "upgrader := websocket.Upgrader{\n\tCheckOrigin: func(_ *http.Request) bool {\n\t\treturn true\n\t},\n}\n",
	},
	{
		// The websocket template shipped this before #29.
		name:    "websocket CheckOrigin accepting every origin behind a comment",
		file:    "websocket.go.tmpl",
		content: "var upgrader = websocket.Upgrader{\n\tCheckOrigin: func(r *http.Request) bool {\n\t\t// Override with proper origin checking in production.\n\t\treturn true\n\t},\n}\n",
	},
	{
		// The http-api router shipped this before 0.3.0.
		name:    "router trusting forwarding headers via chi RealIP",
		file:    "router.go.tmpl",
		content: "\tr.Use(middleware.Recoverer)\n\tr.Use(middleware.RealIP)\n",
	},
	{
		// The multi-tenancy middleware shipped this before 0.3.0.
		name:    "tenant taken from a raw request header",
		file:    "tenant.go.tmpl",
		content: "\t\tid := r.Header.Get(tenantIDHeader)\n\t\tctx := tenant.WithTenant(r.Context(), id)\n",
	},
	{
		// The cors middleware shipped this before 0.3.0.
		name:    "CORS allow-list joined into one header",
		file:    "middleware_cors.go.tmpl",
		content: "\torigins := strings.Join(cfg.AllowedOrigins, \", \")\n",
	},
	{
		// The idempotency middleware shipped this before 0.3.0.
		name:    "idempotency cache keyed by the raw key only",
		file:    "middleware_idempotency.go.tmpl",
		content: "\t\t\thash := sha256.Sum256([]byte(key))\n",
	},
	{
		// The ci-gitlab template shipped this before 0.3.0.
		name:    "CI service image listed without a version tag",
		file:    ".gitlab-ci.yml.tmpl",
		content: "  image: docker:27\n  services:\n    - docker:dind\n",
	},
	{
		name:    "compose image with no tag at all",
		file:    "docker-compose.yml.tmpl",
		content: "  redis:\n    image: redis\n",
	},
}

func TestDenyPatterns_CatchInsecureFixtures(t *testing.T) {
	for _, fx := range insecureFixtures {
		t.Run(fx.name, func(t *testing.T) {
			if len(deniedBy(fx.file, fx.content)) == 0 {
				t.Errorf("no deny pattern matches %s:\n%s", fx.file, fx.content)
			}
		})
	}
}

// deniedBy returns the reasons of every deny pattern that applies to file and
// matches content.
func deniedBy(file, content string) []string {
	var reasons []string
	for _, dp := range denyPatterns {
		if dp.fileGlob != "" {
			if matched, _ := filepath.Match(dp.fileGlob, filepath.Base(file)); !matched {
				continue
			}
		}
		if dp.pattern.MatchString(content) {
			reasons = append(reasons, dp.reason+"\n  pattern: "+dp.pattern.String())
		}
	}
	return reasons
}

func TestTemplates_NoInsecurePatterns(t *testing.T) {
	providers := []struct {
		name string
		p    provider.LanguageProvider
	}{
		{"golang", mustGetProvider(t, "go")},
		{"typescript", mustGetProvider(t, "typescript")},
	}

	for _, prov := range providers {
		t.Run(prov.name, func(t *testing.T) {
			tmplFS := prov.p.GetTemplateFS()
			err := fs.WalkDir(tmplFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}
				if !strings.HasSuffix(path, ".tmpl") && !strings.HasSuffix(path, ".yml.tmpl") && !strings.HasSuffix(path, ".yaml.tmpl") {
					return nil
				}

				data, readErr := fs.ReadFile(tmplFS, path)
				if readErr != nil {
					t.Errorf("failed to read %s: %v", path, readErr)
					return nil
				}
				for _, reason := range deniedBy(path, string(data)) {
					t.Errorf("[SECURITY] %s/%s: %s", prov.name, path, reason)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walking %s templates: %v", prov.name, err)
			}
		})
	}
}

// TestTemplates_NoMutableInstallURLs runs the line-based verikt check guards
// (mutable_install_url and silent_ci_swallow) over every Dockerfile, CI
// config, Makefile and script template, so verikt never scaffolds a project
// that fails its own check (INV-003: no unpinned dependencies). The guards
// ship at warning severity in verikt check; here any finding fails.
func TestTemplates_NoMutableInstallURLs(t *testing.T) {
	for _, lang := range []string{"go", "typescript"} {
		t.Run(lang, func(t *testing.T) {
			tmplFS := mustGetProvider(t, lang).GetTemplateFS()
			scanned := 0
			err := fs.WalkDir(tmplFS, ".", func(p string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil || d.IsDir() || !strings.HasSuffix(p, ".tmpl") {
					return walkErr
				}
				kind, ok := templateScriptKind(p)
				if !ok {
					return nil
				}
				data, err := fs.ReadFile(tmplFS, p)
				if err != nil {
					return err
				}
				scanned++
				for _, v := range scriptguard.Scan(p, kind, data, scriptguard.Options{}) {
					t.Errorf("[SECURITY] %s:%d [%s] %s", p, v.Line, v.Rule, v.Message)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walking %s templates: %v", lang, err)
			}
			if scanned == 0 {
				t.Fatalf("no %s Dockerfile/CI/Makefile templates scanned; template layout changed?", lang)
			}
		})
	}
}

// templateScriptKind classifies a template by the path it renders to: the
// part after ".../files/", without ".tmpl". GitLab and Bitbucket CI configs,
// which the project walk does not read, are scanned as generic CI config.
func templateScriptKind(tmplPath string) (scriptguard.Kind, bool) {
	out := strings.TrimSuffix(tmplPath, ".tmpl")
	if _, after, ok := strings.Cut(out, "/files/"); ok {
		out = after
	}
	switch path.Base(out) {
	case ".gitlab-ci.yml", "bitbucket-pipelines.yml":
		return scriptguard.KindCIConfig, true
	}
	return scriptguard.Classify(out, false)
}

func mustGetProvider(t *testing.T, lang string) provider.LanguageProvider {
	t.Helper()
	p, err := provider.Get(lang)
	if err != nil {
		t.Fatalf("failed to get %s provider: %v", lang, err)
	}
	return p
}

// requiredControls are security controls specific templates must keep. A deny
// pattern catches a known-bad line coming back; these catch the control itself
// being deleted, which no deny pattern can see.
var requiredControls = []struct {
	file    string
	pattern *regexp.Regexp
	reason  string
}{
	{
		file:    "templates/capabilities/http-api/files/adapter/httphandler/handler.go.tmpl",
		pattern: regexp.MustCompile(`http\.MaxBytesReader\(`),
		reason:  "decodeJSON must bound the request body, or one request can allocate without limit",
	},
	{
		file:    "templates/capabilities/rate-limiting/files/adapter/httphandler/middleware_ratelimit.go.tmpl",
		pattern: regexp.MustCompile(`<-ctx\.Done\(\)`),
		reason:  "the visitor cleanup goroutine must stop when its context ends",
	},
	{
		file:    "templates/capabilities/rate-limiting/files/adapter/httphandler/middleware_ratelimit.go.tmpl",
		pattern: regexp.MustCompile(`len\(l\.visitors\) >= maxRateLimitVisitors`),
		reason:  "the visitors map must be capped, or distinct client addresses grow it without bound",
	},
	{
		file:    "templates/capabilities/idempotency/files/adapter/httphandler/middleware_idempotency.go.tmpl",
		pattern: regexp.MustCompile(`idempotencyStorageKey\(\w+, r\.Method, r\.URL\.Path, key\)`),
		reason:  "the idempotency cache key must include the caller's scope, the method and the path",
	},
	{
		file:    "templates/capabilities/idempotency/files/adapter/httphandler/middleware_idempotency.go.tmpl",
		pattern: regexp.MustCompile(`maxIdempotentResponseBytes`),
		reason:  "buffered responses must be capped",
	},
	{
		file:    "templates/capabilities/idempotency/files/adapter/httphandler/middleware_idempotency.go.tmpl",
		pattern: regexp.MustCompile(`delete\(s\.entries, k\)`),
		reason:  "the memory store must evict expired entries",
	},
	{
		file:    "templates/capabilities/oauth2/files/adapter/httphandler/oauth2.go.tmpl",
		pattern: regexp.MustCompile(`subtle\.ConstantTimeCompare\(`),
		reason:  "the callback must compare state with the initiating browser's cookie in constant time",
	},
	{
		file:    "templates/capabilities/oauth2/files/adapter/httphandler/oauth2.go.tmpl",
		pattern: regexp.MustCompile(`HttpOnly:\s+true,\s+Secure:\s+true,\s+SameSite:\s+http\.SameSiteLaxMode`),
		reason:  "the state cookie must be HttpOnly, Secure and SameSite=Lax",
	},
	{
		file:    "templates/capabilities/oauth2/files/adapter/httphandler/oauth2.go.tmpl",
		pattern: regexp.MustCompile(`"Cache-Control", "no-store"`),
		reason:  "a response carrying tokens must not be cached",
	},
	{
		file:    "templates/capabilities/oauth2/files/adapter/httphandler/oauth2.go.tmpl",
		pattern: regexp.MustCompile(`func \(f \*OAuth2Flow\) Login\(`),
		reason:  "the flow needs a start handler that issues state, or the callback can never succeed",
	},
	{
		file:    "templates/capabilities/cors/files/adapter/httphandler/middleware_cors.go.tmpl",
		pattern: regexp.MustCompile(`\.Add\("Vary", "Origin"\)`),
		reason:  "responses that depend on Origin must say so, or a shared cache serves one origin's headers to another",
	},
	{
		file:    "templates/capabilities/cors/files/adapter/httphandler/middleware_cors.go.tmpl",
		pattern: regexp.MustCompile(`AllowCredentials cannot be combined with a wildcard origin`),
		reason:  "credentials with a wildcard origin must be refused",
	},
	{
		file:    "templates/capabilities/multi-tenancy/files/adapter/httphandler/middleware/tenant.go.tmpl",
		pattern: regexp.MustCompile(`resolver\.Resolve\(r\.Context\(\), r\)`),
		reason:  "the tenant middleware must resolve the tenant through the TenantResolver",
	},
	{
		file:    "templates/capabilities/multi-tenancy/files/platform/tenant/resolver.go.tmpl",
		pattern: regexp.MustCompile(`ErrResolverNotConfigured`),
		reason:  "the default resolver must fail closed until it is implemented",
	},
}

func TestTemplates_KeepRequiredSecurityControls(t *testing.T) {
	tmplFS := mustGetProvider(t, "go").GetTemplateFS()
	for _, rc := range requiredControls {
		t.Run(strings.Split(rc.file, "/")[2]+"/"+rc.pattern.String(), func(t *testing.T) {
			data, err := fs.ReadFile(tmplFS, rc.file)
			if err != nil {
				t.Fatalf("reading %s: %v", rc.file, err)
			}
			if !rc.pattern.Match(data) {
				t.Errorf("[SECURITY] %s lacks %s: %s", rc.file, rc.pattern, rc.reason)
			}
		})
	}
}
