package golang

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// These tests render a capability's file, copy it into a throwaway module next
// to a behavioural test written here, and run `go test` on it. The rendered
// file is what users get, so this checks the generated code's behaviour, not
// the template's text.

// renderedPackageTest copies the rendered files from a scaffolded project into a
// fresh module, adds testSrc as an in-package test, and runs it. goMod is the
// module's go.mod; a module with requirements needs network or a warm cache.
func renderedPackageTest(t *testing.T, svc string, rendered []string, goMod, testSrc string) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs the go toolchain")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}

	mod := t.TempDir()
	writeTestFile(t, filepath.Join(mod, "go.mod"), goMod)
	pkg := ""
	for _, rel := range rendered {
		data, err := os.ReadFile(filepath.Join(svc, rel))
		if err != nil {
			t.Fatalf("read rendered %s: %v", rel, err)
		}
		pkg = renderedPackageName(t, string(data))
		writeTestFile(t, filepath.Join(mod, filepath.Base(rel)), string(data))
	}
	writeTestFile(t, filepath.Join(mod, "behaviour_test.go"), "package "+pkg+"\n\n"+testSrc)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goBin, "test", "-count=1", "./...")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test on the rendered %v failed: %v\n%s", rendered, err, out)
	}
}

var renderedPackageRE = regexp.MustCompile(`(?m)^package (\w+)`)

func renderedPackageName(t *testing.T, src string) string {
	t.Helper()
	m := renderedPackageRE.FindStringSubmatch(src)
	if m == nil {
		t.Fatal("rendered file has no package clause")
	}
	return m[1]
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const stdlibOnlyGoMod = "module example.com/rendered\n\ngo 1.22\n"

// SEC-F8: an event name with a line break would end the event early and let the
// caller's string inject fields; multi-line data must become one data: line per
// line, as the WHATWG event-stream format requires.
func TestRenderedSSEEventWriterFramesEventsSafely(t *testing.T) {
	svc := scaffoldArch(t, "hexagonal", "http-api,sse")
	renderedPackageTest(t, svc, []string{"adapter/httphandler/sse.go"}, stdlibOnlyGoMod, sseBehaviourTest)
}

const sseBehaviourTest = `import (
	"net/http/httptest"
	"testing"
)

func TestSendRejectsLineBreaksInEventName(t *testing.T) {
	for _, name := range []string{"a\nb", "a\rb", "a\r\nb"} {
		rec := httptest.NewRecorder()
		ew, err := NewEventWriter(rec)
		if err != nil {
			t.Fatal(err)
		}
		before := rec.Body.Len()
		if err := ew.Send(name, "x"); err == nil {
			t.Errorf("Send(%q) accepted an event name with a line break", name)
		}
		if rec.Body.Len() != before {
			t.Errorf("Send(%q) wrote %q despite rejecting it", name, rec.Body.String()[before:])
		}
	}
}

func TestSendSplitsMultiLineData(t *testing.T) {
	rec := httptest.NewRecorder()
	ew, err := NewEventWriter(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := ew.Send("update", "one\ntwo\r\nthree\rfour"); err != nil {
		t.Fatal(err)
	}
	want := "event: update\ndata: one\ndata: two\ndata: three\ndata: four\n\n"
	if got := rec.Body.String(); got != want {
		t.Fatalf("stream = %q, want %q", got, want)
	}
}

func TestSendSingleLineDataUnchanged(t *testing.T) {
	rec := httptest.NewRecorder()
	ew, err := NewEventWriter(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := ew.Send("ping", "{\"ok\":true}"); err != nil {
		t.Fatal(err)
	}
	if got, want := rec.Body.String(), "event: ping\ndata: {\"ok\":true}\n\n"; got != want {
		t.Fatalf("stream = %q, want %q", got, want)
	}
}
`

// SEC-F7: the hub must bound message size, must not let a client choose its
// connection key (two clients sending the same X-Request-ID evicted each
// other), and must not hold the hub lock while writing to a slow client.
//
// gorilla/websocket is downloaded into the throwaway module, so this runs only
// where scaffold builds run.
func TestRenderedWebSocketHubIsBounded(t *testing.T) {
	if os.Getenv(buildScaffoldsEnvForTemplates) != "1" {
		t.Skipf("downloads gorilla/websocket; set %s=1", buildScaffoldsEnvForTemplates)
	}
	svc := scaffoldArch(t, "hexagonal", "http-api,websocket")
	goMod := stdlibOnlyGoMod + "\nrequire github.com/gorilla/websocket v1.5.3\n"
	renderedPackageTest(t, svc, []string{"adapter/httphandler/websocket.go"}, goMod, wsBehaviourTest)
}

// buildScaffoldsEnvForTemplates matches internal/cli's VERIKT_BUILD_SCAFFOLDS:
// the same opt-in for tests that need module downloads.
const buildScaffoldsEnvForTemplates = "VERIKT_BUILD_SCAFFOLDS"

const wsBehaviourTest = `import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func newHubServer(t *testing.T) (*WSHub, string) {
	t.Helper()
	hub := NewWSHub(slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	t.Cleanup(srv.Close)
	return hub, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func dial(t *testing.T, url string, h http.Header) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(url, h)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// eventually runs cond in a goroutine so a deadlocked hub fails the test
// instead of hanging it.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		done := make(chan bool, 1)
		go func() { done <- cond() }()
		select {
		case ok := <-done:
			if ok {
				return
			}
		case <-time.After(time.Until(deadline)):
			t.Fatalf("%s: blocked", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s: not reached", what)
}

func TestOversizedMessageClosesTheConnection(t *testing.T) {
	_, url := newHubServer(t)
	c := dial(t, url, nil)
	// The server may close before the whole message is sent, so a write error
	// is expected; the close frame is still there to read.
	_ = c.WriteMessage(websocket.TextMessage, make([]byte, 1<<20))
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err := c.ReadMessage()
	if !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
		t.Fatalf("a 1 MiB message was not refused with 1009; read returned %v", err)
	}
}

func TestClientsSharingARequestIDDoNotEvictEachOther(t *testing.T) {
	hub, url := newHubServer(t)
	h := http.Header{"X-Request-Id": []string{"same"}}
	dial(t, url, h)
	dial(t, url, h)
	eventually(t, "two connections registered", func() bool { return hub.Count() == 2 })
}

func TestBroadcastDoesNotHoldTheHubLockWhileWriting(t *testing.T) {
	hub, url := newHubServer(t)
	dial(t, url, nil)
	eventually(t, "first connection registered", func() bool { return hub.Count() == 1 })

	// Stall every write, as a client that stopped reading would.
	hub.mu.RLock()
	var stalled []*WSConn
	for _, ws := range hub.conns {
		stalled = append(stalled, ws)
	}
	hub.mu.RUnlock()
	for _, ws := range stalled {
		ws.writeMu.Lock()
	}
	defer func() {
		for _, ws := range stalled {
			ws.writeMu.Unlock()
		}
	}()

	go hub.Broadcast(t.Context(), []byte("hello"))
	time.Sleep(50 * time.Millisecond)

	dial(t, url, nil)
	eventually(t, "a new connection registers during a stalled broadcast", func() bool { return hub.Count() == 2 })
}
`

// SEC-F9: the BFF client must not follow a backend's redirect to another host,
// and must refuse a path that would change the host it is appended to. The
// tests ship in the template's httpclient_test.go and are run by
// TestBFFGatewayPackageTestsPassUnderRace; this checks they are there.
func TestRenderedBFFClientShipsRedirectAndPathTests(t *testing.T) {
	svc := scaffoldArch(t, "hexagonal", "http-api,bff")
	data, err := os.ReadFile(filepath.Join(svc, "adapter", "bffgateway", "httpclient_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TestCallDoesNotFollowRedirects", "TestCallRejectsPathWithoutSingleLeadingSlash"} {
		if !strings.Contains(string(data), "func "+name+"(") {
			t.Errorf("rendered bff httpclient_test.go is missing %s", name)
		}
	}
}
