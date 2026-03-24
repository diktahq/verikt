package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	// Register language providers via init() so they're available via provider.Get().
	_ "github.com/diktahq/verikt/providers/golang"
	_ "github.com/diktahq/verikt/providers/typescript"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type globalOptions struct {
	NoColor bool
	Output  string
}

func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	opts := &globalOptions{}

	cmd := &cobra.Command{
		Use:   "verikt",
		Short: "Your architecture, in every agent session",
		Long: `verikt — your architecture, in every agent session

  Get started:        verikt init           (detects greenfield or existing codebase)
  Add a service:      verikt new <name>     (scaffold a new service)
  Using an AI agent?  verikt init --ai      (agent conducts the setup)

  Validate:           verikt check
  Generate context:   verikt guide`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().BoolVar(&opts.NoColor, "no-color", false, "Disable colored output")
	cmd.PersistentFlags().StringVarP(&opts.Output, "output", "o", "terminal", "Output format: terminal|json|markdown")

	_ = cmd.RegisterFlagCompletionFunc("output", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"terminal", "json", "markdown"}, cobra.ShellCompDirectiveNoFileComp
	})

	cmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		switch opts.Output {
		case "terminal", "json", "markdown":
			return nil
		default:
			return fmt.Errorf("invalid --output value %q (expected terminal|json|markdown)", opts.Output)
		}
	}

	cmd.AddCommand(
		newNewCommand(opts),
		newAddCommand(opts),
		newInitCommand(opts),
		newAnalyzeCommand(opts),
		newCheckCommand(opts),
		newDiffCommand(opts),
		newGuideCommand(opts),
		newDecideCommand(opts),
		newVersionCommand(),
		newSetupCommand(),
	)

	return cmd
}
