package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/guide"
	"github.com/diktahq/verikt/internal/provider"
	"github.com/spf13/cobra"
)

func newGuideCommand(_ *globalOptions) *cobra.Command {
	var (
		target      string
		catalogOnly bool
	)

	cmd := &cobra.Command{
		Use:   "guide",
		Short: "Generate AI agent architecture instructions",
		Long: `Generate architecture guidance files for AI coding agents.

Reads verikt.yaml from the current directory and generates instruction files
for Claude Code, Cursor (.cursor/rules/verikt.mdc), GitHub Copilot, Windsurf,
and AGENTS.md (read by Codex, Antigravity and other agents).

Always-loaded files stay small. Longer material (capability catalog, review
checklist, full layer detail) is written on demand to .verikt/references/. In
shared files such as AGENTS.md, verikt only rewrites the section between its
<!-- verikt:start --> and <!-- verikt:end --> markers, and refuses to touch a
file whose markers are malformed. A legacy .cursorrules keeps your own content;
only verikt's old section is removed from it.

Use --catalog-only in a project without verikt.yaml to generate only the
capability catalog. In a project with verikt.yaml it refuses and writes
nothing: read .verikt/references/capabilities.md instead.`,
		Example: `  verikt guide
  verikt guide --target claude
  verikt guide --target cursor
  verikt guide --target agents
  verikt guide --catalog-only`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGuide(cmd.ErrOrStderr(), target, catalogOnly)
		},
	}

	cmd.Flags().StringVar(&target, "target", "all", "Output target: "+strings.Join(guide.TargetNames(), ", "))
	cmd.Flags().BoolVar(&catalogOnly, "catalog-only", false, "Generate only the capability catalog, for a project without verikt.yaml")
	_ = cmd.RegisterFlagCompletionFunc("target", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return guide.TargetNames(), cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func runGuide(notices io.Writer, target string, catalogOnly bool) error {
	projectDir := "."

	if catalogOnly {
		return runGuideCatalogOnly(notices, projectDir, target)
	}

	cfgPath, err := config.FindVeriktYAML(".")
	if err != nil {
		return fmt.Errorf("no verikt.yaml found in current directory or parents: %w", err)
	}

	cfg, err := config.LoadVeriktYAML(cfgPath)
	if err != nil {
		return fmt.Errorf("load verikt.yaml: %w", err)
	}

	if err := validateConfigCapabilities(cfg); err != nil {
		return err
	}

	opts := guide.OptionsFromConfig(projectDir, cfg, target)
	opts.Notices = notices
	// The language provider's template FS enables pattern extraction; without
	// it the guide is still generated, just without capability patterns.
	if p, provErr := provider.Get(cfg.Language); provErr == nil {
		opts.TemplateFS = p.GetTemplateFS()
	}
	if err := guide.Generate(opts); err != nil {
		return err
	}

	fmt.Printf("Guide generated for target: %s\n", target)
	return nil
}

// runGuideCatalogOnly writes the capability catalog for a project with no
// verikt.yaml. In a configured project it refuses and writes nothing: the
// catalog-only guide replaces the architecture rules generated from
// verikt.yaml, and the project's own capability reference already holds the
// catalog.
func runGuideCatalogOnly(notices io.Writer, projectDir, target string) error {
	if cfgPath, err := config.FindVeriktYAML(projectDir); err == nil {
		return fmt.Errorf("--catalog-only is for projects without verikt.yaml, and %s configures this one; "+
			"nothing was written. Read .verikt/references/capabilities.md for the capability catalog, "+
			"or run `verikt guide` to regenerate it", cfgPath)
	}
	opts := guide.GenerateOptions{
		ProjectDir:  projectDir,
		Target:      target,
		CatalogOnly: true,
		Notices:     notices,
	}
	if p, err := provider.Get("go"); err == nil {
		opts.TemplateFS = p.GetTemplateFS()
	}
	if err := guide.Generate(opts); err != nil {
		return err
	}
	fmt.Printf("Guide generated (catalog-only) for target: %s\n", target)
	return nil
}
