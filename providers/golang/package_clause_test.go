package golang

import (
	"bufio"
	"bytes"
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"
	"text/template"

	"github.com/diktahq/verikt/internal/scaffold"
)

// Capability templates are written for the hexagonal layout and moved into
// each architecture's directories by its path_mappings. A file that names its
// package with a literal (package httphandler) compiles only where the mapped
// directory happens to carry that name; in every other architecture it clashes
// with the files already there and the scaffold does not build (#28).
//
// These tests check every Go template of every capability, for every
// architecture, statically: no rendering of whole files, no network, no go
// build. They resolve each file's destination exactly as the renderer does
// (RenderPath, then the architecture's PathMapper) and read its package clause
// with ArchPaths substituted.

// archPkgExceptions lists capability templates that sit in an ArchPaths
// directory but keep a literal package name that differs from the directory's
// ArchPaths package. They compile (they are alone in their directory), but the
// package name does not match the directory. Each entry must still be needed:
// TestPackageClauseExceptionsAreStillNeeded fails once one is fixed.
//
// Key: capability name + "/" + template path inside files/.
var archPkgExceptions = map[string]string{}

// goTemplateFile is one Go source template and where it lands.
type goTemplateFile struct {
	origin string // capability/<name>/<rel> or architecture/<name>/<rel>
	key    string // <capability>/<rel>, for archPkgExceptions
	dstDir string // destination directory in the generated project
	pkg    string // package clause, ArchPaths substituted
}

func TestCapabilityPackageClausesAgreeWithinEachDirectory(t *testing.T) {
	for _, arch := range discoveredTemplates(t, "architectures", "manifest.yaml") {
		t.Run(arch, func(t *testing.T) {
			files := goFilesForArchitecture(t, arch)
			byDir := map[string]map[string][]string{} // dir -> package -> origins
			for _, f := range files {
				pkg := strings.TrimSuffix(f.pkg, "_test") // external test packages share the directory
				if byDir[f.dstDir] == nil {
					byDir[f.dstDir] = map[string][]string{}
				}
				byDir[f.dstDir][pkg] = append(byDir[f.dstDir][pkg], f.origin)
			}
			for dir, pkgs := range byDir {
				if len(pkgs) < 2 {
					continue
				}
				names := make([]string, 0, len(pkgs))
				for p := range pkgs {
					names = append(names, p)
				}
				sort.Strings(names)
				var b strings.Builder
				for _, p := range names {
					b.WriteString("\n  package " + p + ": " + strings.Join(pkgs[p], ", "))
				}
				t.Errorf("%s: directory %q gets files from more than one package; it will not compile:%s",
					arch, displayDir(dir), b.String())
			}
		})
	}
}

func TestCapabilityPackageClausesMatchArchPaths(t *testing.T) {
	for _, arch := range discoveredTemplates(t, "architectures", "manifest.yaml") {
		t.Run(arch, func(t *testing.T) {
			dirPkg := archPathsPackageByDir(archPaths(t, arch))
			for _, f := range goFilesForArchitecture(t, arch) {
				want, ok := dirPkg[f.dstDir]
				if !ok || f.key == "" || exceptionFor(f.key) != "" {
					continue
				}
				if got := strings.TrimSuffix(f.pkg, "_test"); got != want {
					t.Errorf("%s: %s lands in %q and declares package %s; that directory's package is %s "+
						"(use the ArchPaths ...Pkg key, e.g. {{index .ArchPaths \"HTTPHandlerPkg\"}})",
						arch, f.origin, displayDir(f.dstDir), f.pkg, want)
				}
			}
		})
	}
}

func TestPackageClauseExceptionsAreStillNeeded(t *testing.T) {
	used := map[string]bool{}
	for _, arch := range discoveredTemplates(t, "architectures", "manifest.yaml") {
		dirPkg := archPathsPackageByDir(archPaths(t, arch))
		for _, f := range goFilesForArchitecture(t, arch) {
			prefix := exceptionFor(f.key)
			if prefix == "" {
				continue
			}
			if want, ok := dirPkg[f.dstDir]; ok && strings.TrimSuffix(f.pkg, "_test") != want {
				used[prefix] = true
			}
		}
	}
	for prefix := range archPkgExceptions {
		if !used[prefix] {
			t.Errorf("archPkgExceptions entry %q no longer matches a mismatched package clause; remove it", prefix)
		}
	}
}

func exceptionFor(key string) string {
	for prefix := range archPkgExceptions {
		if strings.HasPrefix(key, prefix) {
			return prefix
		}
	}
	return ""
}

func displayDir(dir string) string {
	if dir == "." {
		return "<project root>"
	}
	return dir
}

func architectureManifest(t *testing.T, arch string) *scaffold.Manifest {
	t.Helper()
	data, err := fs.ReadFile(templatesFS, path.Join("templates", "architectures", arch, "manifest.yaml"))
	if err != nil {
		t.Fatalf("read %s manifest: %v", arch, err)
	}
	m, err := scaffold.ParseManifest(data)
	if err != nil {
		t.Fatalf("parse %s manifest: %v", arch, err)
	}
	return m
}

func archPaths(t *testing.T, arch string) map[string]string {
	t.Helper()
	return scaffold.NewPathMapper(architectureManifest(t, arch).PathMappings).ArchPaths()
}

// archPathsPackageByDir inverts ArchPaths: directory -> the package name the
// templates must use there (the <Key>Pkg value).
func archPathsPackageByDir(ap map[string]string) map[string]string {
	out := map[string]string{}
	for key, dir := range ap {
		if strings.HasSuffix(key, "Pkg") {
			continue
		}
		if pkg, ok := ap[key+"Pkg"]; ok {
			out[dir] = pkg
		}
	}
	return out
}

// goFilesForArchitecture lists every Go template the architecture and all
// capabilities could write into a project of that architecture.
func goFilesForArchitecture(t *testing.T, arch string) []goTemplateFile {
	t.Helper()
	m := architectureManifest(t, arch)
	pm := scaffold.NewPathMapper(m.PathMappings)
	vars := map[string]interface{}{
		"ServiceName": "svc",
		"ModulePath":  "example.com/svc",
		"ArchPaths":   pm.ArchPaths(),
	}

	// Architecture files use native paths; only capability files are mapped.
	out := goFilesIn(t, path.Join("templates", "architectures", arch, "files"),
		"architecture/"+arch, "", vars, nil)
	for _, c := range discoveredTemplates(t, "capabilities", "capability.yaml") {
		out = append(out, goFilesIn(t, path.Join("templates", "capabilities", c, "files"),
			"capability/"+c, c, vars, pm)...)
	}
	return out
}

func goFilesIn(t *testing.T, root, originPrefix, capName string, vars map[string]interface{}, pm *scaffold.PathMapper) []goTemplateFile {
	t.Helper()
	if _, err := fs.Stat(templatesFS, root); err != nil {
		return nil
	}
	var out []goTemplateFile
	err := fs.WalkDir(templatesFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && p != root {
				return fs.SkipDir // agent tooling leftovers, never shipped
			}
			return nil
		}
		rel := strings.TrimPrefix(p, root+"/")
		if !strings.HasSuffix(strings.TrimSuffix(rel, ".tmpl"), ".go") {
			return nil
		}
		dst, err := scaffold.RenderPath(rel, vars)
		if err != nil {
			t.Fatalf("render path %s: %v", p, err)
		}
		if pm != nil {
			dst = pm.Map(dst)
		}
		pkg := packageClause(t, p, strings.HasSuffix(rel, ".tmpl"), vars)
		if pkg == "" {
			return nil
		}
		f := goTemplateFile{
			origin: originPrefix + "/" + rel,
			dstDir: path.Dir(dst),
			pkg:    pkg,
		}
		if capName != "" {
			f.key = capName + "/" + rel
		}
		out = append(out, f)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// packageClause returns the package name a Go template declares, with any
// template actions on that line (ArchPaths lookups) executed. It returns ""
// for a file excluded from every build.
func packageClause(t *testing.T, p string, isTemplate bool, vars map[string]interface{}) string {
	t.Helper()
	data, err := fs.ReadFile(templatesFS, p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		// A tools.go pinned behind //go:build tools never takes part in a build,
		// so it may share a directory with another package (the flat root).
		if line == "//go:build tools" || line == "//go:build ignore" {
			return ""
		}
		if !strings.HasPrefix(line, "package ") {
			continue
		}
		if isTemplate && strings.Contains(line, "{{") {
			tmpl, err := template.New(p).Parse(line)
			if err != nil {
				t.Fatalf("parse package clause of %s: %v", p, err)
			}
			var b bytes.Buffer
			if err := tmpl.Execute(&b, vars); err != nil {
				t.Fatalf("execute package clause of %s: %v", p, err)
			}
			line = b.String()
		}
		return strings.TrimSpace(strings.TrimPrefix(line, "package "))
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", p, err)
	}
	return ""
}
