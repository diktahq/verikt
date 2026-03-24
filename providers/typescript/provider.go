package typescript

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/guide"
	"github.com/diktahq/verikt/internal/provider"
	"github.com/diktahq/verikt/internal/scaffold"
)

// Compile-time interface checks.
var _ provider.VersionDetector = (*TypeScriptProvider)(nil)
var _ provider.FeatureMatrixProvider = (*TypeScriptProvider)(nil)

// TypeScriptProvider implements the LanguageProvider interface for TypeScript/Node.js projects.
type TypeScriptProvider struct{}

func init() {
	provider.Register("typescript", &TypeScriptProvider{})
}

func (p *TypeScriptProvider) Scaffold(ctx context.Context, req provider.ScaffoldRequest) (*provider.ScaffoldResponse, error) {
	templateName := strings.TrimSpace(req.TemplateName)
	if templateName == "" {
		templateName = "hexagonal"
	}
	if req.OutputDir == "" {
		req.OutputDir = "."
	}
	if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	vars := map[string]any{}
	for k, v := range req.Options {
		vars[k] = v
	}
	if req.ProjectName != "" {
		vars["ServiceName"] = req.ProjectName
		vars["PackageName"] = req.ProjectName
	}
	if req.ModulePath != "" {
		vars["ModulePath"] = req.ModulePath
		vars["PackageName"] = req.ModulePath
	}

	architecture := templateName

	// Resolve version-gated features (ADR-008).
	// If NodeVersion is explicitly set via --set, use it. Otherwise detect from node --version.
	nodeVersion := ""
	if v, ok := req.Options["NodeVersion"]; ok && v != "" {
		nodeVersion = v
	} else {
		if detected, dErr := p.DetectVersion(ctx); dErr == nil && detected != "" {
			nodeVersion = detected
		}
	}
	if nodeVersion != "" {
		if matrixData, mErr := p.GetFeatureMatrix(); mErr == nil {
			resolver := &scaffold.FeatureResolver{}
			if features, fErr := resolver.ResolveWithVersion(nodeVersion, matrixData); fErr == nil {
				vars["Features"] = features
			}
		}
	}
	if vars["Features"] == nil {
		vars["Features"] = map[string]bool{}
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
		// Single architecture template (no capabilities selected).
		archDir := path.Join("templates", "architectures", architecture)
		if _, err := loadManifest(archDir); err != nil {
			return nil, fmt.Errorf("architecture %q has no templates yet; select at least one capability to scaffold", architecture)
		}
		var err error
		renderResult, err = renderer.RenderTemplate(archDir, req.OutputDir, vars)
		if err != nil {
			return nil, err
		}
	}

	archDir := path.Join("templates", "architectures", architecture)
	if manifest, err := loadManifest(archDir); err == nil {
		hooks := manifest.Hooks
		if strings.EqualFold(req.Options["skip_hooks"], "true") {
			hooks = nil
		}
		if len(hooks) > 0 {
			if err := scaffold.RunPostScaffoldHooks(req.OutputDir, hooks, vars); err != nil {
				return nil, err
			}
		}
	}

	veriktCfg := config.DefaultVeriktConfig("typescript", architecture)
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

	// Inject capability-specific npm dependencies into package.json.
	if len(capabilities) > 0 {
		if err := injectPackageDependencies(req.OutputDir, capabilities, req.Options); err != nil {
			return nil, fmt.Errorf("inject package dependencies: %w", err)
		}
	}

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

func (p *TypeScriptProvider) Analyze(_ context.Context, _ provider.AnalyzeRequest) (*provider.AnalyzeResponse, error) {
	return nil, provider.ErrNotImplemented
}

func (p *TypeScriptProvider) Migrate(_ context.Context, _ provider.MigrateRequest) (*provider.MigrateResponse, error) {
	return nil, provider.ErrNotImplemented
}

func (p *TypeScriptProvider) GetInfo(_ context.Context) (*provider.ProviderInfo, error) {
	templates, err := listTemplates()
	if err != nil {
		return nil, err
	}
	return &provider.ProviderInfo{
		Name:                   "verikt-typescript-provider",
		Version:                "v1",
		Language:               "typescript",
		SupportedArchitectures: []string{"hexagonal", "flat"},
		Templates:              templates,
	}, nil
}

func (p *TypeScriptProvider) GetTemplateFS() fs.FS {
	return templatesFS
}

func (p *TypeScriptProvider) DetectVersion(ctx context.Context) (string, error) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, nodePath, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("running node --version: %w", err)
	}

	return parseNodeVersion(strings.TrimSpace(string(out))), nil
}

// parseNodeVersion extracts major from a Node version string like "v20.11.0".
func parseNodeVersion(raw string) string {
	raw = strings.TrimPrefix(raw, "v")
	parts := strings.SplitN(raw, ".", 2)
	if len(parts) == 0 {
		return raw
	}
	return parts[0]
}

func (p *TypeScriptProvider) GetFeatureMatrix() ([]byte, error) {
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
	infos := make([]provider.TemplateInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := loadManifest(path.Join("templates", "architectures", entry.Name()))
		if err != nil {
			// Architecture has no manifest yet — include with minimal info.
			infos = append(infos, provider.TemplateInfo{Name: entry.Name()})
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
