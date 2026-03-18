package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

type setupFlags struct {
	uninstall bool
	check     bool
	force     bool
}

var sentinelPattern = regexp.MustCompile(`<!-- archway:global:v([^>]+) -->`)

func newSetupCommand() *cobra.Command {
	flags := &setupFlags{}

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Register archway with installed AI agents",
		Long:  "Detect installed AI agents and write global architecture guidance files to each one.",
		Example: `  archway setup             # detect agents, write global files
  archway setup --check     # report status without writing
  archway setup --uninstall # remove all archway global files
  archway setup --force     # overwrite even if version matches`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd, flags)
		},
	}

	cmd.Flags().BoolVar(&flags.uninstall, "uninstall", false, "Remove archway global files from all agents")
	cmd.Flags().BoolVar(&flags.check, "check", false, "Report status without writing (exit 1 if any agent is stale)")
	cmd.Flags().BoolVar(&flags.force, "force", false, "Overwrite even if version matches")

	return cmd
}

func runSetup(cmd *cobra.Command, flags *setupFlags) error {
	agents, err := detectInstalledAgents()
	if err != nil {
		return fmt.Errorf("detecting agents: %w", err)
	}

	if len(agents) == 0 {
		cmd.Println("No supported AI agents detected.")
		cmd.Println()
		cmd.Println("To register archway with your AI agent, install one of these first:")
		cmd.Println("  - Claude Code: https://claude.ai/download")
		cmd.Println("  - Cursor: https://cursor.com")
		cmd.Println("  - Windsurf: https://windsurf.com")
		cmd.Println()
		cmd.Println("Then run: archway setup")
		return nil
	}

	if flags.uninstall {
		return runUninstall(cmd, agents)
	}

	if flags.check {
		return runSetupCheck(cmd, agents)
	}

	return runWrite(cmd, agents, flags.force)
}

func runWrite(cmd *cobra.Command, agents []AIAgent, force bool) error {
	content := GlobalRulesContent(version)
	written := 0

	for _, agent := range agents {
		rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)

		if !force {
			existing, err := os.ReadFile(rulesFile) //nolint:gosec
			if err == nil {
				installedVersion := extractSentinelVersion(string(existing))
				if installedVersion == version {
					cmd.Printf("  = %s \u2014 already current (v%s)\n", agent.Name, version)
					continue
				}
			}
		}

		if err := os.MkdirAll(agent.RulesDir, 0o755); err != nil {
			return fmt.Errorf("creating rules dir for %s: %w", agent.Name, err)
		}

		if err := os.WriteFile(rulesFile, []byte(content), 0o644); err != nil { //nolint:gosec
			return fmt.Errorf("writing rules file for %s: %w", agent.Name, err)
		}

		cmd.Printf("  \u2713 %s \u2014 wrote %s\n", agent.Name, rulesFile)
		written++
	}

	cmd.Println()
	if written > 0 {
		cmd.Printf("Registered with %d agent(s).\n", written)
	} else {
		cmd.Println("All agents already current.")
	}

	return nil
}

func runSetupCheck(cmd *cobra.Command, agents []AIAgent) error {
	stale := 0

	for _, agent := range agents {
		rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)

		existing, err := os.ReadFile(rulesFile) //nolint:gosec
		if err != nil {
			cmd.Printf("  \u2717 %s \u2014 not installed\n", agent.Name)
			stale++
			continue
		}

		installedVersion := extractSentinelVersion(string(existing))
		if installedVersion == version {
			cmd.Printf("  \u2713 %s \u2014 current (v%s)\n", agent.Name, version)
		} else {
			cmd.Printf("  \u2717 %s \u2014 stale (v%s \u2192 v%s)\n", agent.Name, installedVersion, version)
			stale++
		}
	}

	if stale > 0 {
		cmd.Println()
		cmd.Printf("%d agent(s) need update. Run: archway setup\n", stale)
		return fmt.Errorf("stale")
	}

	return nil
}

func runUninstall(cmd *cobra.Command, agents []AIAgent) error {
	for _, agent := range agents {
		rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)

		if err := os.Remove(rulesFile); err != nil {
			if os.IsNotExist(err) {
				cmd.Printf("  = %s \u2014 not installed (skipped)\n", agent.Name)
				continue
			}
			return fmt.Errorf("removing %s: %w", rulesFile, err)
		}

		cmd.Printf("  \u2713 %s \u2014 removed %s\n", agent.Name, rulesFile)
	}

	return nil
}

func extractSentinelVersion(content string) string {
	matches := sentinelPattern.FindStringSubmatch(content)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}
