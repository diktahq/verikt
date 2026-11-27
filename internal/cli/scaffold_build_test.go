package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// buildScaffoldsEnv gates TestScaffoldBuilds. The test downloads every module
// the scaffolded projects import, so it cannot run offline and does not run by
// default.
const buildScaffoldsEnv = "VERIKT_BUILD_SCAFFOLDS"

// scaffoldBuildSets are the capability sets every architecture is scaffolded
// with. minimal is the smallest HTTP service; platform-only is platform with no
// transport, whose config template used to declare a variable only transports
// use (issue #24); streaming covers the capabilities that register routes the
// request timeout must not reach (issue #29) next to the ones that wire main.
var scaffoldBuildSets = []struct {
	name string
	caps string
	// streams marks sets whose output gets the streaming test dropped into the
	// HTTP handler package and run.
	streams bool
}{
	{name: "minimal", caps: "http-api"},
	{name: "platform-only", caps: "platform"},
	{name: "streaming", caps: "http-api,sse,websocket,timeout,request-id,platform,bootstrap,observability,mysql", streams: true},
}

// httpHandlerDirs is where each architecture maps adapter/httphandler. It
// mirrors path_mappings in each manifest.yaml.
var httpHandlerDirs = map[string]string{
	"hexagonal": "adapter/httphandler",
	"clean":     "internal/interface/handler",
	"layered":   "internal/handler",
	"flat":      "handler",
}

// TestScaffoldBuilds scaffolds every Go architecture with each capability set
// and runs `go build ./...` and `go vet ./...` on the result (issue #24).
// TestScaffoldPassesCheck runs `verikt check`, which never compiles anything,
// so a project whose main.go called a router that a capability replaced passed
// it. For the streaming set it also runs a test inside the generated handler
// package that holds SSE and WebSocket connections open past every server and
// request timeout (issue #29).
func TestScaffoldBuilds(t *testing.T) {
	if os.Getenv(buildScaffoldsEnv) != "1" {
		t.Skipf("set %s=1 to scaffold every architecture and compile the output (needs Go module downloads)", buildScaffoldsEnv)
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("%s=1 but no go binary is on PATH: %v", buildScaffoldsEnv, err)
	}

	for _, arch := range []string{"hexagonal", "clean", "layered", "flat"} {
		for _, set := range scaffoldBuildSets {
			t.Run(arch+"/"+set.name, func(t *testing.T) {
				tmp := t.TempDir()
				chdir(t, tmp)
				// Hooks are skipped so a failure names the go command that failed
				// rather than surfacing as a scaffold error.
				if out, err := executeCommand(t,
					"new", "svc",
					"--arch", arch,
					"--cap", set.caps,
					"--module", "example.com/svc",
					"--no-wizard",
					"--set", "skip_hooks=true",
				); err != nil {
					t.Fatalf("scaffold %s with %s: %v\n%s", arch, set.caps, err, out)
				}

				svcDir := filepath.Join(tmp, "svc")
				runGo(t, goBin, svcDir, "mod", "tidy")
				runGo(t, goBin, svcDir, "build", "./...")
				runGo(t, goBin, svcDir, "vet", "./...")

				if set.streams {
					handlerDir := filepath.Join(svcDir, filepath.FromSlash(httpHandlerDirs[arch]))
					writeStreamingTest(t, handlerDir)
					runGo(t, goBin, svcDir, "test", "-count=1", "-run", "TestStreaming", "./"+httpHandlerDirs[arch])
				}
			})
		}
	}
}

// runGo runs the go command in dir and fails the test with its output.
func runGo(t *testing.T, goBin, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goBin, args...) //nolint:gosec // G204: go from PATH with fixed arguments
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out.String())
	}
}

var packageClause = regexp.MustCompile(`(?m)^package (\w+)$`)

// writeStreamingTest drops streamingTestSource into the generated HTTP handler
// package, under that package's own name.
func writeStreamingTest(t *testing.T, handlerDir string) {
	t.Helper()
	router, err := os.ReadFile(filepath.Join(handlerDir, "router.go"))
	if err != nil {
		t.Fatalf("read generated router: %v", err)
	}
	m := packageClause.FindSubmatch(router)
	if m == nil {
		t.Fatalf("generated router.go in %s has no package clause", handlerDir)
	}
	src := strings.Replace(streamingTestSource, "package PKG", "package "+string(m[1]), 1)
	if err := os.WriteFile(filepath.Join(handlerDir, "streaming_verikt_test.go"), []byte(src), 0o600); err != nil {
		t.Fatalf("write streaming test: %v", err)
	}
}

// streamingTestSource runs inside a generated project. It serves the generated
// router through the generated http.Server configuration with every timeout cut
// to a fraction of a second, then holds streams open for several times that.
const streamingTestSource = `package PKG

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var shortTimeouts = Timeouts{
	Read:    200 * time.Millisecond,
	Write:   200 * time.Millisecond,
	Request: 150 * time.Millisecond,
}

// streamFor is several times every configured timeout.
const streamFor = 1200 * time.Millisecond

func startServer(t *testing.T, register func(chi.Router)) (*httptest.Server, chi.Router) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(logger, shortTimeouts, register)
	srv := NewHTTPServer("127.0.0.1:0", router, shortTimeouts, logger)
	ts := httptest.NewUnstartedServer(router)
	ts.Config = srv.server
	ts.Start()
	t.Cleanup(ts.Close)
	return ts, router
}

func TestStreamingSSERouteIsRegistered(t *testing.T) {
	ts, _ := startServer(t, func(chi.Router) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /events status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("GET /events Content-Type = %q, want text/event-stream", ct)
	}
}

func TestStreamingSSEOutlivesServerTimeouts(t *testing.T) {
	ts, router := startServer(t, func(chi.Router) {})
	// The generated /events route keeps alive every DefaultSSEKeepAlive, far
	// longer than this test; mount the same handler with a short interval
	// beside it, outside the request-timeout group, as the partial does.
	router.Get("/test/events", NewSSEHandler(50*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/test/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /test/events: %v", err)
	}
	defer resp.Body.Close()

	start := time.Now()
	lines := bufio.NewScanner(resp.Body)
	keepAlives := 0
	for time.Since(start) < streamFor {
		if !lines.Scan() {
			t.Fatalf("stream closed after %v (%d keep-alives), err=%v; every timeout is %v or less",
				time.Since(start).Round(time.Millisecond), keepAlives, lines.Err(), shortTimeouts.Write)
		}
		if strings.HasPrefix(lines.Text(), ":") {
			keepAlives++
		}
	}
	if keepAlives < 5 {
		t.Fatalf("got %d keep-alives in %v, want at least 5", keepAlives, streamFor)
	}
}

func TestStreamingJSONRoutesKeepRequestTimeout(t *testing.T) {
	ctxErr := make(chan error, 1)
	ts, _ := startServer(t, func(r chi.Router) {
		r.Get("/slow", func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
				ctxErr <- r.Context().Err()
			case <-time.After(streamFor):
				ctxErr <- nil
			}
		})
	})
	resp, err := http.Get(ts.URL + "/slow")
	if err == nil {
		resp.Body.Close()
	}
	if got := <-ctxErr; !errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("JSON route context error = %v, want context.DeadlineExceeded after %v", got, shortTimeouts.Request)
	}
}

func TestStreamingWebSocketOutlivesServerTimeouts(t *testing.T) {
	ts, _ := startServer(t, func(chi.Router) {})
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial /ws: %v", err)
	}
	defer conn.Close()

	time.Sleep(streamFor)
	if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("write after %v: %v", streamFor, err)
	}
	// The hub only reads, so an open connection times out here; a closed one
	// reports the close or EOF instead.
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("websocket closed after %v: %v", streamFor, err)
	}
}
`
