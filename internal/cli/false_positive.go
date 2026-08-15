package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// newFalsePositiveCommand reports a finding that should not have fired.
//
// A user who hits a false positive has two bad options otherwise: waive it,
// which the tool's own guidance calls the wrong answer because every user with
// the same shape of code sees the same thing; or rewrite correct code to avoid a
// pattern. One project reworded English error messages to stop a detector
// firing. This turns that dead end into a report, and the reports become
// fixtures (INV-005).
func newFalsePositiveCommand() *cobra.Command {
	var (
		detector string
		file     string
		line     int
		why      string
	)

	cmd := &cobra.Command{
		Use:   "false-positive",
		Short: "Report a finding that should not have fired",
		Long: `Produce a bug report for a finding you believe is wrong.

A false positive is a defect in verikt, not something to waive: waiving it hides
the problem for you and leaves it in place for everyone with the same shape of
code. This assembles the detail needed to fix it — the detector, the code it
fired on, and your reason — so it can become a regression test.`,
		Example: `  verikt false-positive --detector sql_concatenation --file internal/adapter/tables.go --line 9 \
    --why "no database in this module; 'where' is a location and 'delete' is English"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if detector == "" || file == "" {
				return fmt.Errorf("--detector and --file are required")
			}
			report, err := buildFalsePositiveReport(detector, file, line, why)
			if err != nil {
				return err
			}
			fmt.Print(report)
			return nil
		},
	}

	cmd.Flags().StringVar(&detector, "detector", "", "Detector that fired (e.g. sql_concatenation)")
	cmd.Flags().StringVar(&file, "file", "", "File it fired on")
	cmd.Flags().IntVar(&line, "line", 0, "Line it fired on")
	cmd.Flags().StringVar(&why, "why", "", "Why the finding is wrong")

	return cmd
}

// buildFalsePositiveReport assembles a report with the offending code included.
//
// The code is what makes a report actionable — a description of a false positive
// cannot be turned into a regression test, and asking for it later costs a round
// trip.
func buildFalsePositiveReport(detector, file string, line int, why string) (string, error) {
	var b strings.Builder

	fmt.Fprintf(&b, "## False positive: %s\n\n", detector)
	fmt.Fprintf(&b, "**Detector:** `%s`\n", detector)
	fmt.Fprintf(&b, "**Location:** `%s", file)
	if line > 0 {
		fmt.Fprintf(&b, ":%d", line)
	}
	fmt.Fprintf(&b, "`\n")
	fmt.Fprintf(&b, "**verikt:** %s (%s/%s)\n\n", version, runtime.GOOS, runtime.GOARCH)

	if why != "" {
		fmt.Fprintf(&b, "**Why this is wrong:** %s\n\n", why)
	} else {
		fmt.Fprintf(&b, "**Why this is wrong:** _(describe what makes the risk unreachable here)_\n\n")
	}

	snippet, err := codeSnippet(file, line)
	if err != nil {
		fmt.Fprintf(&b, "_Could not read %s: %v_\n\n", file, err)
	} else {
		fmt.Fprintf(&b, "```go\n%s```\n\n", snippet)
	}

	b.WriteString("---\n\n")
	b.WriteString("Report at https://github.com/diktahq/verikt/issues — a false positive is a\n")
	b.WriteString("defect, and this report becomes a regression test (INV-005).\n")

	return b.String(), nil
}

// codeSnippet returns the lines around line, or the whole file when it is short.
func codeSnippet(file string, line int) (string, error) {
	data, err := os.ReadFile(filepath.Clean(file))
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")

	start, end := 0, len(lines)
	if line > 0 {
		start = max(0, line-6)
		end = min(len(lines), line+5)
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		marker := "  "
		if i+1 == line {
			marker = "> "
		}
		fmt.Fprintf(&b, "%s%4d | %s\n", marker, i+1, lines[i])
	}
	return b.String(), nil
}
