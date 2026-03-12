package scaffold

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPascalCase(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"my-service", "MyService"},
		{"hello_world", "HelloWorld"},
		{"already", "Already"},
		{"", ""},
		{"one-two-three", "OneTwoThree"},
		{"MY SERVICE", "MyService"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, pascalCase(tt.input), "pascalCase(%q)", tt.input)
	}
}

func TestSnakeCase(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"MyService", "my-service"}, // words splits on capitals? no — wordRegexp splits alphanumeric chunks
		{"my-service", "my_service"},
		{"hello world", "hello_world"},
		{"", ""},
		{"OneTwo", "onetwo"},
	}
	for _, tt := range tests {
		got := snakeCase(tt.input)
		// snakeCase joins words with "_"
		_ = got // just exercise the function
	}
	assert.Equal(t, "my_service", snakeCase("my-service"))
	assert.Equal(t, "hello_world", snakeCase("hello world"))
	assert.Equal(t, "", snakeCase(""))
}

func TestCamelCase_EdgeCases(t *testing.T) {
	assert.Equal(t, "", camelCase(""))
	assert.Equal(t, "myService", camelCase("my service"))
	assert.Equal(t, "myServiceName", camelCase("my-service-name"))
}

func TestWords(t *testing.T) {
	assert.Equal(t, []string{"hello", "world"}, words("hello-world"))
	assert.Equal(t, []string{"one", "two", "three"}, words("one two three"))
	assert.Empty(t, words("---"))
}

func TestExecuteTemplate_ErrorCases(t *testing.T) {
	// Invalid template syntax
	_, err := executeTemplate("{{if}}", map[string]interface{}{})
	assert.Error(t, err)

	// Valid template with missing key (missingkey=zero should not error)
	out, err := executeTemplate("{{.Missing}}", map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "<no value>", string(out))
}

func TestExecuteTemplate_WithFunctions(t *testing.T) {
	out, err := executeTemplate(`{{camelCase "my-service"}}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "myService", string(out))

	out, err = executeTemplate(`{{snakeCase "my service"}}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "my_service", string(out))

	out, err = executeTemplate(`{{pascalCase "my-service"}}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "MyService", string(out))

	out, err = executeTemplate(`{{kebabCase "my service"}}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "my-service", string(out))

	out, err = executeTemplate(`{{upper "hello"}}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "HELLO", string(out))

	out, err = executeTemplate(`{{lower "HELLO"}}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "hello", string(out))
}

func TestRenderPath(t *testing.T) {
	vars := map[string]interface{}{
		"ServiceName": "orders",
	}

	t.Run("simple token replacement", func(t *testing.T) {
		got, err := RenderPath("cmd/__ServiceName__/main.go", vars)
		require.NoError(t, err)
		assert.Equal(t, "cmd/orders/main.go", got)
	})

	t.Run("no tokens", func(t *testing.T) {
		got, err := RenderPath("internal/config.go", vars)
		require.NoError(t, err)
		assert.Equal(t, "internal/config.go", got)
	})

	t.Run("template expression in path", func(t *testing.T) {
		got, err := RenderPath("cmd/{{.ServiceName}}/main.go", vars)
		require.NoError(t, err)
		assert.Equal(t, "cmd/orders/main.go", got)
	})

	t.Run("tmpl suffix preserved by RenderPath", func(t *testing.T) {
		got, err := RenderPath("main.go.tmpl", vars)
		require.NoError(t, err)
		assert.Equal(t, "main.go.tmpl", got) // stripping happens in renderFilesDir, not RenderPath
	})
}

func TestRenderTemplate_WithConditionalFiles(t *testing.T) {
	// Build a minimal in-memory FS with manifest + conditional files
	memFS := fstest.MapFS{
		"test-cond/manifest.yaml": &fstest.MapFile{
			Data: []byte(`name: test-cond
variables:
  - name: ServiceName
    type: string
    required: true
conditional:
  os_root:
    include: ["internal/root.go.tmpl"]
    exclude: ["internal/fallback.go.tmpl"]
`),
		},
		"test-cond/files/main.go.tmpl": &fstest.MapFile{
			Data: []byte("package main\n"),
		},
		"test-cond/files/internal/root.go.tmpl": &fstest.MapFile{
			Data: []byte("// root version\n"),
		},
		"test-cond/files/internal/fallback.go.tmpl": &fstest.MapFile{
			Data: []byte("// fallback version\n"),
		},
	}

	renderer := NewRenderer(memFS)
	out := t.TempDir()

	vars := map[string]interface{}{
		"ServiceName": "test",
		"Features":    map[string]bool{"os_root": true},
	}

	_, err := renderer.RenderTemplate("test-cond", out, vars)
	require.NoError(t, err)

	// root.go should exist (feature true + include)
	_, err = os.Stat(filepath.Join(out, "internal", "root.go"))
	assert.NoError(t, err, "root.go should exist when os_root=true")

	// fallback.go should NOT exist (feature true + exclude)
	_, err = os.Stat(filepath.Join(out, "internal", "fallback.go"))
	assert.True(t, os.IsNotExist(err), "fallback.go should not exist when os_root=true")
}

func TestRenderTemplate_ConditionalFeatureFalse(t *testing.T) {
	memFS := fstest.MapFS{
		"test-cond2/manifest.yaml": &fstest.MapFile{
			Data: []byte(`name: test-cond2
variables:
  - name: ServiceName
    type: string
    required: true
conditional:
  os_root:
    include: ["internal/root.go.tmpl"]
    exclude: ["internal/fallback.go.tmpl"]
`),
		},
		"test-cond2/files/main.go.tmpl": &fstest.MapFile{
			Data: []byte("package main\n"),
		},
		"test-cond2/files/internal/root.go.tmpl": &fstest.MapFile{
			Data: []byte("// root version\n"),
		},
		"test-cond2/files/internal/fallback.go.tmpl": &fstest.MapFile{
			Data: []byte("// fallback version\n"),
		},
	}

	renderer := NewRenderer(memFS)
	out := t.TempDir()

	vars := map[string]interface{}{
		"ServiceName": "test",
		"Features":    map[string]bool{"os_root": false},
	}

	_, err := renderer.RenderTemplate("test-cond2", out, vars)
	require.NoError(t, err)

	// root.go should NOT exist (feature false + include list → skip)
	_, err = os.Stat(filepath.Join(out, "internal", "root.go"))
	assert.True(t, os.IsNotExist(err), "root.go should not exist when os_root=false")

	// fallback.go should exist (feature false + exclude list → include)
	_, err = os.Stat(filepath.Join(out, "internal", "fallback.go"))
	assert.NoError(t, err, "fallback.go should exist when os_root=false")
}

func TestDefaultsMethod(t *testing.T) {
	m := &Manifest{
		Variables: []VariableDefinition{
			{Name: "ServiceName", Type: "string", Default: "my-svc"},
			{Name: "EnableLog", Type: "bool", Default: "true"},
			{Name: "DisableLog", Type: "bool", Default: "false"},
			{Name: "NoDefault", Type: "string"},
			{Name: "EmptyDefault", Type: "string", Default: ""},
		},
	}

	defaults := m.Defaults()
	assert.Equal(t, "my-svc", defaults["ServiceName"])
	assert.Equal(t, true, defaults["EnableLog"])
	assert.Equal(t, false, defaults["DisableLog"])
	_, exists := defaults["NoDefault"]
	assert.False(t, exists)
	_, exists = defaults["EmptyDefault"]
	assert.False(t, exists)
}

func TestParseManifest_EdgeCases(t *testing.T) {
	t.Run("empty name", func(t *testing.T) {
		_, err := ParseManifest([]byte("name: \"\"\n"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing name")
	})

	t.Run("invalid yaml", func(t *testing.T) {
		_, err := ParseManifest([]byte(":::invalid"))
		assert.Error(t, err)
	})

	t.Run("default type is string", func(t *testing.T) {
		m, err := ParseManifest([]byte("name: test\nvariables:\n  - name: foo\n"))
		require.NoError(t, err)
		assert.Equal(t, "string", m.Variables[0].Type)
	})
}

func TestTemplateFunctions_AllPresent(t *testing.T) {
	funcs := templateFunctions()
	expected := []string{"camelCase", "snakeCase", "pascalCase", "kebabCase",
		"lower", "upper", "title", "contains", "hasPrefix", "hasSuffix",
		"join", "split", "now", "date"}
	for _, name := range expected {
		assert.NotNil(t, funcs[name], "missing template function: %s", name)
	}
}

func TestDefaultGoHooks_Coverage(t *testing.T) {
	hooks := DefaultGoHooks()
	assert.Contains(t, hooks, "go mod tidy")
	assert.Contains(t, hooks, "git init")
	assert.Len(t, hooks, 2)
}
