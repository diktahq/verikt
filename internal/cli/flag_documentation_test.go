package cli

import (
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagPattern matches long flags referenced in help text, e.g. "--dry-run".
var flagPattern = regexp.MustCompile(`--[a-z][a-z0-9-]*`)

// TestHelpTextOnlyReferencesRegisteredFlags walks the whole command tree and
// asserts every long flag mentioned in a command's Long description or Example
// block is actually registered on that command.
//
// `verikt add --help` documented `--dry-run` while newAddCommand registered no
// flags at all, so the documented invocation failed with "unknown flag". Help
// text is the contract users copy from; it must not describe flags that do not
// exist.
func TestHelpTextOnlyReferencesRegisteredFlags(t *testing.T) {
	forEachCommand(t, newRootCommand(), func(t *testing.T, cmd *cobra.Command) {
		for _, source := range []struct{ label, text string }{
			{label: "Long", text: cmd.Long},
			{label: "Example", text: cmd.Example},
		} {
			for _, line := range strings.Split(source.text, "\n") {
				mentioned := flagPattern.FindAllString(line, -1)
				if len(mentioned) == 0 {
					continue
				}
				// A line may document a subcommand invocation, as the root's
				// Long does with "verikt init --ai" — check the flag against the
				// command that line actually invokes.
				target := commandInvokedBy(cmd, line)
				registered := registeredFlagNames(target)

				for _, name := range mentioned {
					name = strings.TrimPrefix(name, "--")
					if !registered[name] {
						t.Errorf("%s %s documents %q, but --%s is not registered on %q",
							cmd.CommandPath(), source.label, strings.TrimSpace(line), name, target.CommandPath())
					}
				}
			}
		}
	})
}

// commandInvokedBy resolves the command a documentation line invokes, following
// any subcommand path after the root name. It falls back to cmd when the line
// names no command.
func commandInvokedBy(cmd *cobra.Command, line string) *cobra.Command {
	root := cmd.Root()
	fields := strings.Fields(line)

	rootIdx := -1
	for i, f := range fields {
		if f == root.Name() {
			rootIdx = i
			break
		}
	}
	if rootIdx == -1 {
		return cmd
	}

	target := root
	for _, arg := range fields[rootIdx+1:] {
		if strings.HasPrefix(arg, "-") {
			break
		}
		child := childNamed(target, arg)
		if child == nil {
			break
		}
		target = child
	}
	return target
}

// childNamed returns the subcommand of parent with the given name or alias.
func childNamed(parent *cobra.Command, name string) *cobra.Command {
	for _, sub := range parent.Commands() {
		if sub.Name() == name {
			return sub
		}
		for _, alias := range sub.Aliases {
			if alias == name {
				return sub
			}
		}
	}
	return nil
}

// registeredFlagNames returns every long flag name accepted by cmd, including
// inherited persistent flags from its parents.
func registeredFlagNames(cmd *cobra.Command) map[string]bool {
	names := map[string]bool{}
	collect := func(f *pflag.Flag) { names[f.Name] = true }

	cmd.Flags().VisitAll(collect)
	cmd.PersistentFlags().VisitAll(collect)
	cmd.InheritedFlags().VisitAll(collect)
	for parent := cmd.Parent(); parent != nil; parent = parent.Parent() {
		parent.PersistentFlags().VisitAll(collect)
	}

	return names
}

// forEachCommand runs fn against cmd and every subcommand, as a subtest named
// after the command path.
func forEachCommand(t *testing.T, cmd *cobra.Command, fn func(*testing.T, *cobra.Command)) {
	t.Helper()
	t.Run(cmd.Name(), func(t *testing.T) {
		fn(t, cmd)
		for _, sub := range cmd.Commands() {
			forEachCommand(t, sub, fn)
		}
	})
}
