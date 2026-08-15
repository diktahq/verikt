package scaffold

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const platformConfigDir = "../../providers/golang/templates/capabilities/platform/files/config"

// renderPlatformConfig renders a platform config template with every optional
// capability enabled, so the credential-carrying sections are present.
func renderPlatformConfig(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(platformConfigDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	tmpl, err := template.New(name).Parse(string(data))
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}

	var out strings.Builder
	err = tmpl.Execute(&out, map[string]any{
		"ServiceName":   "testsvc",
		"HasHTTP":       true,
		"HasGRPC":       true,
		"HasKafka":      true,
		"HasMySQL":      true,
		"HasPostgreSQL": true,
		"HasRedis":      true,
		"HasMailpit":    true,
		"HasI18n":       true,
	})
	if err != nil {
		t.Fatalf("execute %s: %v", name, err)
	}
	return out.String()
}

// A credential must have no yaml key at all.
//
// The generated loader read a committed YAML file and nothing else, so a project
// whose only real setting was an API token had two options: commit the secret, or
// stop using the config layer. Adding an environment source fixes that; the
// `yaml:"-"` tag is what stops the file from being a second, silent source.
//
// Leaving the tag off is not equivalent: yaml.v3 falls back to the lowercased
// field name, so `token:` in the file is accepted and the secret-in-a-committed-
// file case passes quietly. With no yaml key and a KnownFields decoder, the key
// is unknown and the whole file is refused.
func TestPlatformConfigCredentialsHaveNoYAMLKey(t *testing.T) {
	src := renderPlatformConfig(t, "config.go.tmpl")

	file, err := parser.ParseFile(token.NewFileSet(), "config.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("rendered config.go does not parse: %v", err)
	}

	credentials := map[string]string{
		"MySQLConfig.DSN":      "MYSQL_DSN",
		"PostgresConfig.URL":   "POSTGRES_URL",
		"RedisConfig.Password": "REDIS_PASSWORD",
	}

	found := map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					key := typeSpec.Name.Name + "." + name.Name
					if _, isCredential := credentials[key]; !isCredential {
						continue
					}
					found[key] = true
					if field.Tag == nil {
						t.Errorf("%s has no struct tag; a credential must be tagged yaml:\"-\"", key)
						continue
					}
					raw, err := strconv.Unquote(field.Tag.Value)
					if err != nil {
						t.Fatalf("%s: unquote tag: %v", key, err)
					}
					if got := reflect.StructTag(raw).Get("yaml"); got != "-" {
						t.Errorf("%s has yaml:%q — a credential must not be settable from the committed config file", key, got)
					}
				}
			}
		}
	}

	for key := range credentials {
		if !found[key] {
			t.Errorf("%s not found in the rendered config; the test no longer guards what it names", key)
		}
	}

	// Each credential needs an environment source, or it cannot be supplied at all.
	for key, env := range credentials {
		if !strings.Contains(src, `"`+env+`"`) {
			t.Errorf("%s has no environment source (%s); it would be unsettable", key, env)
		}
	}
}

// The environment must be read after the file, or it cannot override it.
func TestPlatformConfigAppliesEnvironmentAfterFile(t *testing.T) {
	src := renderPlatformConfig(t, "config.go.tmpl")

	fileIdx := strings.Index(src, "loadFile(configPath")
	envIdx := strings.Index(src, "applyEnv(&cfg)")

	if fileIdx < 0 || envIdx < 0 {
		t.Fatalf("expected Load to read the file then the environment; got:\n%s", src)
	}
	if envIdx < fileIdx {
		t.Error("the environment is applied before the file, so it cannot override it")
	}
	if !strings.Contains(src, "decoder.KnownFields(true)") {
		t.Error("without KnownFields an unknown key is ignored, so a secret in the file is accepted silently")
	}
}

// The example config is committed, so anything it shows is something a reader
// will commit. It shipped `password: ""` and two DSNs with literal passwords,
// which teaches the wrong habit by demonstration.
func TestPlatformConfigExampleShowsNoSecrets(t *testing.T) {
	example := renderPlatformConfig(t, "config.yaml.example.tmpl")

	for _, forbidden := range []string{"password@", "root:password", ":password@"} {
		if strings.Contains(example, forbidden) {
			t.Errorf("config.yaml.example contains a literal credential (%q)", forbidden)
		}
	}

	// The credential keys must not appear as settable YAML keys.
	for _, line := range strings.Split(example, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue // a comment naming the variable is the point
		}
		for _, key := range []string{"dsn:", "url:", "password:"} {
			if strings.HasPrefix(trimmed, key) {
				t.Errorf("config.yaml.example offers %q as a file key; it is refused by the loader", key)
			}
		}
	}

	// And the reader has to be told where the credentials do come from.
	for _, env := range []string{"MYSQL_DSN", "POSTGRES_URL", "REDIS_PASSWORD"} {
		if !strings.Contains(example, env) {
			t.Errorf("config.yaml.example does not mention %s, so the reader cannot know how to set it", env)
		}
	}
}

// envVarRef matches an environment variable name in either template.
var envVarRef = regexp.MustCompile(`\b([A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+)\b`)

// Every variable the example advertises must actually be read, and every
// variable the loader reads must be documented.
//
// config.yaml.example said "any value above can also be set from the
// environment" while applyEnv covered a subset — no APP_VERSION, no
// HTTP_READ_TIMEOUT, no GRPC_REFLECTION, no KAFKA_TOPICS, no REDIS_DB. A
// documented variable that does nothing is worse than an undocumented one: the
// operator sets it, sees no effect, and has no reason to suspect the docs.
func TestPlatformConfigEnvCoverageMatchesTheExample(t *testing.T) {
	code := renderPlatformConfig(t, "config.go.tmpl")
	example := renderPlatformConfig(t, "config.yaml.example.tmpl")

	inCode := map[string]bool{}
	for _, m := range regexp.MustCompile(`set(?:String|Int|Int32|Bool|Strings)\(&[^,]+,\s*"([A-Z0-9_]+)"\)`).FindAllStringSubmatch(code, -1) {
		inCode[m[1]] = true
	}
	require.NotEmpty(t, inCode, "no environment variables parsed from config.go — did the setter shape change?")

	documented := map[string]bool{}
	for _, m := range envVarRef.FindAllStringSubmatch(example, -1) {
		documented[m[1]] = true
	}

	for name := range documented {
		assert.True(t, inCode[name],
			"config.yaml.example documents %s but config.go never reads it", name)
	}
	for name := range inCode {
		assert.True(t, documented[name],
			"config.go reads %s but config.yaml.example does not mention it", name)
	}
}

// A malformed value must be an error, not a silently ignored setting.
func TestPlatformConfigReportsMalformedEnvValues(t *testing.T) {
	code := renderPlatformConfig(t, "config.go.tmpl")

	require.Contains(t, code, "func applyEnv(cfg *Config) error",
		"applyEnv must be able to report a bad value")
	require.Contains(t, code, "if err := applyEnv(&cfg); err != nil",
		"Load must propagate what applyEnv reports")
	assert.Contains(t, code, "errors.Join(errs...)",
		"every malformed value should be reported together, so one run finds every typo")

	// Each numeric or boolean setter must return an error rather than swallow one.
	for _, setter := range []string{
		"func setInt(target *int, key string) error",
		"func setInt32(target *int32, key string) error",
		"func setBool(target *bool, key string) error",
	} {
		assert.Contains(t, code, setter, "%s must report a malformed value", setter)
	}
}
