package scriptguard

import (
	"fmt"
	"sort"
	"testing"
)

// waivedFindings scans src and returns the waived findings as
// "rule@line:reason", sorted, failing on any finding left active.
func waivedFindings(t *testing.T, kind Kind, src string) []string {
	t.Helper()
	res := ScanAll("x", kind, []byte(src), Options{})
	if len(res.Findings) != 0 {
		t.Fatalf("active findings = %+v, want none", res.Findings)
	}
	out := make([]string, 0, len(res.Waived))
	for _, w := range res.Waived {
		if w.File != "x" || w.Message == "" {
			t.Errorf("bad waived finding %+v", w)
		}
		out = append(out, fmt.Sprintf("%s@%d:%s", w.Rule, w.Line, w.Reason))
	}
	sort.Strings(out)
	return out
}

// A verikt:allow marker waives a finding; it does not make it disappear. The
// finding is returned with the marker's reason so verikt check can list it
// under WAIVED, the same as a severity_overrides waiver.
func TestScanAll_InlineMarkerRecordsWaivedFinding(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		src  string
		want []string
	}{
		{name: "swallow, marker above", kind: KindShell,
			src:  "# verikt:allow silent_ci_swallow reason=\"advisory during lint migration\"\ngolangci-lint run || true\n",
			want: []string{sw + "@2:advisory during lint migration"}},
		{name: "continue-on-error, trailing marker", kind: KindWorkflow,
			src:  "    continue-on-error: true # verikt:allow silent_ci_swallow reason=\"best effort\"\n",
			want: []string{sw + "@1:best effort"}},
		{name: "pipe to shell, escaped quote in reason", kind: KindShell,
			src:  "# verikt:allow mutable_install_url reason=\"vendor's \\\"official\\\" installer\"\ncurl https://x.dev/i | sh\n",
			want: []string{mu + "@2:vendor's \"official\" installer"}},
		{name: "markdown, marker above the fence", kind: KindMarkdown,
			src:  "<!-- verikt:allow mutable_install_url reason=\"quickstart\" -->\n```sh\ncurl https://x.dev/i | sh\n```\n",
			want: []string{mu + "@3:quickstart"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := waivedFindings(t, c.kind, c.src)
			if fmt.Sprint(got) != fmt.Sprint(c.want) {
				t.Fatalf("waived = %v, want %v", got, c.want)
			}
		})
	}
}

// A marker for one rule does not waive the other rule on the same line, and
// a line with nothing to report records no waiver.
func TestScanAll_MarkerWaivesOnlyWhatItNames(t *testing.T) {
	src := "# verikt:allow silent_ci_swallow reason=\"x\"\ncurl https://x.dev/i | sh\necho ok\n"
	res := ScanAll("x", KindShell, []byte(src), Options{})
	if len(res.Waived) != 0 {
		t.Errorf("waived = %+v, want none", res.Waived)
	}
	if len(res.Findings) != 1 || res.Findings[0].Rule != mu {
		t.Errorf("findings = %+v, want one %s", res.Findings, mu)
	}
}

func TestScan_ReturnsOnlyActiveFindings(t *testing.T) {
	src := "# verikt:allow silent_ci_swallow reason=\"x\"\ngo test ./... || true\n"
	if got := Scan("x", KindShell, []byte(src), Options{}); len(got) != 0 {
		t.Fatalf("Scan = %+v, want none", got)
	}
}
