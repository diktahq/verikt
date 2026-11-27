package golang

import (
	"bufio"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/scaffold"
)

// The capability registry is the embedded template FS: `verikt new` and
// `verikt add` offer every directory under templates/capabilities/ that has a
// capability.yaml. These tests keep three things in lockstep with it: the
// capabilityMeta display table in matrix.go, the reference page in docs/, and
// the website copy of that page.

var (
	capabilitiesMatrixDoc = filepath.Join("..", "..", "docs", "reference", "capabilities-matrix.md")
	capabilitiesMatrixWeb = filepath.Join("..", "..", "website", "src", "content", "docs", "reference", "capabilities-matrix.mdx")
	typescriptCapsDir     = filepath.Join("..", "typescript", "templates", "capabilities")
)

// discoveredTemplates returns the names of the template directories under
// templates/<kind> in the embedded FS that carry the given manifest file, the
// same discovery `verikt new` and `verikt add` use.
//
// Dot-directories are skipped. The embed uses the all: prefix, so a stray
// directory such as templates/capabilities/.claude (left behind by agent
// tooling in a working tree, never committed) would otherwise be read as a
// capability called ".claude".
func discoveredTemplates(t *testing.T, kind, manifest string) []string {
	t.Helper()
	dir := path.Join("templates", kind)
	entries, err := fs.ReadDir(templatesFS, dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		data, err := fs.ReadFile(templatesFS, path.Join(dir, e.Name(), manifest))
		if err != nil {
			continue // not offered by verikt new/add either
		}
		if kind == "capabilities" {
			cm, err := scaffold.ParseCapabilityManifest(data)
			if err != nil {
				t.Fatalf("parse %s/%s/%s: %v", dir, e.Name(), manifest, err)
			}
			if cm.Name != e.Name() {
				t.Errorf("capability directory %q declares name %q; they must match", e.Name(), cm.Name)
			}
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		t.Fatalf("no %s found under %s", kind, dir)
	}
	slices.Sort(names)
	return names
}

var docRowName = regexp.MustCompile("^\\|\\s*`([a-z0-9-]+)`\\s*\\|")

// documentedNames returns the backticked first-column names of every table row
// under the "## <section>" heading, up to the next level-2 heading. Rows in any
// other section (suggestion tables, a future "## Planned" section) do not count.
func documentedNames(t *testing.T, doc, section string) []string {
	t.Helper()
	var (
		names []string
		in    bool
		seen  = map[string]bool{}
	)
	sc := bufio.NewScanner(strings.NewReader(doc))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "## ") {
			in = strings.TrimSpace(strings.TrimPrefix(line, "## ")) == section
			continue
		}
		if !in {
			continue
		}
		m := docRowName.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if seen[m[1]] {
			t.Errorf("%s lists %q more than once in section %q", capabilitiesMatrixDoc, m[1], section)
			continue
		}
		seen[m[1]] = true
		names = append(names, m[1])
	}
	if len(names) == 0 {
		t.Fatalf("%s: no rows found in section %q; did the heading or table format change?", capabilitiesMatrixDoc, section)
	}
	slices.Sort(names)
	return names
}

// missingFrom returns the elements of a that are not in b.
func missingFrom(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

func assertSameSet(t *testing.T, what, source string, want, got []string) {
	t.Helper()
	if m := missingFrom(want, got); len(m) > 0 {
		t.Errorf("%s with a template but missing from %s: %s", what, source, strings.Join(m, ", "))
	}
	if m := missingFrom(got, want); len(m) > 0 {
		t.Errorf("%s listed in %s with no template (remove them or move them to a planned section): %s",
			what, source, strings.Join(m, ", "))
	}
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(raw)
}

// TestCapabilitiesMatrixDocParity keeps docs/reference/capabilities-matrix.md
// in lockstep with the Go templates verikt actually scaffolds.
func TestCapabilitiesMatrixDocParity(t *testing.T) {
	doc := readFile(t, capabilitiesMatrixDoc)
	caps := discoveredTemplates(t, "capabilities", "capability.yaml")
	archs := discoveredTemplates(t, "architectures", "manifest.yaml")

	assertSameSet(t, "capabilities", capabilitiesMatrixDoc, caps, documentedNames(t, doc, "Capabilities"))
	assertSameSet(t, "architectures", capabilitiesMatrixDoc, archs, documentedNames(t, doc, "Architectures"))

	counts := regexp.MustCompile(`\*\*(\d+) architectures\*\* and \*\*(\d+) capabilities\*\*`).FindStringSubmatch(doc)
	if counts == nil {
		t.Fatalf("%s: summary \"**N architectures** and **M capabilities**\" not found", capabilitiesMatrixDoc)
	}
	if n, _ := strconv.Atoi(counts[1]); n != len(archs) {
		t.Errorf("%s states %d architectures; templates define %d", capabilitiesMatrixDoc, n, len(archs))
	}
	if n, _ := strconv.Atoi(counts[2]); n != len(caps) {
		t.Errorf("%s states %d capabilities; templates define %d", capabilitiesMatrixDoc, n, len(caps))
	}
}

// webCapRow matches a capability row whose last column is the Languages
// column, e.g. "| `postgres` | ... | Go, TS |". Rows in the conflict and
// suggestion tables end with a reason, not a language list, so they never match.
var webCapRow = regexp.MustCompile("^\\|\\s*`([a-z0-9-]+)`\\s*\\|.*\\|\\s*((?:Go|TS)(?:,\\s*(?:Go|TS))*)\\s*\\|\\s*$")

// webArchRow matches "| **Hexagonal** | ... | Go, TS |" in the architectures table.
var webArchRow = regexp.MustCompile(`^\|\s*\*\*([A-Za-z]+)\*\*\s*\|.*\|\s*((?:Go|TS)(?:,\s*(?:Go|TS))*)\s*\|\s*$`)

// TestCapabilitiesMatrixWebsiteParity keeps the website copy of the matrix in
// lockstep with both providers: a row marked Go must have a Go template, a row
// marked TS must have a TypeScript template, and the stated totals must match.
func TestCapabilitiesMatrixWebsiteParity(t *testing.T) {
	web := readFile(t, capabilitiesMatrixWeb)

	var goCaps, tsCaps, goArchs []string
	sc := bufio.NewScanner(strings.NewReader(web))
	for sc.Scan() {
		line := sc.Text()
		if m := webCapRow.FindStringSubmatch(line); m != nil {
			if slices.Contains(goCaps, m[1]) || slices.Contains(tsCaps, m[1]) {
				t.Errorf("%s lists %q more than once", capabilitiesMatrixWeb, m[1])
				continue
			}
			if strings.Contains(m[2], "Go") {
				goCaps = append(goCaps, m[1])
			}
			if strings.Contains(m[2], "TS") {
				tsCaps = append(tsCaps, m[1])
			}
			continue
		}
		if m := webArchRow.FindStringSubmatch(line); m != nil && strings.Contains(m[2], "Go") {
			goArchs = append(goArchs, strings.ToLower(m[1]))
		}
	}
	slices.Sort(goCaps)
	slices.Sort(tsCaps)
	slices.Sort(goArchs)

	caps := discoveredTemplates(t, "capabilities", "capability.yaml")
	assertSameSet(t, "Go capabilities", capabilitiesMatrixWeb, caps, goCaps)
	assertSameSet(t, "Go architectures", capabilitiesMatrixWeb, discoveredTemplates(t, "architectures", "manifest.yaml"), goArchs)

	// The TypeScript provider is read from disk: its embed lives in another package.
	entries, err := os.ReadDir(typescriptCapsDir)
	if err != nil {
		t.Fatalf("read %s: %v", typescriptCapsDir, err)
	}
	var ts []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") { // same dot-directory rule as above
			continue
		}
		if _, err := os.Stat(filepath.Join(typescriptCapsDir, e.Name(), "capability.yaml")); err == nil {
			ts = append(ts, e.Name())
		}
	}
	slices.Sort(ts)
	assertSameSet(t, "TypeScript capabilities", capabilitiesMatrixWeb, ts, tsCaps)

	if m := regexp.MustCompile(`all (\d+) verikt capabilities`).FindStringSubmatch(web); m == nil {
		t.Errorf("%s: description \"all N verikt capabilities\" not found", capabilitiesMatrixWeb)
	} else if n, _ := strconv.Atoi(m[1]); n != len(caps) {
		t.Errorf("%s description states %d capabilities; Go templates define %d", capabilitiesMatrixWeb, n, len(caps))
	}
	if m := regexp.MustCompile(`supported with (\d+) capabilities`).FindStringSubmatch(web); m == nil {
		t.Errorf("%s: TypeScript note \"supported with N capabilities\" not found", capabilitiesMatrixWeb)
	} else if n, _ := strconv.Atoi(m[1]); n != len(ts) {
		t.Errorf("%s states TypeScript has %d capabilities; TypeScript templates define %d", capabilitiesMatrixWeb, n, len(ts))
	}
}

// TestCapabilityMetaCoversTemplates ensures every architecture and capability
// template has display metadata for the generated docs/PROJECT.md (a missing
// entry silently drops it from that file), and that capabilityMeta names
// nothing that cannot be scaffolded.
func TestCapabilityMetaCoversTemplates(t *testing.T) {
	want := append(
		discoveredTemplates(t, "capabilities", "capability.yaml"),
		discoveredTemplates(t, "architectures", "manifest.yaml")...,
	)
	want = append(want, "go") // language row, always rendered
	slices.Sort(want)

	keys := make([]string, 0, len(capabilityMeta))
	for k := range capabilityMeta {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	assertSameSet(t, "architectures and capabilities", "capabilityMeta in matrix.go", want, keys)
}
