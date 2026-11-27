package golang

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// These tests scaffold a whole project with the HTTP security capabilities,
// drop behavioural tests into its handler packages and run them there. The
// handler package depends on chi, the tenant package and the rest of the
// project, so unlike renderedPackageTest it can't be lifted into a bare module:
// it is tested where it was generated, after `go mod tidy`.

// httpSecurityCaps are the capabilities whose secure defaults these tests pin.
const httpSecurityCaps = "http-api,rate-limiting,idempotency,multi-tenancy,oauth2,cors,auth-jwt,platform"

// httpHandlerPkgs maps each architecture to where it puts the HTTP handler
// package and that package's name; the tenant middleware lives in its
// middleware subpackage.
var httpHandlerPkgs = map[string]struct{ dir, name string }{
	"flat":      {"handler", "handler"},
	"hexagonal": {filepath.Join("adapter", "httphandler"), "httphandler"},
}

// TestRenderedHTTPSecurityDefaults covers INV-003 for http-api, rate-limiting,
// idempotency, multi-tenancy, oauth2 and cors: spoofed forwarding headers,
// per-principal idempotency, exact-origin CORS, a bounded JSON body, an OAuth2
// state bound to the browser, and a tenant taken only from the resolver.
func TestRenderedHTTPSecurityDefaults(t *testing.T) {
	if os.Getenv(buildScaffoldsEnvForTemplates) != "1" {
		t.Skipf("downloads chi, jwx and otel; set %s=1", buildScaffoldsEnvForTemplates)
	}
	for arch, pkg := range httpHandlerPkgs {
		t.Run(arch, func(t *testing.T) {
			svc := scaffoldArch(t, arch, httpSecurityCaps)
			scaffoldedPackageTest(t, svc, map[string]string{
				filepath.Join(pkg.dir, "behaviour_test.go"):               "package " + pkg.name + "\n\n" + httpHandlerBehaviourTest,
				filepath.Join(pkg.dir, "middleware", "behaviour_test.go"): tenantBehaviourTest,
			})
		})
	}
}

// scaffoldedPackageTest writes each test file into the scaffolded project svc,
// tidies its module and runs `go test` on the packages those files are in.
func scaffoldedPackageTest(t *testing.T, svc string, tests map[string]string) {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	args := make([]string, 0, 2+len(tests))
	args = append(args, "test", "-count=1")
	for rel, src := range tests {
		writeTestFile(t, filepath.Join(svc, rel), src)
		args = append(args, "./"+filepath.ToSlash(filepath.Dir(rel)))
	}
	runGo(t, svc, goBin, "mod", "tidy")
	runGo(t, svc, goBin, args...)
}

func runGo(t *testing.T, dir, goBin string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %v in the scaffolded project failed: %v\n%s", args, err, out)
	}
}

// httpHandlerBehaviourTest has no package clause: the flat and hexagonal
// layouts name the handler package differently, so the caller adds it.
const httpHandlerBehaviourTest = `import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestRouterIgnoresSpoofedForwardingHeaders(t *testing.T) {
	var seen string
	r := NewRouter(discardLogger(), DefaultTimeouts(), func(r chi.Router) {
		r.Get("/ip", func(_ http.ResponseWriter, r *http.Request) { seen = r.RemoteAddr })
	})
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	req.Header.Set("X-Forwarded-For", "10.9.9.9")
	req.Header.Set("X-Real-IP", "10.8.8.8")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "203.0.113.7:1234" {
		t.Fatalf("RemoteAddr rewritten from a client header to %q", seen)
	}
}

func TestRateLimiterNotBypassedByForwardedFor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := RateLimiter(ctx, 1, 1, discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	var codes []int
	for _, xff := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		req.Header.Set("X-Forwarded-For", xff)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}
	if codes[1] != http.StatusTooManyRequests || codes[2] != http.StatusTooManyRequests {
		t.Fatalf("a fresh X-Forwarded-For per request bypassed the limit: codes %v", codes)
	}
}

// idempotencyHarness scopes keys by an X-User header, standing in for the
// verified identity a real scope function reads from the auth context.
type idempotencyHarness struct {
	h    http.Handler
	runs int
}

func newIdempotencyHarness() *idempotencyHarness {
	ih := &idempotencyHarness{}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ih.runs++
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, r.Header.Get("X-User")+"-secret")
	})
	scope := func(r *http.Request) string { return r.Header.Get("X-User") }
	ih.h = Idempotency(NewMemoryIdempotencyStore(), scope, discardLogger())(next)
	return ih
}

func (ih *idempotencyHarness) do(user, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("Idempotency-Key", "order-100")
	if user != "" {
		req.Header.Set("X-User", user)
	}
	rec := httptest.NewRecorder()
	ih.h.ServeHTTP(rec, req)
	return rec
}

func replayed(rec *httptest.ResponseRecorder) bool {
	return rec.Header().Get("X-Idempotent-Replayed") == "true"
}

func TestIdempotencyReplaysForTheSamePrincipal(t *testing.T) {
	ih := newIdempotencyHarness()
	ih.do("alice", "/orders")
	rec := ih.do("alice", "/orders")
	if !replayed(rec) || rec.Body.String() != "alice-secret" || ih.runs != 1 {
		t.Fatalf("same principal and key: body %q replayed %v runs %d", rec.Body, replayed(rec), ih.runs)
	}
}

func TestIdempotencyIsolatesPrincipalsSharingAKey(t *testing.T) {
	ih := newIdempotencyHarness()
	ih.do("alice", "/orders")
	rec := ih.do("bob", "/orders")
	if replayed(rec) || rec.Body.String() != "bob-secret" {
		t.Fatalf("bob with alice's key got %q (replayed %v)", rec.Body, replayed(rec))
	}
}

func TestIdempotencyDoesNotReplayAcrossPaths(t *testing.T) {
	ih := newIdempotencyHarness()
	ih.do("alice", "/orders")
	if rec := ih.do("alice", "/refunds"); replayed(rec) || ih.runs != 2 {
		t.Fatalf("a different path was replayed: replayed %v runs %d", replayed(rec), ih.runs)
	}
}

func TestIdempotencyKeyedAnonymousRequestIsUnauthorized(t *testing.T) {
	ih := newIdempotencyHarness()
	if rec := ih.do("", "/orders"); rec.Code != http.StatusUnauthorized || ih.runs != 0 {
		t.Fatalf("anonymous keyed request: code %d runs %d, want 401 and no run", rec.Code, ih.runs)
	}
}

func TestIdempotencyDoesNotCacheOversizedResponse(t *testing.T) {
	runs := 0
	big := strings.Repeat("x", maxIdempotentResponseBytes+1)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { runs++; io.WriteString(w, big) })
	h := Idempotency(NewMemoryIdempotencyStore(), func(*http.Request) string { return "a" }, discardLogger())(next)
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		req.Header.Set("Idempotency-Key", "k")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Body.Len() != len(big) {
			t.Fatalf("response truncated to %d bytes", rec.Body.Len())
		}
	}
	if runs != 2 {
		t.Fatalf("an oversized response was cached and replayed: handler ran %d times", runs)
	}
}

// TestIdempotencyStoreAtCapacitySweepsOnlyOnTheInterval fills the store with
// entries that then expire. Before the sweep interval has passed a Set must
// not scan the map, so the store still reports itself full; after it, the
// sweep frees the space.
func TestIdempotencyStoreAtCapacitySweepsOnlyOnTheInterval(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	s := NewMemoryIdempotencyStore().(*memoryStore)
	s.now = func() time.Time { return now }
	ctx := context.Background()
	for i := 0; i < maxIdempotencyEntries; i++ {
		if err := s.Set(ctx, strconv.Itoa(i), nil, time.Second); err != nil {
			t.Fatalf("filling entry %d: %v", i, err)
		}
	}
	now = now.Add(2 * time.Second)
	if err := s.Set(ctx, "new", nil, time.Hour); !errors.Is(err, ErrIdempotencyStoreFull) {
		t.Fatalf("Set at capacity swept before the interval: err %v", err)
	}
	now = now.Add(idempotencySweepInterval)
	if err := s.Set(ctx, "new", nil, time.Hour); err != nil {
		t.Fatalf("Set after the sweep interval: %v", err)
	}
}

func corsOrigin(t *testing.T, origin string) http.Header {
	t.Helper()
	h := CORS(CORSConfig{AllowedOrigins: []string{"https://a.example", "https://b.example"}, AllowedMethods: []string{http.MethodGet}})(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Header()
}

func TestCORSEchoesOnlyTheExactMatchedOrigin(t *testing.T) {
	cases := map[string]string{
		"https://b.example":      "https://b.example",
		"https://evil.example":   "",
		"https://a.example.evil": "",
		"https://xa.example":     "",
	}
	for origin, want := range cases {
		if got := corsOrigin(t, origin).Get("Access-Control-Allow-Origin"); got != want {
			t.Errorf("Origin %s: Access-Control-Allow-Origin %q, want %q", origin, got, want)
		}
	}
}

func TestCORSVariesOnOrigin(t *testing.T) {
	for _, origin := range []string{"https://a.example", "https://evil.example"} {
		if got := corsOrigin(t, origin).Values("Vary"); !strings.Contains(strings.Join(got, ","), "Origin") {
			t.Errorf("Origin %s: Vary %v lacks Origin; a shared cache could serve one origin's answer to another", origin, got)
		}
	}
}

func TestCORSPanicsOnWildcardWithCredentials(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("CORS accepted AllowedOrigins * with AllowCredentials")
		}
	}()
	CORS(CORSConfig{AllowedOrigins: []string{"*"}, AllowCredentials: true})
}

func TestDecodeJSONRejectsBodyOverTheLimit(t *testing.T) {
	body := ` + "`" + `{"a":"` + "`" + ` + strings.Repeat("x", maxJSONBodyBytes) + ` + "`" + `"}` + "`" + `
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	var dst map[string]string
	err := decodeJSON(httptest.NewRecorder(), req, &dst)
	var tooBig *http.MaxBytesError
	if !errors.As(err, &tooBig) {
		t.Fatalf("decodeJSON over the limit returned %v, want *http.MaxBytesError", err)
	}
}

// newOAuthFlow returns a flow whose IdP counts code exchanges.
func newOAuthFlow(t *testing.T) (*OAuth2Flow, *int) {
	t.Helper()
	exchanged := new(int)
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*exchanged++
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, ` + "`" + `{"access_token":"t","token_type":"bearer"}` + "`" + `)
	}))
	t.Cleanup(idp.Close)
	return NewOAuth2Flow(OAuth2Config{ClientID: "c", AuthURL: "https://idp.example/authorize", TokenURL: idp.URL, RedirectURL: "https://app.example/cb"}), exchanged
}

// login starts a login and returns the state cookie it set and the state from
// the redirect.
func login(t *testing.T, flow *OAuth2Flow) (*http.Cookie, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	flow.Login(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rec.Code != http.StatusFound || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("login: code %d, cookies %v", rec.Code, rec.Result().Cookies())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return rec.Result().Cookies()[0], loc.Query().Get("state")
}

// oauthFlow starts a login and returns the flow, the state cookie it set, the
// state from the redirect, and how many code exchanges the IdP has seen.
func oauthFlow(t *testing.T) (*OAuth2Flow, *http.Cookie, string, *int) {
	t.Helper()
	flow, exchanged := newOAuthFlow(t)
	cookie, state := login(t, flow)
	return flow, cookie, state, exchanged
}

// floodLogins starts n anonymous logins that never come back.
func floodLogins(t *testing.T, flow *OAuth2Flow, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		rec := httptest.NewRecorder()
		flow.Login(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
		if rec.Code != http.StatusFound {
			t.Fatalf("login %d of a flood: code %d", i, rec.Code)
		}
	}
}

func callback(flow *OAuth2Flow, state string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/cb?code=x&state="+url.QueryEscape(state), nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	flow.Callback(rec, req)
	return rec
}

func TestOAuth2StateCookieIsLocked(t *testing.T) {
	_, cookie, _, _ := oauthFlow(t)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("state cookie not HttpOnly, Secure and SameSite=Lax: %+v", cookie)
	}
}

func TestOAuth2CallbackWithoutStateCookieIsRejected(t *testing.T) {
	flow, _, state, exchanged := oauthFlow(t)
	if rec := callback(flow, state, nil); rec.Code != http.StatusBadRequest || *exchanged != 0 {
		t.Fatalf("callback without the cookie: code %d, exchanges %d", rec.Code, *exchanged)
	}
}

func TestOAuth2CallbackWithWrongStateCookieIsRejected(t *testing.T) {
	flow, cookie, state, exchanged := oauthFlow(t)
	wrong := &http.Cookie{Name: cookie.Name, Value: "deadbeef"}
	if rec := callback(flow, state, wrong); rec.Code != http.StatusBadRequest || *exchanged != 0 {
		t.Fatalf("callback with another cookie: code %d, exchanges %d", rec.Code, *exchanged)
	}
}

func TestOAuth2CallbackFromTheLoginBrowserIsAccepted(t *testing.T) {
	flow, cookie, state, _ := oauthFlow(t)
	rec := callback(flow, state, cookie)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("legitimate callback: code %d, Cache-Control %q, body %s", rec.Code, rec.Header().Get("Cache-Control"), rec.Body)
	}
}

func TestOAuth2StateIsSingleUse(t *testing.T) {
	flow, cookie, state, exchanged := oauthFlow(t)
	callback(flow, state, cookie)
	if rec := callback(flow, state, cookie); rec.Code != http.StatusBadRequest || *exchanged != 1 {
		t.Fatalf("replayed callback: code %d, exchanges %d", rec.Code, *exchanged)
	}
}

func TestOAuth2LoginFloodDoesNotLockOutLogins(t *testing.T) {
	flow, _ := newOAuthFlow(t)
	floodLogins(t, flow, maxPendingOAuth2States+1)
	cookie, state := login(t, flow)
	if rec := callback(flow, state, cookie); rec.Code != http.StatusOK {
		t.Fatalf("login after a flood: callback code %d, body %s", rec.Code, rec.Body)
	}
}

// A flood can still push out a login in progress; that user retries.
func TestOAuth2LoginPushedOutByAFloodCanRetry(t *testing.T) {
	flow, cookie, state, _ := oauthFlow(t)
	floodLogins(t, flow, maxPendingOAuth2States)
	if rec := callback(flow, state, cookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("evicted state: callback code %d, want 400", rec.Code)
	}
	cookie, state = login(t, flow)
	if rec := callback(flow, state, cookie); rec.Code != http.StatusOK {
		t.Fatalf("retried login: callback code %d, body %s", rec.Code, rec.Body)
	}
}
`

const tenantBehaviourTest = `package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/svc/platform/tenant"
)

func serveTenant(t *testing.T, resolver tenant.TenantResolver, next http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "victim")
	rec := httptest.NewRecorder()
	Tenant(resolver)(next).ServeHTTP(rec, req)
	return rec
}

func mustNotRun(t *testing.T) http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the handler ran without a resolved tenant")
	})
}

func TestTenantWithDefaultResolverFailsClosed(t *testing.T) {
	if rec := serveTenant(t, tenant.NotConfigured, mustNotRun(t)); rec.Code != http.StatusInternalServerError {
		t.Fatalf("NotConfigured resolver: code %d, want 500", rec.Code)
	}
}

func TestTenantUnresolvedIsForbidden(t *testing.T) {
	unresolved := tenant.ResolverFunc(func(context.Context, *http.Request) (string, error) {
		return "", errors.New("no tenant claim")
	})
	if rec := serveTenant(t, unresolved, mustNotRun(t)); rec.Code != http.StatusForbidden {
		t.Fatalf("unresolved tenant: code %d, want 403", rec.Code)
	}
}

func TestTenantFromResolverNotHeader(t *testing.T) {
	var got string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = tenant.TenantFromContext(r.Context())
	})
	resolved := tenant.ResolverFunc(func(context.Context, *http.Request) (string, error) { return "acme", nil })
	serveTenant(t, resolved, next)
	if got != "acme" {
		t.Fatalf("tenant in context = %q, want the resolver's %q (X-Tenant-ID was %q)", got, "acme", "victim")
	}
}
`
