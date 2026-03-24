package scaffold

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type Renderer struct {
	fs fs.FS
}

type RenderResult struct {
	FilesCreated []string `json:"files_created"`
}

func NewRenderer(fsys fs.FS) *Renderer {
	return &Renderer{fs: fsys}
}

// writeToRoot writes content to the given relative path inside an os.Root,
// creating parent directories as needed.
func writeToRoot(root *os.Root, relPath string, content []byte) error {
	dir := filepath.Dir(relPath)
	if dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	f, err := root.OpenFile(relPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", relPath, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(content); err != nil {
		return fmt.Errorf("write %s: %w", relPath, err)
	}
	return nil
}

func (r *Renderer) RenderTemplate(templateDir, outputDir string, vars map[string]interface{}) (*RenderResult, error) {
	if vars == nil {
		vars = map[string]interface{}{}
	}
	manifestData, err := fs.ReadFile(r.fs, path.Join(templateDir, "manifest.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	manifest, err := ParseManifest(manifestData)
	if err != nil {
		return nil, err
	}

	for key, value := range manifest.Defaults() {
		if _, exists := vars[key]; !exists {
			vars[key] = value
		}
	}

	// Coerce string booleans to actual bools so template conditionals work correctly.
	for _, def := range manifest.Variables {
		if def.Type == "bool" {
			if v, ok := vars[def.Name]; ok {
				if s, isStr := v.(string); isStr {
					vars[def.Name] = strings.EqualFold(s, "true")
				}
			}
		}
	}

	for _, def := range manifest.Variables {
		if def.Required {
			if v, ok := vars[def.Name]; !ok || strings.TrimSpace(fmt.Sprint(v)) == "" {
				return nil, fmt.Errorf("missing required variable %q", def.Name)
			}
		}
	}

	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return nil, fmt.Errorf("resolve output dir: %w", err)
	}

	// Ensure the output directory exists before opening an os.Root on it.
	if err := os.MkdirAll(absOutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	root, err := os.OpenRoot(absOutputDir)
	if err != nil {
		return nil, fmt.Errorf("open root %s: %w", absOutputDir, err)
	}
	defer func() { _ = root.Close() }()

	// Extract features for conditional file inclusion.
	var features map[string]bool
	if f, ok := vars["Features"].(map[string]bool); ok {
		features = f
	}

	filesRoot := path.Join(templateDir, "files")
	result := &RenderResult{}
	if err := fs.WalkDir(r.fs, filesRoot, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == filesRoot {
			return nil
		}
		rel := strings.TrimPrefix(current, filesRoot+"/")

		// Check conditional rules before rendering.
		if !d.IsDir() && !shouldIncludeFile(rel, manifest.Conditional, features) {
			return nil
		}

		renderedRel, err := RenderPath(rel, vars)
		if err != nil {
			return fmt.Errorf("render path %q: %w", rel, err)
		}

		// Defense-in-depth: validate before os.Root (which provides the primary defense).
		dstPath := filepath.Join(absOutputDir, filepath.FromSlash(renderedRel))
		if err := validatePathWithinDir(dstPath, absOutputDir); err != nil {
			return err
		}

		// Use the relative path for all os.Root operations.
		relDst := filepath.FromSlash(renderedRel)

		if d.IsDir() {
			return root.MkdirAll(relDst, 0o755)
		}

		srcBytes, err := fs.ReadFile(r.fs, current)
		if err != nil {
			return err
		}

		if strings.HasSuffix(relDst, ".tmpl") {
			relDst = strings.TrimSuffix(relDst, ".tmpl")
			rendered, err := executeTemplate(string(srcBytes), vars)
			if err != nil {
				return fmt.Errorf("render template %q: %w", rel, err)
			}
			// Skip files whose rendered content is empty (allows conditional file
			// inclusion by wrapping entire templates in {{if}} blocks).
			if len(strings.TrimSpace(string(rendered))) == 0 {
				return nil
			}
			if err := writeToRoot(root, relDst, rendered); err != nil {
				return err
			}
		} else {
			if err := writeToRoot(root, relDst, srcBytes); err != nil {
				return err
			}
		}
		result.FilesCreated = append(result.FilesCreated, filepath.Join(absOutputDir, relDst))
		return nil
	}); err != nil {
		return nil, err
	}

	// Remove empty directories left behind by conditionally-skipped files.
	removeEmptyDirs(absOutputDir)

	return result, nil
}

// RenderComposition renders an architecture + capability composition into the output directory.
func (r *Renderer) RenderComposition(plan *CompositionPlan, outputDir string) (*RenderResult, error) {
	// Inject partials into vars so templates can use {{range .Partials.main_imports}}.
	vars := plan.Vars
	vars["Partials"] = plan.Partials

	// Build path mapper from architecture manifest and inject ArchPaths for templates.
	pm := NewPathMapper(plan.Manifest.PathMappings)
	vars["ArchPaths"] = pm.ArchPaths()

	// Set boolean flags for each capability (e.g., HasHTTPAPI = true).
	capSet := map[string]bool{}
	for _, c := range plan.Capabilities {
		capSet[c] = true
	}
	vars["SelectedCapabilities"] = plan.Capabilities

	// Extract resolved features from vars for conditional file inclusion.
	var features map[string]bool
	if f, ok := vars["Features"].(map[string]bool); ok {
		features = f
	}

	result := &RenderResult{}

	// Render architecture files — no path mapping needed (arch templates use native paths).
	archConditionals := plan.Manifest.Conditional
	archResult, err := r.renderFilesDir(path.Join(plan.ArchDir, "files"), outputDir, vars, archConditionals, features, nil)
	if err != nil {
		return nil, fmt.Errorf("render architecture: %w", err)
	}
	result.FilesCreated = append(result.FilesCreated, archResult.FilesCreated...)

	// Render each capability's files with path mapping applied.
	for i, capDir := range plan.CapDirs {
		var capConditionals map[string]ConditionalRule
		if i < len(plan.CapManifests) {
			capConditionals = plan.CapManifests[i].Conditional
		}
		capResult, err := r.renderFilesDir(path.Join(capDir, "files"), outputDir, vars, capConditionals, features, pm)
		if err != nil {
			return nil, fmt.Errorf("render capability %s: %w", capDir, err)
		}
		result.FilesCreated = append(result.FilesCreated, capResult.FilesCreated...)
	}

	removeEmptyDirs(outputDir)
	return result, nil
}

// renderFilesDir walks a files/ directory and renders templates into outputDir.
// conditionals and features control file-level inclusion; nil values include all files.
func (r *Renderer) renderFilesDir(filesRoot, outputDir string, vars map[string]interface{}, conditionals map[string]ConditionalRule, features map[string]bool, pm *PathMapper) (*RenderResult, error) {
	result := &RenderResult{}

	// Check if the directory exists.
	if _, err := fs.Stat(r.fs, filesRoot); err != nil {
		return result, nil // no files directory
	}

	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return nil, fmt.Errorf("resolve output dir: %w", err)
	}

	if err := os.MkdirAll(absOutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	root, err := os.OpenRoot(absOutputDir)
	if err != nil {
		return nil, fmt.Errorf("open root %s: %w", absOutputDir, err)
	}
	defer func() { _ = root.Close() }()

	if err := fs.WalkDir(r.fs, filesRoot, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == filesRoot {
			return nil
		}
		rel := strings.TrimPrefix(current, filesRoot+"/")

		// Check conditional rules before rendering.
		if !d.IsDir() && !shouldIncludeFile(rel, conditionals, features) {
			return nil
		}

		renderedRel, err := RenderPath(rel, vars)
		if err != nil {
			return fmt.Errorf("render path %q: %w", rel, err)
		}

		// Apply architecture path mapping to capability files.
		if pm != nil {
			renderedRel = pm.Map(renderedRel)
		}

		// Defense-in-depth: validate before os.Root (which provides the primary defense).
		dstPath := filepath.Join(absOutputDir, filepath.FromSlash(renderedRel))
		if err := validatePathWithinDir(dstPath, absOutputDir); err != nil {
			return err
		}

		relDst := filepath.FromSlash(renderedRel)

		if d.IsDir() {
			return root.MkdirAll(relDst, 0o755)
		}

		srcBytes, err := fs.ReadFile(r.fs, current)
		if err != nil {
			return err
		}

		if strings.HasSuffix(relDst, ".tmpl") {
			relDst = strings.TrimSuffix(relDst, ".tmpl")
			rendered, err := executeTemplate(string(srcBytes), vars)
			if err != nil {
				return fmt.Errorf("render template %q: %w", rel, err)
			}
			if len(strings.TrimSpace(string(rendered))) == 0 {
				return nil
			}
			if err := writeToRoot(root, relDst, rendered); err != nil {
				return err
			}
		} else {
			if err := writeToRoot(root, relDst, srcBytes); err != nil {
				return err
			}
		}
		result.FilesCreated = append(result.FilesCreated, filepath.Join(absOutputDir, relDst))
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// RenderCapabilityFiles renders files from a capability directory into outputDir,
// skipping files that already exist on disk. Returns the render result (created files)
// and a list of skipped files.
func (r *Renderer) RenderCapabilityFiles(capDir, outputDir string, vars map[string]interface{}) (*RenderResult, []string, error) {
	filesRoot := path.Join(capDir, "files")
	result := &RenderResult{}
	var skipped []string

	if _, err := fs.Stat(r.fs, filesRoot); err != nil {
		return result, skipped, nil
	}

	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve output dir: %w", err)
	}

	if err := os.MkdirAll(absOutputDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create output dir: %w", err)
	}

	root, err := os.OpenRoot(absOutputDir)
	if err != nil {
		return nil, nil, fmt.Errorf("open root %s: %w", absOutputDir, err)
	}
	defer func() { _ = root.Close() }()

	if err := fs.WalkDir(r.fs, filesRoot, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == filesRoot {
			return nil
		}
		rel := strings.TrimPrefix(current, filesRoot+"/")

		renderedRel, renderErr := RenderPath(rel, vars)
		if renderErr != nil {
			return fmt.Errorf("render path %q: %w", rel, renderErr)
		}

		// Defense-in-depth: validate before os.Root (which provides the primary defense).
		dstPath := filepath.Join(absOutputDir, filepath.FromSlash(renderedRel))
		if err := validatePathWithinDir(dstPath, absOutputDir); err != nil {
			return err
		}

		relDst := filepath.FromSlash(renderedRel)

		if d.IsDir() {
			return root.MkdirAll(relDst, 0o755)
		}

		// Compute final relative path (strip .tmpl suffix) before checking existence.
		finalRel := strings.TrimSuffix(relDst, ".tmpl")
		finalPath := filepath.Join(absOutputDir, finalRel)

		// Skip if file already exists on disk.
		if _, statErr := os.Stat(finalPath); statErr == nil {
			skipped = append(skipped, finalPath)
			return nil
		}

		srcBytes, readErr := fs.ReadFile(r.fs, current)
		if readErr != nil {
			return readErr
		}

		if strings.HasSuffix(relDst, ".tmpl") {
			rendered, tmplErr := executeTemplate(string(srcBytes), vars)
			if tmplErr != nil {
				return fmt.Errorf("render template %q: %w", rel, tmplErr)
			}
			if len(strings.TrimSpace(string(rendered))) == 0 {
				return nil
			}
			if err := writeToRoot(root, finalRel, rendered); err != nil {
				return err
			}
		} else {
			if err := writeToRoot(root, finalRel, srcBytes); err != nil {
				return err
			}
		}
		result.FilesCreated = append(result.FilesCreated, finalPath)
		return nil
	}); err != nil {
		return nil, nil, err
	}

	return result, skipped, nil
}

// removeEmptyDirs walks bottom-up and removes directories that contain no files.
func removeEmptyDirs(rootDir string) {
	// Collect dirs in reverse depth order (deepest first).
	var dirs []string
	// Best-effort cleanup: walk errors are intentionally ignored because failing
	// to remove empty directories is not a critical error.
	_ = filepath.WalkDir(rootDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Skip symlinked directories — they point outside the project boundary (INV-002).
		if d.IsDir() && d.Type()&fs.ModeSymlink != 0 {
			return filepath.SkipDir
		}
		if d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		if dirs[i] == rootDir {
			continue
		}
		entries, err := os.ReadDir(dirs[i])
		if err == nil && len(entries) == 0 {
			_ = os.Remove(dirs[i])
		}
	}
}

func executeTemplate(content string, vars map[string]interface{}) ([]byte, error) {
	tmpl, err := template.New("file").Funcs(templateFunctions()).Option("missingkey=zero").Parse(content)
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	if err := tmpl.Execute(buf, vars); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// validatePathWithinDir ensures the resolved path stays within the output directory.
// This is a defense-in-depth check -- os.Root provides the primary kernel-level defense
// against path traversal. This function catches traversal attempts early with a clear
// error message before os.Root would reject them.
func validatePathWithinDir(target string, absDir string) error {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	// Ensure the target is within the directory (equal to or a child of absDir).
	if !strings.HasPrefix(absTarget, absDir+string(filepath.Separator)) && absTarget != absDir {
		return fmt.Errorf("path traversal detected: %q escapes output directory", target)
	}
	return nil
}

// RenderPath applies template variable substitution to a relative file path.
// It replaces __Key__ tokens and evaluates {{...}} Go template expressions.
func RenderPath(rel string, vars map[string]interface{}) (string, error) {
	segments := strings.Split(rel, "/")
	for i := range segments {
		for key, value := range vars {
			token := "__" + key + "__"
			segments[i] = strings.ReplaceAll(segments[i], token, fmt.Sprint(value))
		}
		if strings.Contains(segments[i], "{{") {
			rendered, err := executeTemplate(segments[i], vars)
			if err != nil {
				return "", err
			}
			segments[i] = string(rendered)
		}
	}
	return path.Join(segments...), nil
}

func templateFunctions() template.FuncMap {
	return template.FuncMap{
		"camelCase":  camelCase,
		"snakeCase":  snakeCase,
		"pascalCase": pascalCase,
		"kebabCase":  kebabCase,
		"lower":      strings.ToLower,
		"upper":      strings.ToUpper,
		"title":      cases.Title(language.Und).String,
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"join":       strings.Join,
		"split":      strings.Split,
		"now":        time.Now,
		"date": func(layout string, t time.Time) string {
			return t.Format(layout)
		},
	}
}

var wordRegexp = regexp.MustCompile(`[A-Za-z0-9]+`)

func words(value string) []string {
	chunks := wordRegexp.FindAllString(value, -1)
	for i := range chunks {
		chunks[i] = strings.ToLower(chunks[i])
	}
	return chunks
}

func pascalCase(value string) string {
	parts := words(value)
	for i := range parts {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func camelCase(value string) string {
	parts := words(value)
	if len(parts) == 0 {
		return ""
	}
	for i := 1; i < len(parts); i++ {
		parts[i] = cases.Title(language.Und).String(parts[i])
	}
	return strings.Join(parts, "")
}

func snakeCase(value string) string {
	return strings.Join(words(value), "_")
}

func kebabCase(value string) string {
	return strings.Join(words(value), "-")
}
