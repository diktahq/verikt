package cli

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/diktahq/verikt/internal/guide"
	"github.com/spf13/cobra"
)

//go:embed skills/verikt-init.md
var skillVeriktInit string

type setupFlags struct {
	uninstall   bool
	check       bool
	force       bool
	codex       bool
	antigravity bool
}

// optIn returns the opt-in agents the user explicitly requested.
func (f *setupFlags) optIn() map[string]bool {
	return map[string]bool{"codex": f.codex, "antigravity": f.antigravity}
}

var sentinelPattern = regexp.MustCompile(`<!-- verikt:global:v([^>]+) -->`)

// globalMarkers delimit verikt's section in an agent's shared global
// instructions file (Codex ~/.codex/AGENTS.md, Antigravity ~/.gemini/GEMINI.md).
var globalMarkers = guide.SectionMarkers{
	Start: "<!-- verikt:global:start -->",
	End:   "<!-- verikt:global:end -->",
}

func newSetupCommand() *cobra.Command {
	flags := &setupFlags{}

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Register verikt with installed AI agents",
		Long: `Detect installed AI agents and write global architecture guidance files to each one.

Claude Code, Cursor and Windsurf are configured automatically when their config
directory exists. Codex and Antigravity keep their global instructions in a single
file you also edit (~/.codex/AGENTS.md, ~/.gemini/GEMINI.md), so they are opt-in:
pass --codex or --antigravity. Only verikt's own marked section of that file is
written, and an agent's config directory is never created.`,
		Example: `  verikt setup                 # detect agents, write global files
  verikt setup --codex         # also register with Codex (~/.codex/AGENTS.md)
  verikt setup --antigravity   # also register with Antigravity (~/.gemini/GEMINI.md)
  verikt setup --check         # report status without writing
  verikt setup --uninstall     # remove all verikt global files
  verikt setup --force         # overwrite even if version matches`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd, flags)
		},
	}

	cmd.Flags().BoolVar(&flags.uninstall, "uninstall", false, "Remove verikt global files from all agents")
	cmd.Flags().BoolVar(&flags.check, "check", false, "Report status without writing (exit 1 if any agent is stale)")
	cmd.Flags().BoolVar(&flags.force, "force", false, "Overwrite even if version matches")
	cmd.Flags().BoolVar(&flags.codex, "codex", false, "Opt in to Codex: manage verikt's section in ~/.codex/AGENTS.md")
	cmd.Flags().BoolVar(&flags.antigravity, "antigravity", false, "Opt in to Antigravity: manage verikt's section in ~/.gemini/GEMINI.md")

	return cmd
}

func runSetup(cmd *cobra.Command, flags *setupFlags) error {
	det, err := detectAgents(flags.optIn())
	if err != nil {
		return fmt.Errorf("detecting agents: %w", err)
	}

	for _, a := range det.OptInMissing {
		cmd.Printf("  \u2717 %s \u2014 not found at ~/%s (skipped; verikt never creates agent config directories)\n", a.name, a.configDir)
	}

	agents := det.Selected
	if len(agents) == 0 {
		cmd.Println("No supported AI agents detected.")
		cmd.Println()
		cmd.Println("To register verikt with your AI agent, install one of these first:")
		cmd.Println("  - Claude Code: https://claude.ai/download")
		cmd.Println("  - Cursor: https://cursor.com")
		cmd.Println("  - Windsurf: https://windsurf.com")
		cmd.Println("  - Codex or Antigravity: then opt in with --codex or --antigravity")
		cmd.Println()
		cmd.Println("Then run: verikt setup")
		return nil
	}

	if flags.uninstall {
		return runUninstall(cmd, agents)
	}

	if flags.check {
		return runSetupCheck(cmd, agents)
	}

	if err := runWrite(cmd, agents, flags.force); err != nil {
		return err
	}
	printOptInHints(cmd, det.OptInAvailable)

	return installSkills(cmd, agents)
}

// printOptInHints tells the user about installed opt-in agents that were left alone.
func printOptInHints(cmd *cobra.Command, available []agentSpec) {
	for _, a := range available {
		cmd.Printf("  \u00b7 %s detected but not configured. Opt in with: verikt setup --%s\n", a.name, a.flag)
	}
}

func runWrite(cmd *cobra.Command, agents []AIAgent, force bool) error {
	content := GlobalRulesContent(version)
	written := 0

	for _, agent := range agents {
		rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)

		existing, readErr := os.ReadFile(rulesFile) //nolint:gosec // path built from the home dir and a fixed name
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return fmt.Errorf("reading rules file for %s: %w", agent.Name, readErr)
		}

		if !force && readErr == nil && installedVersion(agent, string(existing)) == version {
			cmd.Printf("  = %s \u2014 already current (v%s)\n", agent.Name, version)
			continue
		}

		out := content
		if agent.Shared {
			merged, err := guide.ReplaceSection(string(existing), globalMarkers, content)
			if err != nil {
				return fmt.Errorf("%s: %s: %w; fix or remove the verikt markers by hand, verikt will not overwrite the file", agent.Name, rulesFile, err)
			}
			out = merged
		}

		if err := os.MkdirAll(agent.RulesDir, 0o755); err != nil {
			return fmt.Errorf("creating rules dir for %s: %w", agent.Name, err)
		}

		if err := guide.WriteFileAtomic(rulesFile, []byte(out)); err != nil {
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

// installedVersion returns the verikt version recorded in an agent's rules
// file, or "" when verikt is not installed there. For shared files only
// verikt's own section is considered.
func installedVersion(agent AIAgent, content string) string {
	if agent.Shared {
		start := strings.Index(content, globalMarkers.Start)
		end := strings.Index(content, globalMarkers.End)
		if start < 0 || end < start {
			return ""
		}
		content = content[start:end]
	}
	return extractSentinelVersion(content)
}

func runSetupCheck(cmd *cobra.Command, agents []AIAgent) error {
	stale := 0

	for _, agent := range agents {
		rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)

		existing, err := os.ReadFile(rulesFile) //nolint:gosec
		installed := installedVersion(agent, string(existing))
		if err != nil || installed == "" {
			cmd.Printf("  \u2717 %s \u2014 not installed\n", agent.Name)
			stale++
			continue
		}

		if installed == version {
			cmd.Printf("  \u2713 %s \u2014 current (v%s)\n", agent.Name, version)
		} else {
			cmd.Printf("  \u2717 %s \u2014 stale (v%s \u2192 v%s)\n", agent.Name, installed, version)
			stale++
		}
	}

	if stale > 0 {
		cmd.Println()
		cmd.Printf("%d agent(s) need update. Run: verikt setup\n", stale)
		return fmt.Errorf("stale")
	}

	return nil
}

func runUninstall(cmd *cobra.Command, agents []AIAgent) error {
	for _, agent := range agents {
		rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)

		if agent.Shared {
			if err := uninstallSharedSection(cmd, agent, rulesFile); err != nil {
				return err
			}
			continue
		}

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

// uninstallSharedSection removes verikt's marked section from a shared global
// file, keeping the user's content. The file itself is deleted only when
// nothing but whitespace remains.
func uninstallSharedSection(cmd *cobra.Command, agent AIAgent, rulesFile string) error {
	existing, err := os.ReadFile(rulesFile) //nolint:gosec // path built from the home dir and a fixed name
	if errors.Is(err, os.ErrNotExist) {
		cmd.Printf("  = %s \u2014 not installed (skipped)\n", agent.Name)
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", rulesFile, err)
	}

	remaining, removed, err := guide.RemoveSection(string(existing), globalMarkers)
	if err != nil {
		return fmt.Errorf("%s: %s: %w; remove verikt's section by hand, verikt will not rewrite the file", agent.Name, rulesFile, err)
	}
	if !removed {
		cmd.Printf("  = %s \u2014 not installed (skipped)\n", agent.Name)
		return nil
	}

	if strings.TrimSpace(remaining) == "" {
		if err := os.Remove(rulesFile); err != nil {
			return fmt.Errorf("removing %s: %w", rulesFile, err)
		}
	} else if err := guide.WriteFileAtomic(rulesFile, []byte(remaining)); err != nil {
		return fmt.Errorf("writing %s: %w", rulesFile, err)
	}

	cmd.Printf("  \u2713 %s \u2014 removed verikt section from %s\n", agent.Name, rulesFile)
	return nil
}

func extractSentinelVersion(content string) string {
	matches := sentinelPattern.FindStringSubmatch(content)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

// installSkills asks the user where to install verikt skills and writes them.
func installSkills(cmd *cobra.Command, agents []AIAgent) error {
	// Only offer skills for Claude Code (skills are a Claude Code feature).
	var claudeAgent *AIAgent
	for i := range agents {
		if agents[i].Name == "Claude Code" {
			claudeAgent = &agents[i]
			break
		}
	}
	if claudeAgent == nil {
		return nil
	}

	cmd.Println()

	var installScope string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Install verikt skills?").
			Description("Skills let you run /verikt:init inside Claude Code.").
			Options(
				huh.NewOption("Global — available in all projects", "global"),
				huh.NewOption("Local — this project only", "local"),
				huh.NewOption("Both — global and local", "both"),
				huh.NewOption("Skip — don't install skills", "skip"),
			).
			Value(&installScope),
	)).Run(); err != nil {
		// The skills prompt is optional and comes after the agent rules are
		// written. An aborted prompt, or one that cannot run without a TTY
		// (install.sh --setup under curl | bash), skips skill installation.
		return nil //nolint:nilerr // optional prompt; setup already succeeded
	}

	if installScope == "skip" {
		return nil
	}

	skills := map[string]string{
		"verikt:init": skillVeriktInit,
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}

	writeSkill := func(baseDir, name, content string) error {
		dir := filepath.Join(baseDir, "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating skill dir %s: %w", dir, err)
		}
		path := filepath.Join(dir, "SKILL.md")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing skill %s: %w", path, err)
		}
		cmd.Printf("  ✓ %s → %s\n", name, path)
		return nil
	}

	for name, content := range skills {
		if installScope == "global" || installScope == "both" {
			globalBase := claudeGlobalConfigDir(home)
			if err := writeSkill(globalBase, name, content); err != nil {
				return err
			}
		}
		if installScope == "local" || installScope == "both" {
			localBase := ".claude"
			if err := writeSkill(localBase, name, content); err != nil {
				return err
			}
		}
	}

	return nil
}
