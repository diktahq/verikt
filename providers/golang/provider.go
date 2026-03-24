package golang

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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

func (p *GoProvider) Scaffold(_ context.Context, req provider.ScaffoldRequest) (*provider.ScaffoldResponse, error) {
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

	renderer := scaffold.NewRenderer(templatesFS)

	// Parse capabilities from options (comma-separated).
	var capabilities []string
	if capStr := req.Options["capabilities"]; capStr != "" {
		for _, c := range strings.Split(capStr, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				capabilities = append(capabilities, c)
			}
		}
	}

	var renderResult *scaffold.RenderResult
	if len(capabilities) > 0 {
		// Composition mode: architecture + capabilities.
		plan, err := scaffold.ComposeProject(templatesFS, architecture, capabilities, vars)
		if err != nil {
			return nil, fmt.Errorf("compose project: %w", err)
		}
		renderResult, err = renderer.RenderComposition(plan, req.OutputDir)
		if err != nil {
			return nil, err
		}
	} else {
		// Legacy mode: single template directory.
		archDir := path.Join("templates", "architectures", architecture)
		var err error
		renderResult, err = renderer.RenderTemplate(archDir, req.OutputDir, vars)
		if err != nil {
			return nil, err
		}
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

	cmd := exec.CommandContext(ctx, goPath, "env", "GOVERSION")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("running go env GOVERSION: %w", err)
	}

	return parseGoVersion(strings.TrimSpace(stdout.String())), nil
}

// parseGoVersion extracts major.minor from a Go version string like "go1.26.1".
func parseGoVersion(raw string) string {
	if raw == "" {
		return ""
	}
	raw = strings.TrimPrefix(raw, "go")
	parts := strings.SplitN(raw, ".", 3)
	if len(parts) < 2 {
		return raw
	}
	return parts[0] + "." + parts[1]
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
