package scaffold

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// A conditional struct field changes the alignment gofmt wants for the whole
// block, which a template cannot know; rendered Go is formatted on the way out.
const conditionalStructTmpl = "package config\n\ntype Config struct {\n\tApp string\n{{- if .Long}}\n\tLongerName string\n{{- end}}\n}\n"

func renderOne(t *testing.T, name, body string, vars map[string]interface{}) string {
	t.Helper()
	fsys := fstest.MapFS{
		"tpl/manifest.yaml": &fstest.MapFile{Data: []byte("name: tpl\ndescription: test\nlanguage: go\nversion: \"1.0\"\n")},
		"tpl/files/" + name: &fstest.MapFile{Data: []byte(body)},
	}
	out := t.TempDir()
	if _, err := NewRenderer(fsys).RenderTemplate("tpl", out, vars); err != nil {
		t.Fatalf("render: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, name[:len(name)-len(".tmpl")]))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRenderFormatsGeneratedGo(t *testing.T) {
	got := renderOne(t, "config.go.tmpl", conditionalStructTmpl, map[string]interface{}{"Long": true})
	want := "package config\n\ntype Config struct {\n\tApp        string\n\tLongerName string\n}\n"
	if got != want {
		t.Errorf("rendered Go is not gofmt-formatted:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderLeavesUnparseableGoUnchanged(t *testing.T) {
	body := "package config\n\nfunc {{.Name}}( {\n"
	got := renderOne(t, "broken.go.tmpl", body, map[string]interface{}{"Name": "x"})
	if want := "package config\n\nfunc x( {\n"; got != want {
		t.Errorf("unparseable Go was changed:\n%q\nwant %q", got, want)
	}
}

func TestRenderDoesNotFormatNonGoFiles(t *testing.T) {
	body := "a:   {{.V}}\n\n\n"
	got := renderOne(t, "config.yaml.tmpl", body, map[string]interface{}{"V": 1})
	if want := "a:   1\n\n\n"; got != want {
		t.Errorf("non-Go file was changed: %q, want %q", got, want)
	}
}
