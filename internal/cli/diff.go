package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dcsg/archway/internal/config"
	"github.com/dcsg/archway/internal/diff"
	"github.com/dcsg/archway/internal/provider"
	"github.com/dcsg/archway/internal/scaffold"
	"github.com/spf13/cobra"
)

type diffFlags struct {
	projectPath string
}

func newDiffCommand(opts *globalOptions) *cobra.Command {
	flags := &diffFlags{}

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show structural drift between archway.yaml and files on disk",
		Long: `Diff compares the architecture and capabilities declared in archway.yaml
against the files that actually exist on disk. Like 'terraform plan' for code structure.

Reports which expected files are present, partially present, or fully missing.`,
		Example: `  archway diff
  archway diff --path ./my-service
  archway diff -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiff(opts, flags)
		},
	}

	cmd.Flags().StringVar(&flags.projectPath, "path", ".", "Project path to diff")

	return cmd
}

func runDiff(opts *globalOptions, flags *diffFlags) error {
	projectPath := flags.projectPath

	archwayPath, err := config.FindArchwayYAML(projectPath)
	if err != nil {
		return fmt.Errorf("no archway.yaml found in %s (or parent directories)", projectPath)
	}

	cfg, err := config.LoadArchwayYAML(archwayPath)
	if err != nil {
		return fmt.Errorf("load archway.yaml: %w", err)
	}

	// Get the language provider for template FS access.
	p, err := provider.Get(cfg.Language)
	if err != nil {
		return fmt.Errorf("get provider for %q: %w", cfg.Language, err)
	}
	templateFS := p.GetTemplateFS()

	// Infer ServiceName from directory name, ModulePath from go.mod.
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	serviceName := filepath.Base(absPath)
	modulePath := fmt.Sprintf("example.com/%s", serviceName)
	if goModData, readErr := os.ReadFile(filepath.Join(projectPath, "go.mod")); readErr == nil {
		for _, line := range strings.Split(string(goModData), "\n") {
			if strings.HasPrefix(line, "module ") {
				modulePath = strings.TrimSpace(strings.TrimPrefix(line, "module "))
				break
			}
		}
	}

	// Build the composition plan to get vars, partials, and directory paths.
	vars := map[string]interface{}{
		"ServiceName": serviceName,
		"ModulePath":  modulePath,
	}
	plan, err := scaffold.ComposeProject(templateFS, cfg.Architecture, cfg.Capabilities, vars)
	if err != nil {
		return fmt.Errorf("compose project: %w", err)
	}

	result, err := diff.Run(templateFS, plan, projectPath)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}

	switch opts.Output {
	case "json":
		return printDiffJSON(result)
	case "markdown":
		printDiffMarkdown(result)
	default:
		printDiffTerminal(result)
	}

	return nil
}

func printDiffTerminal(r *diff.Result) {
	capCount := len(r.CapabilityDiffs)
	fmt.Printf("\nArchway Diff — %s + %d capabilities\n", r.Architecture, capCount-1)
	fmt.Println(strings.Repeat("═", 55))

	for _, d := range r.CapabilityDiffs {
		switch d.Status {
		case "ok":
			fmt.Printf("  ✓ %-18s all files present\n", d.Name)
		case "partial":
			fmt.Printf("  ✗ %-18s %d missing files\n", d.Name, len(d.MissingFiles))
			for _, f := range d.MissingFiles {
				fmt.Printf("    - %s\n", f)
			}
		case "missing":
			fmt.Printf("  ✗ %-18s all %d files missing\n", d.Name, len(d.MissingFiles))
			for _, f := range d.MissingFiles {
				fmt.Printf("    - %s\n", f)
			}
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("═", 55))
	fmt.Printf("\nSummary: %d/%d capabilities fully present | drift score: %.2f\n",
		r.Summary.FullyPresent, r.Summary.TotalCapabilities, r.Summary.DriftScore)
}

func printDiffJSON(r *diff.Result) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func printDiffMarkdown(r *diff.Result) {
	fmt.Printf("# Archway Diff — %s\n\n", r.Architecture)
	fmt.Println("| Capability | Status | Missing |")
	fmt.Println("|------------|--------|---------|")

	for _, d := range r.CapabilityDiffs {
		missing := "-"
		if len(d.MissingFiles) > 0 {
			missing = fmt.Sprintf("%d files", len(d.MissingFiles))
		}
		fmt.Printf("| %s | %s | %s |\n", d.Name, d.Status, missing)
	}

	fmt.Printf("\n**Drift score:** %.2f\n", r.Summary.DriftScore)
}
