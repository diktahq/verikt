package golang

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// SEC-F14: the dev compose file runs databases and mail with fixed passwords or
// no auth at all. Published on every interface, they are reachable from the
// whole network the laptop is on, so every published port binds loopback.
func TestRenderedComposePublishesPortsOnLoopbackOnly(t *testing.T) {
	svc := scaffoldArch(t, "hexagonal", "http-api,grpc,docker,mysql,postgres,redis,kafka-consumer,mailpit")
	var compose struct {
		Services map[string]struct {
			Ports []string `yaml:"ports"`
		} `yaml:"services"`
	}
	readYAML(t, filepath.Join(svc, "docker-compose.yml"), &compose)

	published := 0
	for name, s := range compose.Services {
		for _, p := range s.Ports {
			published++
			if !strings.HasPrefix(p, "127.0.0.1:") {
				t.Errorf("service %s publishes %q on every interface; bind it to 127.0.0.1", name, p)
			}
		}
	}
	if published < 7 {
		t.Fatalf("expected the app, mysql, postgres, redis, kafka and mailpit ports; found %d", published)
	}
}

// SEC-F14 / INV-003: every image the GitLab pipeline pulls, services included,
// carries a version tag. `docker:dind` without one floats to whatever is newest.
func TestRenderedGitLabCIPinsEveryImage(t *testing.T) {
	svc := scaffoldArch(t, "hexagonal", "http-api,docker,ci-gitlab")
	var pipeline map[string]any
	readYAML(t, filepath.Join(svc, ".gitlab-ci.yml"), &pipeline)

	images := gitlabImages(pipeline)
	if len(images) < 3 {
		t.Fatalf("expected the default, docker job and dind service images; found %v", images)
	}
	for _, img := range images {
		// A tag must name a version: "dind" and "latest" are tags that float.
		_, tag, _ := strings.Cut(img, ":")
		if !versionTagRE.MatchString(tag) {
			t.Errorf("image %q is not pinned to a version tag", img)
		}
	}
}

var versionTagRE = regexp.MustCompile(`^v?[0-9]`)

// gitlabImages collects the top-level image and each job's image and services.
func gitlabImages(pipeline map[string]any) []string {
	var images []string
	if img, ok := pipeline["image"].(string); ok {
		images = append(images, img)
	}
	for _, v := range pipeline {
		job, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if img, ok := job["image"].(string); ok {
			images = append(images, img)
		}
		services, _ := job["services"].([]any)
		for _, s := range services {
			if img, ok := s.(string); ok {
				images = append(images, img)
			}
		}
	}
	return images
}

func readYAML(t *testing.T, path string, into any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, into); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

// SEC-F16: a line break in a header value ends the header and lets the caller
// add others (Bcc, a second body). net/smtp already refuses one in the envelope
// from and to; the subject reaches the message unchecked. All three are refused
// before any connection is made.
func TestRenderedMailpitProviderRefusesHeaderInjection(t *testing.T) {
	svc := scaffoldArch(t, "hexagonal", "mailpit")
	renderedPackageTest(t, svc, []string{"adapter/mailpit/provider.go"}, stdlibOnlyGoMod, mailpitBehaviourTest)
}

const mailpitBehaviourTest = `import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
)

func TestSendRefusesLineBreaksInHeaders(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var dialled atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			dialled.Add(1)
			_ = c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cases := []struct{ name, from, to, subject string }{
		{"subject", "a@localhost", "b@localhost", "hi\r\nBcc: evil@example.com"},
		{"subject LF", "a@localhost", "b@localhost", "hi\nBcc: evil@example.com"},
		{"to", "a@localhost", "b@localhost\r\nBcc: evil@example.com", "hi"},
		{"from", "a@localhost\r\nBcc: evil@example.com", "b@localhost", "hi"},
	}
	for _, tc := range cases {
		p := NewProvider(ProviderConfig{Host: "127.0.0.1", Port: port, From: tc.from}, logger)
		if err := p.Send(context.Background(), tc.to, tc.subject, "<p>x</p>"); err == nil {
			t.Errorf("%s: Send accepted a header value with a line break", tc.name)
		}
	}
	if n := dialled.Load(); n != 0 {
		t.Fatalf("Send connected to the SMTP server %d times with an invalid header", n)
	}
}
`
