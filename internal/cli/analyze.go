package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/diktahq/verikt/internal/analyzer/detector"
	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/output"
	"github.com/diktahq/verikt/internal/provider"
	"github.com/spf13/cobra"
)

type analyzeCommandOptions struct {
	Path     string
	Output   string
	Init     bool
	Language string
	NoColor  bool
}

func newAnalyzeCommand(rootOpts *globalOptions) *cobra.Command {
	opts := &analyzeCommandOptions{}

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze an existing codebase",
		Long:  "Analyze project architecture, framework choices, and conventions for a codebase.",
		Example: `  verikt analyze
  verikt analyze --path . --output json
  verikt analyze --init`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.Output == "" {
				opts.Output = rootOpts.Output
			}
			opts.NoColor = rootOpts.NoColor
			return runAnalyze(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Path, "path", ".", "Path to project")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "Output format: terminal|json|markdown")
	cmd.Flags().BoolVar(&opts.Init, "init", false, "Generate verikt.yaml from analysis")
	cmd.Flags().StringVar(&opts.Language, "language", "", "Force language")

	return cmd
}

func runAnalyze(ctx context.Context, opts *analyzeCommandOptions) error {
	language := strings.TrimSpace(opts.Language)
	if language == "" {
		detected, conf, err := detector.DetectLanguage(opts.Path)
		if err != nil {
			return err
		}
		language = detected
		if language == "unknown" {
			return fmt.Errorf("could not detect language from %s; use --language", opts.Path)
		}
		fmt.Printf("Detected language: %s (confidence %.2f)\n", language, conf)
	}

	providerImpl, err := provider.Get(language)
	if err != nil {
		return err
	}

	result, err := providerImpl.Analyze(ctx, provider.AnalyzeRequest{Path: opts.Path})
	if err != nil {
		return err
	}

	formatter, err := output.NewFormatter(opts.Output, opts.NoColor)
	if err != nil {
		return err
	}
	formatted, err := formatter.Format(result)
	if err != nil {
		return err
	}
	fmt.Println(formatted)

	if opts.Init {
		archCfg := config.DefaultVeriktConfig(language, result.Architecture.Pattern)
		path := filepath.Join(opts.Path, "verikt.yaml")
		if err := config.SaveVeriktYAML(path, archCfg); err != nil {
			return err
		}
		fmt.Printf("Generated %s\n", path)
	}

	return nil
}
