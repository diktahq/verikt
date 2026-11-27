package scaffold

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
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
			if err := writeToRoot(root, relDst, formatGo(relDst, rendered)); err != nil {
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
	vars, features, pm := prepareCompositionVars(plan)
	result := &RenderResult{}

	// Render architecture files — no path mapping needed (arch templates use native paths).
	archOut, err := r.renderFilesDir(path.Join(plan.ArchDir, "files"), outputDir, vars, filesDirOptions{
		conditionals: plan.Manifest.Conditional,
		features:     features,
	})
	if err != nil {
		return nil, fmt.Errorf("render architecture: %w", err)
	}
	result.FilesCreated = append(result.FilesCreated, archOut.created...)

	// Render each capability's files with path mapping applied.
	for i := range plan.CapDirs {
		capOut, err := r.renderPlanCapability(plan, i, outputDir, vars, filesDirOptions{features: features, pm: pm})
		if err != nil {
			return nil, err
		}
		result.FilesCreated = append(result.FilesCreated, capOut.created...)
	}

	removeEmptyDirs(outputDir)
	return result, nil
}

// RenderAddedCapabilities renders only the named capabilities of plan into an
// existing project. It is the `verikt add` counterpart of RenderComposition
// and shares its rendering path: the same template vars (ArchPaths, Partials,
// SelectedCapabilities), the same conditional rules and the same architecture
// path mappings. Files that already exist are never overwritten; they are
// returned in the skipped list instead.
func (r *Renderer) RenderAddedCapabilities(plan *CompositionPlan, outputDir string, capabilities []string) (*RenderResult, []string, error) {
	out, err := r.renderAdded(plan, outputDir, capabilities, false)
	if err != nil {
		return nil, nil, err
	}
	return &RenderResult{FilesCreated: out.created}, out.skipped, nil
}

// PreviewAddedCapabilities reports the files RenderAddedCapabilities would
// create and skip, without writing anything. It backs `verikt add --dry-run`.
func (r *Renderer) PreviewAddedCapabilities(plan *CompositionPlan, outputDir string, capabilities []string) (created, skipped []string, err error) {
	out, err := r.renderAdded(plan, outputDir, capabilities, true)
	if err != nil {
		return nil, nil, err
	}
	return out.created, out.skipped, nil
}

// renderAdded renders (or, with dryRun, previews) the named capabilities of
// plan with skip-existing semantics. Every name is checked against the plan
// before anything is written.
func (r *Renderer) renderAdded(plan *CompositionPlan, outputDir string, capabilities []string, dryRun bool) (*filesDirOutcome, error) {
	indexes, err := planCapabilityIndexes(plan, capabilities)
	if err != nil {
		return nil, err
	}
	vars, features, pm := prepareCompositionVars(plan)

	total := &filesDirOutcome{}
	for _, i := range indexes {
		out, err := r.renderPlanCapability(plan, i, outputDir, vars, filesDirOptions{
			features:     features,
			pm:           pm,
			skipExisting: true,
			dryRun:       dryRun,
		})
		if err != nil {
			return nil, err
		}
		total.created = append(total.created, out.created...)
		total.skipped = append(total.skipped, out.skipped...)
	}
	return total, nil
}

// planCapabilityIndexes returns the position of each named capability in
// plan.CapDirs, in the order given.
func planCapabilityIndexes(plan *CompositionPlan, capabilities []string) ([]int, error) {
	position := make(map[string]int, len(plan.CapDirs))
	for i, capDir := range plan.CapDirs {
		position[path.Base(capDir)] = i
	}
	indexes := make([]int, 0, len(capabilities))
	for _, c := range capabilities {
		i, ok := position[c]
		if !ok {
			return nil, fmt.Errorf("capability %q is not part of the composition plan", c)
		}
		indexes = append(indexes, i)
	}
	return indexes, nil
}

// prepareCompositionVars injects the render-time variables every composition
// render needs and returns the plan's vars, resolved features and the
// architecture's path mapper.
func prepareCompositionVars(plan *CompositionPlan) (map[string]interface{}, map[string]bool, *PathMapper) {
	vars := plan.Vars
	// Partials let templates use {{range .Partials.main_imports}}.
	vars["Partials"] = plan.Partials

	// The path mapper rewrites capability paths (written for hexagonal) to the
	// target architecture; ArchPaths exposes the same mapping to templates.
	pm := NewPathMapper(plan.Manifest.PathMappings)
	vars["ArchPaths"] = pm.ArchPaths()
	vars["SelectedCapabilities"] = plan.Capabilities

	// Resolved features control conditional file inclusion.
	var features map[string]bool
	if f, ok := vars["Features"].(map[string]bool); ok {
		features = f
	}
	return vars, features, pm
}

// renderPlanCapability renders the capability at index i of plan, adding the
// capability's own conditional rules to opts.
func (r *Renderer) renderPlanCapability(plan *CompositionPlan, i int, outputDir string, vars map[string]interface{}, opts filesDirOptions) (*filesDirOutcome, error) {
	capDir := plan.CapDirs[i]
	if i < len(plan.CapManifests) {
		opts.conditionals = plan.CapManifests[i].Conditional
	}
	out, err := r.renderFilesDir(path.Join(capDir, "files"), outputDir, vars, opts)
	if err != nil {
		return nil, fmt.Errorf("render capability %s: %w", capDir, err)
	}
	return out, nil
}

// filesDirOptions controls how renderFilesDir renders a files/ directory.
type filesDirOptions struct {
	// conditionals and features control file-level inclusion; nil includes all files.
	conditionals map[string]ConditionalRule
	features     map[string]bool
	// pm maps capability paths to the target architecture; nil leaves them as-is.
	pm *PathMapper
	// skipExisting leaves files already on disk untouched and reports them as skipped.
	skipExisting bool
	// dryRun computes the outcome without creating any file or directory.
	dryRun bool
}

// filesDirOutcome lists the absolute paths a render created (or, in a dry
// run, would create) and the existing files it left untouched.
type filesDirOutcome struct {
	created []string
	skipped []string
}

// filesDirRender holds the state shared by every file of one renderFilesDir walk.
type filesDirRender struct {
	r      *Renderer
	root   *os.Root // nil in a dry run
	absOut string
	vars   map[string]interface{}
	opts   filesDirOptions
	out    filesDirOutcome
}

// renderFilesDir walks a files/ directory and renders its templates into
// outputDir. It is the single file walker behind `verikt new` and `verikt add`.
func (r *Renderer) renderFilesDir(filesRoot, outputDir string, vars map[string]interface{}, opts filesDirOptions) (*filesDirOutcome, error) {
	_, statErr := fs.Stat(r.fs, filesRoot)
	if errors.Is(statErr, fs.ErrNotExist) {
		return &filesDirOutcome{}, nil // no files directory
	}
	if statErr != nil {
		return nil, fmt.Errorf("stat %s: %w", filesRoot, statErr)
	}

	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return nil, fmt.Errorf("resolve output dir: %w", err)
	}
	w := &filesDirRender{r: r, absOut: absOutputDir, vars: vars, opts: opts}

	if !opts.dryRun {
		if err := os.MkdirAll(absOutputDir, 0o755); err != nil {
			return nil, fmt.Errorf("create output dir: %w", err)
		}
		root, err := os.OpenRoot(absOutputDir)
		if err != nil {
			return nil, fmt.Errorf("open root %s: %w", absOutputDir, err)
		}
		defer func() { _ = root.Close() }()
		w.root = root
	}

	if err := fs.WalkDir(r.fs, filesRoot, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Directories are never created up front: each written file creates its
		// own parents. Creating them here would leave the unmapped hexagonal
		// parents (adapter/ for adapter/httphandler) and the directories of
		// skipped or excluded files behind as empty directories.
		if d.IsDir() {
			return nil
		}
		return w.renderFile(current, strings.TrimPrefix(current, filesRoot+"/"))
	}); err != nil {
		return nil, err
	}
	return &w.out, nil
}

// renderFile renders the template file src (rel is its path inside files/)
// to its mapped destination.
func (w *filesDirRender) renderFile(src, rel string) error {
	if !shouldIncludeFile(rel, w.opts.conditionals, w.opts.features) {
		return nil
	}
	relDst, err := w.destinationPath(rel)
	if err != nil {
		return err
	}
	finalRel := strings.TrimSuffix(relDst, ".tmpl")
	finalPath := filepath.Join(w.absOut, finalRel)
	if w.opts.skipExisting {
		if _, statErr := os.Stat(finalPath); statErr == nil {
			w.out.skipped = append(w.out.skipped, finalPath)
			return nil
		}
	}

	content, ok, err := w.r.fileContent(src, rel, relDst != finalRel, w.vars)
	if err != nil || !ok {
		return err
	}
	if !w.opts.dryRun {
		if err := writeToRoot(w.root, finalRel, content); err != nil {
			return err
		}
	}
	w.out.created = append(w.out.created, finalPath)
	return nil
}

// destinationPath renders a template-relative path, applies the architecture
// path mapping and checks the result stays inside the output directory. It
// returns the OS-specific path relative to the output directory.
func (w *filesDirRender) destinationPath(rel string) (string, error) {
	renderedRel, err := RenderPath(rel, w.vars)
	if err != nil {
		return "", fmt.Errorf("render path %q: %w", rel, err)
	}
	if w.opts.pm != nil {
		renderedRel = w.opts.pm.Map(renderedRel)
	}

	// Defense-in-depth: validate before os.Root (which provides the primary defense).
	relDst := filepath.FromSlash(renderedRel)
	if err := validatePathWithinDir(filepath.Join(w.absOut, relDst), w.absOut); err != nil {
		return "", err
	}
	return relDst, nil
}

// fileContent reads the template file src and, when isTemplate is set,
// executes it. ok is false for a template that renders to whitespace only:
// such a file is not written (conditional inclusion via a wrapping {{if}}).
func (r *Renderer) fileContent(src, rel string, isTemplate bool, vars map[string]interface{}) (content []byte, ok bool, err error) {
	srcBytes, err := fs.ReadFile(r.fs, src)
	if err != nil {
		return nil, false, err
	}
	if !isTemplate {
		return srcBytes, true, nil
	}
	rendered, err := executeTemplate(string(srcBytes), vars)
	if err != nil {
		return nil, false, fmt.Errorf("render template %q: %w", rel, err)
	}
	if len(strings.TrimSpace(string(rendered))) == 0 {
		return nil, false, nil
	}
	return formatGo(strings.TrimSuffix(rel, ".tmpl"), rendered), true, nil
}

// formatGo gofmt-formats a rendered Go file. Templates cannot align a struct or
// composite literal whose fields are conditional, so formatting happens here
// rather than in each template; the post-scaffold gofmt hook is skippable and
// does not run when `go mod tidy` fails before it. Source that does not parse
// is returned unchanged: the generated project's build reports it with file and
// line, where a scaffold error could not.
func formatGo(name string, src []byte) []byte {
	if !strings.HasSuffix(name, ".go") {
		return src
	}
	formatted, err := format.Source(src)
	if err != nil {
		return src
	}
	return formatted
}

// RenderCapabilityFiles renders files from a single capability directory into
// outputDir without any architecture path mapping, skipping files that already
// exist on disk. Returns the render result (created files) and the skipped
// files. To add capabilities to a project, use RenderAddedCapabilities, which
// applies the architecture's path mappings.
func (r *Renderer) RenderCapabilityFiles(capDir, outputDir string, vars map[string]interface{}) (*RenderResult, []string, error) {
	out, err := r.renderFilesDir(path.Join(capDir, "files"), outputDir, vars, filesDirOptions{skipExisting: true})
	if err != nil {
		return nil, nil, err
	}
	return &RenderResult{FilesCreated: out.created}, out.skipped, nil
}

// PreviewCapabilityFiles reports the files RenderCapabilityFiles would create
// and skip, without writing anything. Templates are still executed, because a
// template that renders to empty content is not written — the preview would
// otherwise overreport. To preview an add, use PreviewAddedCapabilities.
func (r *Renderer) PreviewCapabilityFiles(capDir, outputDir string, vars map[string]interface{}) (created, skipped []string, err error) {
	out, err := r.renderFilesDir(path.Join(capDir, "files"), outputDir, vars, filesDirOptions{skipExisting: true, dryRun: true})
	if err != nil {
		return nil, nil, err
	}
	return out.created, out.skipped, nil
}

// removeEmptyDirs walks bottom-up and removes directories that contain no files.
func removeEmptyDirs(rootDir string) {
	// Collect dirs in reverse depth order (deepest first).
	var dirs []string
	// Best-effort cleanup: walk errors are intentionally ignored because failing
	// to remove empty directories is not a critical error.
	_ = filepath.WalkDir(rootDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Unreadable directory: skip its subtree and keep cleaning the rest.
			return filepath.SkipDir
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
