package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/diktahq/verikt/internal/analyzer/detector"
	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/guide"
	"github.com/diktahq/verikt/internal/provider"
	"github.com/diktahq/verikt/internal/scaffold"
	"github.com/spf13/cobra"
)

type initCommandOptions struct {
	Path         string
	Architecture string
	Language     string
	Capabilities string
	GuideMode    string
	Force        bool
	NoWizard     bool
	AI           bool
}

func newInitCommand(_ *globalOptions) *cobra.Command {
	opts := &initCommandOptions{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set up verikt — detects greenfield or existing codebase",
		Long: `The single entry point for verikt onboarding. Detects your project state
and routes to the right flow:

  Empty directory     → Greenfield: scaffold a new service from scratch
  Existing code       → Brownfield: analyze codebase, map architecture, or start a clean bubble
  Has verikt.yaml    → Reconfigure: update existing setup (use --force)

Use --ai to let your AI agent conduct the setup interview instead.`,
		Example: `  verikt init
  verikt init --ai
  verikt init --language typescript --no-wizard`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Path, "path", ".", "Project path")
	cmd.Flags().StringVar(&opts.Architecture, "architecture", "", "Architecture pattern")
	cmd.Flags().StringVar(&opts.Language, "language", "", "Language (go, typescript)")
	cmd.Flags().StringVar(&opts.Capabilities, "cap", "", "Capabilities (comma-separated)")
	cmd.Flags().StringVar(&opts.GuideMode, "guide-mode", "", "Guide mode: passive|audit|prompted")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Overwrite existing verikt.yaml")
	cmd.Flags().BoolVar(&opts.NoWizard, "no-wizard", false, "Disable interactive wizard")
	cmd.Flags().BoolVar(&opts.AI, "ai", false, "Print AI interview protocol for agent-driven setup")

	return cmd
}

func runInit(ctx context.Context, opts *initCommandOptions) error {
	if opts.AI {
		fmt.Print(guide.InterviewProtocol())
		return nil
	}

	if opts.Path == "" {
		opts.Path = "."
	}

	// Detect project state.
	veriktPath := filepath.Join(opts.Path, "verikt.yaml")
	hasVerikt := false
	if _, err := os.Stat(veriktPath); err == nil {
		hasVerikt = true
		if !opts.Force {
			return fmt.Errorf("%s already exists (use --force to reconfigure)", veriktPath)
		}
	}

	projectState := detectProjectState(opts.Path, hasVerikt)

	switch projectState {
	case stateGreenfield:
		fmt.Println("Empty project detected — starting greenfield setup.")
		fmt.Println()
		return runGreenfieldInit(ctx, opts)
	case stateBrownfield:
		fmt.Println("Existing codebase detected.")
		fmt.Println()
		return runBrownfieldInit(ctx, opts)
	case stateReconfigure:
		fmt.Println("Existing verikt.yaml found — reconfiguring.")
		fmt.Println()
		return runBrownfieldInit(ctx, opts)
	default:
		return runGreenfieldInit(ctx, opts)
	}
}

type projectState int

const (
	stateGreenfield  projectState = iota
	stateBrownfield
	stateReconfigure
)

// detectProjectState checks whether the directory is empty, has code, or has verikt.yaml.
func detectProjectState(path string, hasVerikt bool) projectState {
	if hasVerikt {
		return stateReconfigure
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return stateGreenfield
	}

	for _, e := range entries {
		name := e.Name()
		// Skip hidden files and common non-code files.
		if strings.HasPrefix(name, ".") || name == "README.md" || name == "LICENSE" {
			continue
		}
		// Any real file or directory = brownfield.
		return stateBrownfield
	}

	return stateGreenfield
}

// --- Greenfield Flow ---
// Empty directory. Full scaffold wizard — same quality as `verikt new`.

func runGreenfieldInit(ctx context.Context, opts *initCommandOptions) error {
	// Reuse the full `verikt new` wizard flow.
	newOpts := &newCommandOptions{
		Language:     opts.Language,
		Architecture: opts.Architecture,
		Capabilities: opts.Capabilities,
		GuideMode:    opts.GuideMode,
		NoWizard:     opts.NoWizard,
		OutputDir:    opts.Path,
	}

	if !opts.NoWizard {
		wizardVars, err := runCompositionWizard(newOpts)
		if err != nil {
			return err
		}
		_ = wizardVars
	}

	if strings.TrimSpace(newOpts.Name) == "" {
		// Use current directory name as project name.
		absPath, _ := filepath.Abs(opts.Path)
		newOpts.Name = filepath.Base(absPath)
	}

	return runNew(ctx, newOpts)
}

// --- Brownfield Flow ---
// Existing code. Analyze first, then ask what the user wants to do.

func runBrownfieldInit(ctx context.Context, opts *initCommandOptions) error {
	// Step 1: Detect language.
	if strings.TrimSpace(opts.Language) == "" {
		if detected, conf, err := detector.DetectLanguage(opts.Path); err == nil && detected != "unknown" {
			opts.Language = detected
			fmt.Printf("  Language:     %s (%.0f%% confidence)\n", detected, conf*100)
		} else {
			opts.Language = "go"
			fmt.Printf("  Language:     %s (default)\n", opts.Language)
		}
	}

	// Step 2: Run analyzer.
	providerImpl, err := provider.Get(opts.Language)
	if err != nil {
		return err
	}

	fmt.Println("  Analyzing codebase...")
	analysis, analyzeErr := providerImpl.Analyze(ctx, provider.AnalyzeRequest{Path: opts.Path})

	if analyzeErr == nil && analysis != nil {
		if analysis.Architecture.Pattern != "" {
			if strings.TrimSpace(opts.Architecture) == "" {
				opts.Architecture = analysis.Architecture.Pattern
			}
			fmt.Printf("  Architecture: %s (%.0f%% confidence)\n", analysis.Architecture.Pattern, analysis.Architecture.Confidence*100)
		}
		if analysis.Framework.Name != "" {
			fmt.Printf("  Framework:    %s\n", analysis.Framework.Name)
		}
		if len(analysis.Framework.Libraries) > 0 {
			names := make([]string, 0, len(analysis.Framework.Libraries))
			for _, lib := range analysis.Framework.Libraries {
				names = append(names, lib.Name)
			}
			fmt.Printf("  Libraries:    %s\n", strings.Join(names, ", "))
		}
		fmt.Printf("  Files:        %d  Packages: %d\n", analysis.FileCount, analysis.PackageCount)
	}
	fmt.Println()

	if strings.TrimSpace(opts.Architecture) == "" {
		opts.Architecture = "hexagonal"
	}
	if strings.TrimSpace(opts.GuideMode) == "" {
		opts.GuideMode = "passive"
	}

	// Step 3: Ask what the user wants to do.
	if !opts.NoWizard {
		var strategy string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("What would you like to do?").
				Options(
					huh.NewOption("Map existing architecture — detect and govern what's already here", "map"),
					huh.NewOption("Bubble context — start a clean new service inside this project", "bubble"),
				).
				Value(&strategy),
		)).Run(); err != nil {
			return err
		}

		if strategy == "bubble" {
			return runBubbleContext(ctx, opts)
		}
	}

	// Map existing: confirm/adjust detected values, generate verikt.yaml.
	return runMapExisting(ctx, opts, providerImpl)
}

// runMapExisting lets the user confirm/adjust the detected architecture and capabilities,
// then generates verikt.yaml + guide.
func runMapExisting(ctx context.Context, opts *initCommandOptions, providerImpl provider.LanguageProvider) error {
	if !opts.NoWizard {
		if err := runInitWizard(opts, providerImpl); err != nil {
			return err
		}
	}

	// Generate verikt.yaml.
	veriktPath := filepath.Join(opts.Path, "verikt.yaml")
	cfg := config.DefaultVeriktConfig(opts.Language, opts.Architecture)
	if opts.Capabilities != "" {
		cfg.Capabilities = strings.Split(opts.Capabilities, ",")
	}
	cfg.Guide.Mode = opts.GuideMode

	if err := config.SaveVeriktYAML(veriktPath, cfg); err != nil {
		return err
	}
	fmt.Printf("\nGenerated %s\n", veriktPath)

	// Show capability warnings.
	if len(cfg.Capabilities) > 0 {
		capWarnings := scaffold.CapabilityWarnings(cfg.Capabilities)
		if len(capWarnings) > 0 {
			fmt.Println("\n⚠  Capability warnings:")
			for _, w := range capWarnings {
				fmt.Printf("  • %s\n", w.Message)
			}
		}
	}

	// Auto-run verikt guide.
	fmt.Println("\nGenerating AI agent context files...")
	var templateFS fs.FS
	if p, pErr := provider.Get(opts.Language); pErr == nil {
		templateFS = p.GetTemplateFS()
	}
	if gErr := guide.GenerateFromConfig(opts.Path, cfg, "all", templateFS); gErr != nil {
		fmt.Printf("  (guide generation failed: %v — run 'verikt guide' manually)\n", gErr)
	} else {
		fmt.Println("  Guide generated for all targets.")
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  verikt check                # validate architecture compliance")
	fmt.Println("  verikt add <capability>     # add more capabilities")

	return nil
}

// runBubbleContext scaffolds a new clean service inside an existing project.
// The new service becomes the "bubble" — well-governed, clean architecture —
// and features can migrate from the old code into it over time (strangler fig pattern).
func runBubbleContext(ctx context.Context, opts *initCommandOptions) error {
	fmt.Println("\nBubble context: scaffold a clean new service inside this project.")
	fmt.Println("The new service will have proper architecture from day one.")
	fmt.Println("Migrate features from the existing code into it over time.")
	fmt.Println()

	newOpts := &newCommandOptions{
		Language:     opts.Language,
		Architecture: opts.Architecture,
		Capabilities: opts.Capabilities,
		GuideMode:    opts.GuideMode,
		NoWizard:     opts.NoWizard,
		OutputDir:    opts.Path,
	}

	if !opts.NoWizard {
		wizardVars, err := runCompositionWizard(newOpts)
		if err != nil {
			return err
		}
		_ = wizardVars
	}

	if strings.TrimSpace(newOpts.Name) == "" {
		return fmt.Errorf("service name is required for bubble context")
	}

	return runNew(ctx, newOpts)
}

// runInitWizard lets the user confirm/adjust detected values for brownfield init.
func runInitWizard(opts *initCommandOptions, providerImpl provider.LanguageProvider) error {
	language := opts.Language
	architecture := opts.Architecture

	// Step 1: Confirm or change language.
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Language").Value(&language).Options(
			huh.NewOption("Go", "go"),
			huh.NewOption("TypeScript / Node.js", "typescript"),
		),
	)).Run(); err != nil {
		return err
	}

	// If language changed, get the new provider.
	if language != opts.Language {
		opts.Language = language
		var err error
		providerImpl, err = provider.Get(language)
		if err != nil {
			return err
		}
	}
	opts.Language = language

	// Step 2: Confirm or change architecture.
	tFS := providerImpl.GetTemplateFS()
	architectures, err := listArchitectures(tFS)
	if err != nil {
		return err
	}
	archOpts := make([]huh.Option[string], 0, len(architectures))
	for _, a := range architectures {
		archOpts = append(archOpts, huh.NewOption(a.label, a.name))
	}
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Architecture pattern").
			Options(archOpts...).
			Value(&architecture),
	)).Run(); err != nil {
		return err
	}
	opts.Architecture = architecture

	// Step 3: Capability selection.
	capabilities, err := listCapabilities(tFS)
	if err != nil {
		return err
	}
	capOpts := make([]huh.Option[string], 0, len(capabilities))
	for _, c := range capabilities {
		capOpts = append(capOpts, huh.NewOption(
			fmt.Sprintf("%-18s %s", c.name, c.description),
			c.name,
		))
	}

	var selectedCaps []string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("What capabilities does this service use? (space to toggle)").
			Options(capOpts...).
			Value(&selectedCaps),
	)).Run(); err != nil {
		return err
	}

	// Step 4: Smart suggestions.
	suggestions := scaffold.ComputeSuggestions(selectedCaps)
	if len(suggestions) > 0 {
		suggOpts := make([]huh.Option[string], 0, len(suggestions))
		for _, s := range suggestions {
			suggOpts = append(suggOpts, huh.NewOption(
				fmt.Sprintf("%-18s %s", s.Capability, s.Reason),
				s.Capability,
			))
		}

		var acceptedSuggestions []string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Based on your selections, you might also want:").
				Options(suggOpts...).
				Value(&acceptedSuggestions),
		)).Run(); err != nil {
			return err
		}
		selectedCaps = append(selectedCaps, acceptedSuggestions...)
	}

	// Warnings.
	capWarnings := scaffold.CapabilityWarnings(selectedCaps)
	if len(capWarnings) > 0 {
		fmt.Println("\n⚠  Capability warnings:")
		for _, w := range capWarnings {
			fmt.Printf("  • %s\n", w.Message)
		}
		fmt.Println()
	}

	opts.Capabilities = strings.Join(selectedCaps, ",")

	// Step 5: Guide mode.
	guideMode := opts.GuideMode
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("How should AI agents use the architecture guide?").
			Description("Controls how Claude, Cursor, Copilot, and Windsurf behave with verikt context.").
			Options(
				huh.NewOption("passive  — answer first, architecture notes at the end", "passive"),
				huh.NewOption("audit    — read codebase on session start, lead with gap analysis", "audit"),
				huh.NewOption("prompted — passive + suggested prompts appended to guide", "prompted"),
			).
			Value(&guideMode),
	)).Run(); err != nil {
		return err
	}
	opts.GuideMode = guideMode

	// Step 6: Confirmation.
	fmt.Printf("\nReady to initialize:\n")
	fmt.Printf("  Language:       %s\n", opts.Language)
	fmt.Printf("  Architecture:   %s\n", opts.Architecture)
	if len(selectedCaps) > 0 {
		fmt.Printf("  Capabilities:   %s\n", strings.Join(selectedCaps, ", "))
	}
	fmt.Printf("  Guide mode:     %s\n", opts.GuideMode)

	var confirmed bool
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title("Proceed?").Value(&confirmed),
	)).Run(); err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("cancelled")
	}

	return nil
}
