package golang

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/diktahq/verikt/internal/analyzer"
	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/guide"
	"github.com/diktahq/verikt/internal/provider"
	"github.com/diktahq/verikt/internal/scaffold"
)

// Compile-time interface checks.
var _ provider.VersionDetector = (*GoProvider)(nil)
var _ provider.FeatureMatrixProvider = (*GoProvider)(nil)

type GoProvider struct{}

func init() {
	provider.Register("go", &GoProvider{})
}

func (p *GoProvider) Scaffold(ctx context.Context, req provider.ScaffoldRequest) (*provider.ScaffoldResponse, error) {
	templateName := strings.TrimSpace(req.TemplateName)
	if templateName == "" {
		templateName = "api"
	}
	if req.OutputDir == "" {
		req.OutputDir = "."
	}
	if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	vars := map[string]interface{}{}
	for k, v := range req.Options {
		vars[k] = v
	}
	if req.ProjectName != "" {
		vars["ServiceName"] = req.ProjectName
	}
	if req.ModulePath != "" {
		vars["ModulePath"] = req.ModulePath
	}

	// Map legacy template names to architectures.
	archMap := map[string]string{"api": "hexagonal", "cli": "flat", "worker": "hexagonal"}
	architecture := archMap[templateName]
	if architecture == "" {
		architecture = templateName
	}

	// Resolve the target Go version and its features (ADR-007).
	target, err := p.goTarget(ctx, req.Options, architecture)
	if err != nil {
		return nil, err
	}
	if target.Version != "" {
		vars["GoVersion"] = target.Version
	}
	vars["Features"] = target.Features

	capabilities := parseCapabilities(req.Options["capabilities"])
	renderResult, err := renderProject(architecture, capabilities, vars, req.OutputDir)
	var missing *scaffold.MissingFeaturesError
	if errors.As(err, &missing) {
		return nil, explainMissingFeatures(missing, target)
	}
	if err != nil {
		return nil, err
	}

	archDir := path.Join("templates", "architectures", architecture)
	manifest, err := loadManifest(archDir)
	if err == nil {
		hooks := manifest.Hooks
		if len(hooks) == 0 {
			hooks = scaffold.DefaultGoHooks()
		}
		if strings.EqualFold(req.Options["skip_hooks"], "true") {
			hooks = nil
		}
		if err := scaffold.RunPostScaffoldHooks(req.OutputDir, hooks, vars); err != nil {
			return nil, err
		}
	}

	veriktCfg := config.DefaultVeriktConfig("go", architecture)
	// The manifest is the source of truth for the component graph a project
	// is scaffolded with; DefaultVeriktConfig supplies the remaining rules.
	if manifest != nil && len(manifest.Components) > 0 {
		veriktCfg.Components = manifest.Components
	}
	if len(capabilities) > 0 {
		veriktCfg.Capabilities = capabilities
	}
	if guideMode := strings.TrimSpace(req.Options["guide_mode"]); guideMode != "" {
		veriktCfg.Guide.Mode = guideMode
	}
	veriktPath := filepath.Join(req.OutputDir, "verikt.yaml")
	if err := config.SaveVeriktYAML(veriktPath, veriktCfg); err != nil {
		return nil, err
	}
	veriktBytes, _ := os.ReadFile(veriktPath)

	files := append([]string{}, renderResult.FilesCreated...)
	files = append(files, veriktPath)

	// Generate project matrix doc.
	matrixPath, err := generateProjectMatrix(req.OutputDir, architecture, capabilities, veriktCfg)
	if err == nil && matrixPath != "" {
		files = append(files, matrixPath)
	}

	// Generate AI agent architecture guides.
	if guideErr := guide.GenerateFromConfig(req.OutputDir, veriktCfg, "all", templatesFS); guideErr != nil {
		return nil, fmt.Errorf("generate guide: %w", guideErr)
	}

	return &provider.ScaffoldResponse{FilesCreated: files, VeriktYAML: veriktBytes}, nil
}

// parseCapabilities splits the comma-separated capabilities option.
func parseCapabilities(capStr string) []string {
	var capabilities []string
	for _, c := range strings.Split(capStr, ",") {
		if c = strings.TrimSpace(c); c != "" {
			capabilities = append(capabilities, c)
		}
	}
	return capabilities
}

// renderProject renders the architecture with its capabilities (composition
// mode) or, with none, the architecture's template directory alone (legacy
// mode). Both enforce requires_features against vars["Features"].
func renderProject(architecture string, capabilities []string, vars map[string]interface{}, outputDir string) (*scaffold.RenderResult, error) {
	renderer := scaffold.NewRenderer(templatesFS)
	archDir := path.Join("templates", "architectures", architecture)
	if len(capabilities) > 0 {
		plan, err := scaffold.ComposeProject(templatesFS, architecture, capabilities, vars)
		if err != nil {
			return nil, fmt.Errorf("compose project: %w", err)
		}
		return renderer.RenderComposition(plan, outputDir)
	}
	manifest, err := loadManifest(archDir)
	if err != nil {
		return nil, fmt.Errorf("load architecture manifest %q: %w", architecture, err)
	}
	if err := scaffold.EnforceRequiredFeatures(templatesFS, manifest, nil, vars); err != nil {
		return nil, err
	}
	return renderer.RenderTemplate(archDir, outputDir, vars)
}

// goTarget resolves the Go version a new project targets and the features it
// provides: --set GoVersion when given, else the installed toolchain, else the
// architecture manifest's GoVersion default. ADR-007 names that default as the
// fallback; without a toolchain the post-scaffold hooks cannot run anyway, so
// the default only has to produce a go.mod a later toolchain accepts.
func (p *GoProvider) goTarget(ctx context.Context, options map[string]string, architecture string) (*scaffold.Target, error) {
	fallback := ""
	if manifest, err := loadManifest(path.Join("templates", "architectures", architecture)); err == nil {
		if v, ok := manifest.Defaults()["GoVersion"].(string); ok {
			fallback = v
		}
	}
	pinned := scaffold.VersionCandidate{Version: options["GoVersion"], Source: scaffold.VersionExplicit}
	target, err := (&scaffold.FeatureResolver{}).ResolveTarget(ctx, p, pinned, fallback)
	if err != nil {
		return nil, fmt.Errorf("GoVersion: %w", err)
	}
	return target, nil
}

// explainMissingFeatures adds the target Go version, where it came from, and
// how to raise it to a requires_features failure.
func explainMissingFeatures(missing *scaffold.MissingFeaturesError, target *scaffold.Target) error {
	var origin string
	switch target.Source {
	case scaffold.VersionExplicit:
		origin = "set with --set GoVersion"
	case scaffold.VersionProject:
		origin = "from go.mod"
	case scaffold.VersionDetected:
		origin = "the installed Go toolchain"
	case scaffold.VersionDefault:
		origin = "the architecture default; no Go toolchain was detected"
	default:
		origin = "unknown origin"
	}
	version := target.Version
	if version == "" {
		version = "unknown"
	}
	minimum := missing.MinimumVersion()
	if minimum == "" {
		return fmt.Errorf("%w\n  target Go version is %s (%s)\n  no Go version provides that feature: the template declares a feature missing from providers/golang/templates/features.yaml", missing, version, origin)
	}
	return fmt.Errorf("%w\n  target Go version is %s (%s)\n  fix: install Go %s or newer, or pass --set GoVersion=%s", missing, version, origin, minimum, minimum)
}

func (p *GoProvider) Analyze(ctx context.Context, req provider.AnalyzeRequest) (*provider.AnalyzeResponse, error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		path = "."
	}
	a := analyzer.New(path)
	if err := a.LoadPackages(""); err != nil {
		return nil, err
	}
	return a.Analyze(ctx)
}

func (p *GoProvider) Migrate(_ context.Context, _ provider.MigrateRequest) (*provider.MigrateResponse, error) {
	return nil, provider.ErrNotImplemented
}

func (p *GoProvider) GetInfo(_ context.Context) (*provider.ProviderInfo, error) {
	templates, err := listTemplates()
	if err != nil {
		return nil, err
	}
	return &provider.ProviderInfo{
		Name:                   "verikt-go-provider",
		Version:                "v1",
		Language:               "go",
		SupportedArchitectures: []string{"hexagonal", "flat", "layered", "clean"},
		Templates:              templates,
	}, nil
}

func (p *GoProvider) GetTemplateFS() fs.FS {
	return templatesFS
}

func (p *GoProvider) DetectVersion(ctx context.Context) (string, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// goPath is the go binary found on PATH, run with fixed arguments and no shell.
	cmd := exec.CommandContext(ctx, goPath, "env", "GOVERSION") //nolint:gosec // G204: fixed arguments
	// Run outside the working directory: a go.mod there with a newer go or
	// toolchain line would make GOTOOLCHAIN=auto switch (and download) a
	// toolchain, reporting that instead of the one installed.
	cmd.Dir = os.TempDir()
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("running go env GOVERSION: %w", err)
	}

	return parseGoVersion(strings.TrimSpace(stdout.String())), nil
}

// goVersionPattern matches a release or pre-release Go version: go1.26.1,
// go1.22, 1.21.3, go1.23rc1.
var goVersionPattern = regexp.MustCompile(`^(?:go)?(\d+\.\d+)(?:\.\d+)?((?:rc|beta)\d+)?$`)

// parseGoVersion extracts major.minor (plus any pre-release suffix) from a Go
// version string like "go1.26.1". Anything else, such as a devel build,
// returns "" so callers treat the version as undetected.
func parseGoVersion(raw string) string {
	m := goVersionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return ""
	}
	return m[1] + m[2]
}

func (p *GoProvider) GetFeatureMatrix() ([]byte, error) {
	return fs.ReadFile(p.GetTemplateFS(), "templates/features.yaml")
}

func loadManifest(templateDir string) (*scaffold.Manifest, error) {
	data, err := fs.ReadFile(templatesFS, path.Join(templateDir, "manifest.yaml"))
	if err != nil {
		return nil, err
	}
	return scaffold.ParseManifest(data)
}

func listTemplates() ([]provider.TemplateInfo, error) {
	entries, err := fs.ReadDir(templatesFS, path.Join("templates", "architectures"))
	if err != nil {
		return nil, err
	}
	infos := []provider.TemplateInfo{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := loadManifest(path.Join("templates", "architectures", entry.Name()))
		if err != nil {
			continue
		}
		vars := make([]provider.VariableInfo, 0, len(manifest.Variables))
		for _, v := range manifest.Variables {
			vars = append(vars, provider.VariableInfo{
				Name:        v.Name,
				Type:        v.Type,
				Description: v.Description,
				Default:     v.Default,
				Required:    v.Required,
				Choices:     v.Choices,
			})
		}
		infos = append(infos, provider.TemplateInfo{
			Name:        manifest.Name,
			Description: manifest.Description,
			Variables:   vars,
		})
	}
	return infos, nil
}
