package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print version info",
		Long:    "Print verikt version, commit, and build date information.",
		Example: "verikt version",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Printf("verikt version %s\ncommit: %s\nbuilt: %s\n", version, commit, date)
			return nil
		},
	}
}
