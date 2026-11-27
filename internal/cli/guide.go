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

Use --catalog-only to generate only the capability catalog without requiring
an verikt.yaml configuration file.`,
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
	cmd.Flags().BoolVar(&catalogOnly, "catalog-only", false, "Generate only the capability catalog (no verikt.yaml required)")
	_ = cmd.RegisterFlagCompletionFunc("target", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return guide.TargetNames(), cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func runGuide(notices io.Writer, target string, catalogOnly bool) error {
	projectDir := "."

	if catalogOnly {
		// Catalog-only mode: no verikt.yaml required, but load capabilities if present
		// so the dynamic "Your Stack: Specific Warnings" section fires.
		p, provErr := provider.Get("go")
		opts := guide.GenerateOptions{
			ProjectDir:  projectDir,
			Target:      target,
			CatalogOnly: true,
			Notices:     notices,
		}
		if provErr == nil {
			opts.TemplateFS = p.GetTemplateFS()
		}
		if cfgPath, cfgErr := config.FindVeriktYAML("."); cfgErr == nil {
			if cfg, cfgErr := config.LoadVeriktYAML(cfgPath); cfgErr == nil {
				opts.Capabilities = cfg.Capabilities
			}
		}
		if err := guide.Generate(opts); err != nil {
			return err
		}
		fmt.Printf("Guide generated (catalog-only) for target: %s\n", target)
		return nil
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
