package detector

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/diktahq/verikt/internal/provider"
	"golang.org/x/tools/go/packages"
)

func DetectConventions(pkgs []*packages.Package) provider.ConventionResults {
	return provider.ConventionResults{
		ErrorHandling: detectErrorHandling(pkgs),
		Logging:       detectLogging(pkgs),
		Config:        detectConfig(pkgs),
		Testing:       detectTesting(pkgs),
	}
}

func detectErrorHandling(pkgs []*packages.Package) provider.ConventionFinding {
	sentinel := 0
	typed := 0
	wrapped := 0

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.ValueSpec:
					for _, name := range n.Names {
						if strings.HasPrefix(name.Name, "Err") {
							sentinel++
						}
					}
				case *ast.TypeSpec:
					if strings.HasSuffix(n.Name.Name, "Error") {
						typed++
					}
				case *ast.CallExpr:
					if sel, ok := n.Fun.(*ast.SelectorExpr); ok {
						if x, ok := sel.X.(*ast.Ident); ok && x.Name == "fmt" && sel.Sel.Name == "Errorf" {
							for _, arg := range n.Args {
								if lit, ok := arg.(*ast.BasicLit); ok && strings.Contains(lit.Value, "%w") {
									wrapped++
								}
							}
						}
					}
				}
				return true
			})
		}
	}

	pattern := "minimal"
	confidence := 0.3
	evidence := []string{}
	max := sentinel
	if typed > max {
		max = typed
		pattern = "typed"
		confidence = 0.8
	}
	if wrapped > max {
		_ = max // consumed by comparison above
		pattern = "wrapped"
		confidence = 0.8
	}
	if sentinel > 0 {
		evidence = append(evidence, "sentinel errors detected")
		if pattern == "minimal" {
			pattern = "sentinel"
			confidence = 0.7
		}
	}
	if typed > 0 {
		evidence = append(evidence, "typed errors detected")
	}
	if wrapped > 0 {
		evidence = append(evidence, "error wrapping with %w detected")
	}
	return provider.ConventionFinding{Pattern: pattern, Confidence: confidence, Evidence: evidence}
}

func detectLogging(pkgs []*packages.Package) provider.ConventionFinding {
	slog := 0
	zap := 0
	zerolog := 0
	unstructured := 0
	structured := 0

	for _, pkg := range pkgs {
		for importPath := range pkg.Imports {
			switch {
			case strings.Contains(importPath, "log/slog"):
				slog++
			case strings.Contains(importPath, "go.uber.org/zap"):
				zap++
			case strings.Contains(importPath, "github.com/rs/zerolog"):
				zerolog++
			}
		}
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if sel.Sel.Name == "With" {
					structured++
				}
				if sel.Sel.Name == "Printf" || sel.Sel.Name == "Println" {
					unstructured++
				}
				return true
			})
		}
	}

	pattern := "unstructured"
	if structured >= unstructured {
		pattern = "structured"
	}
	library := "stdlib"
	switch {
	case slog > 0:
		library = "slog"
	case zap > 0:
		library = "zap"
	case zerolog > 0:
		library = "zerolog"
	}

	return provider.ConventionFinding{
		Pattern:    library + "/" + pattern,
		Confidence: 0.8,
		Evidence:   []string{"detected logging library: " + library},
	}
}

func detectConfig(pkgs []*packages.Package) provider.ConventionFinding {
	hits := map[string]int{"viper": 0, "koanf": 0, "godotenv": 0, "envconfig": 0}
	tags := 0
	for _, pkg := range pkgs {
		for importPath := range pkg.Imports {
			switch {
			case strings.Contains(importPath, "spf13/viper"):
				hits["viper"]++
			case strings.Contains(importPath, "knadh/koanf"):
				hits["koanf"]++
			case strings.Contains(importPath, "godotenv"):
				hits["godotenv"]++
			case strings.Contains(importPath, "envconfig"):
				hits["envconfig"]++
			}
		}
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(node ast.Node) bool {
				field, ok := node.(*ast.Field)
				if !ok || field.Tag == nil {
					return true
				}
				tag := field.Tag.Value
				if strings.Contains(tag, "mapstructure") || strings.Contains(tag, "env") || strings.Contains(tag, "yaml") {
					tags++
				}
				return true
			})
		}
	}

	best := "env"
	bestScore := 0
	for k, v := range hits {
		if v > bestScore {
			best = k
			bestScore = v
		}
	}
	if bestScore == 0 && tags > 0 {
		best = "struct-tags"
	}
	confidence := 0.6
	if bestScore > 0 || tags > 0 {
		confidence = 0.8
	}
	return provider.ConventionFinding{Pattern: best, Confidence: confidence, Evidence: []string{"config tags: " + strconv.Itoa(tags)}}
}

// testFileStats summarises the _test.go files found alongside loaded packages.
type testFileStats struct {
	files       int
	tableDriven int
	bdd         int
}

// countRunCalls counts calls to a .Run method — the signal for subtests, and
// therefore a table-driven suite.
func countRunCalls(file *ast.File) int {
	count := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" {
			count++
		}
		return true
	})
	return count
}

// scanTestFiles parses the _test.go files in the given directories. Unparseable
// files are counted but contribute no signal — a syntax error in one test file
// should not discard the statistics for the rest.
func scanTestFiles(dirs map[string]bool) testFileStats {
	var stats testFileStats
	fset := token.NewFileSet()

	for dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			stats.files++

			file, parseErr := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
			if parseErr != nil {
				continue
			}
			stats.tableDriven += countRunCalls(file)
			for _, imp := range file.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if strings.Contains(path, "ginkgo") || strings.Contains(path, "godog") {
					stats.bdd++
				}
			}
		}
	}

	return stats
}

func detectTesting(pkgs []*packages.Package) provider.TestingFinding {
	totalGoFiles := 0
	tableDriven := 0
	bdd := 0
	dirs := map[string]bool{}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			filename := pkg.Fset.Position(file.Pos()).Filename
			if strings.HasSuffix(filename, ".go") {
				totalGoFiles++
			}
			if dir := filepath.Dir(filename); dir != "." {
				dirs[dir] = true
			}
			// Subtests are only counted from test files (see scanTestFiles):
			// ".Run(" in production code is some other method entirely.
		}
		// BDD libraries are counted from test files only (see scanTestFiles). Counting
		// pkg.Imports mixed a production signal into a test statistic: a project with
		// any test file and ginkgo imported by a non-test helper was reported as bdd.
	}

	// packages.Load runs with Tests disabled, so no _test.go file appears in
	// pkg.Syntax — test files have to be read from the package directories or
	// the count is always 0 and t.Run usage is never seen.
	tests := scanTestFiles(dirs)
	testFiles := tests.files
	totalGoFiles += tests.files
	tableDriven += tests.tableDriven
	bdd += tests.bdd

	pattern := "minimal"
	confidence := 0.4
	evidence := []string{}
	// A testing pattern cannot be inferred from a project with no test files.
	// Both signals can appear in production code — a Run method, or a BDD
	// library imported by a helper — and reporting a pattern from those alone
	// produced a high-confidence answer about tests that do not exist.
	if testFiles > 0 {
		if tableDriven > 0 {
			pattern = "table-driven"
			confidence = 0.85
			evidence = append(evidence, "found t.Run usage")
		}
		if bdd > 0 {
			pattern = "bdd"
			confidence = 0.9
			evidence = append(evidence, "found BDD test libraries")
		}
	}

	return provider.TestingFinding{
		Pattern:      pattern,
		Confidence:   confidence,
		Evidence:     evidence,
		TestFiles:    testFiles,
		TotalGoFiles: totalGoFiles,
	}
}
