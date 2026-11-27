package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/guide"
	"github.com/diktahq/verikt/internal/provider"
	"github.com/diktahq/verikt/internal/scaffold"
	"github.com/spf13/cobra"
)

func newAddCommand(_ *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <capability> [capability...]",
		Short: "Add capabilities to an existing project",
		Long: `Add one or more capabilities to an existing verikt project.

Capabilities are validated against the provider's template FS, conflicts are checked,
and transitive dependencies are auto-resolved. Existing files are never overwritten.`,
		Example: `  verikt add redis
  verikt add kafka-consumer observability
  verikt add grpc --dry-run`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, err := cmd.Flags().GetBool("dry-run")
			if err != nil {
				return fmt.Errorf("read --dry-run flag: %w", err)
			}
			return runAdd(args, dryRun)
		},
	}
	cmd.Flags().Bool("dry-run", false, "Show what would be added without writing any files")
	return cmd
}

func runAdd(capabilities []string, dryRun bool) error {
	cfgPath, err := config.FindVeriktYAML(".")
	if err != nil {
		return fmt.Errorf("no verikt.yaml found — run `verikt new` first: %w", err)
	}

	cfg, err := config.LoadVeriktYAML(cfgPath)
	if err != nil {
		return fmt.Errorf("load verikt.yaml: %w", err)
	}

	providerImpl, err := provider.Get(cfg.Language)
	if err != nil {
		return fmt.Errorf("get provider: %w", err)
	}
	tFS := providerImpl.GetTemplateFS()

	// The architecture's path mappings decide where capability files go, so an
	// unknown or missing architecture is rejected before anything is rendered.
	if err := validateArchitecture(tFS, cfg.Architecture); err != nil {
		return err
	}

	toAdd, autoDeps, err := resolveCapabilitiesToAdd(tFS, cfg, capabilities)
	if err != nil {
		return err
	}
	if len(toAdd) == 0 {
		fmt.Println("All requested capabilities are already installed.")
		return nil
	}

	if dryRun {
		fmt.Println("Dry run — no files will be written.")
	}
	autoDepSet := make(map[string]bool, len(autoDeps))
	for _, c := range autoDeps {
		autoDepSet[c] = true
	}
	fmt.Println("Adding capabilities:")
	for _, c := range toAdd {
		suffix := ""
		if autoDepSet[c] {
			suffix = " (auto-dependency)"
		}
		fmt.Printf("  + %s%s\n", c, suffix)
	}

	projectDir, vars, err := projectScaffoldVars(cfgPath)
	if err != nil {
		return err
	}

	// Use ComposeProject to get proper vars with Has* flags for all capabilities.
	finalCaps := make([]string, 0, len(cfg.Capabilities)+len(toAdd))
	finalCaps = append(finalCaps, cfg.Capabilities...)
	finalCaps = append(finalCaps, toAdd...)

	plan, err := scaffold.ComposeProject(tFS, cfg.Architecture, finalCaps, vars)
	if err != nil {
		return fmt.Errorf("compose project: %w", err)
	}

	if dryRun {
		return previewAdd(tFS, plan, toAdd, projectDir, autoDeps)
	}
	return applyAdd(tFS, plan, cfg, cfgPath, projectDir, toAdd, finalCaps, autoDeps)
}

// resolveCapabilitiesToAdd validates the requested capabilities, rejects
// conflicts, and resolves transitive dependencies. It returns the full set to
// install (dependencies first) and the subset that was auto-resolved.
func resolveCapabilitiesToAdd(tFS fs.FS, cfg *config.VeriktConfig, capabilities []string) (toAdd, autoDeps []string, err error) {
	// Build set of already-installed capabilities.
	installedSet := make(map[string]bool, len(cfg.Capabilities))
	for _, c := range cfg.Capabilities {
		installedSet[c] = true
	}

	// Filter out already-installed capabilities.
	newCaps := make([]string, 0, len(capabilities))
	for _, c := range capabilities {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if installedSet[c] {
			fmt.Printf("  %s: already installed, skipping\n", c)
			continue
		}
		newCaps = append(newCaps, c)
	}
	if len(newCaps) == 0 {
		return nil, nil, nil
	}

	// Validate each capability exists in the provider's template FS.
	available, err := listAvailableCapabilities(tFS)
	if err != nil {
		return nil, nil, fmt.Errorf("list available capabilities: %w", err)
	}
	availableSet := make(map[string]bool, len(available))
	for _, a := range available {
		availableSet[a] = true
	}
	for _, c := range newCaps {
		if !availableSet[c] {
			return nil, nil, fmt.Errorf("unknown capability %q — available: %s", c, strings.Join(available, ", "))
		}
	}

	// Check for conflicts with existing capabilities.
	allCaps := make([]string, 0, len(cfg.Capabilities)+len(newCaps))
	allCaps = append(allCaps, cfg.Capabilities...)
	allCaps = append(allCaps, newCaps...)

	for _, c := range newCaps {
		capDir := path.Join("templates", "capabilities", c)
		data, readErr := fs.ReadFile(tFS, path.Join(capDir, "capability.yaml"))
		if readErr != nil {
			continue
		}
		cm, parseErr := scaffold.ParseCapabilityManifest(data)
		if parseErr != nil {
			continue
		}
		for _, conflict := range cm.Conflicts {
			if installedSet[conflict] || slices.Contains(newCaps, conflict) {
				return nil, nil, fmt.Errorf("capability %q conflicts with %q", c, conflict)
			}
		}
	}

	// Auto-resolve transitive dependencies.
	capSet := make(map[string]bool, len(allCaps))
	for _, c := range allCaps {
		capSet[c] = true
	}
	resolved := resolveNewDeps(tFS, newCaps, capSet, installedSet)

	// Determine auto-resolved deps (not in original newCaps and not already installed).
	newCapsSet := make(map[string]bool, len(newCaps))
	for _, c := range newCaps {
		newCapsSet[c] = true
	}
	for _, c := range resolved {
		if !newCapsSet[c] && !installedSet[c] {
			autoDeps = append(autoDeps, c)
		}
	}

	// Merge: auto-deps + explicitly requested (all that are truly new).
	for _, c := range resolved {
		if !installedSet[c] {
			toAdd = append(toAdd, c)
		}
	}

	return toAdd, autoDeps, nil
}

// projectScaffoldVars infers the template variables for an existing project:
// ServiceName from the directory name, ModulePath from go.mod.
func projectScaffoldVars(cfgPath string) (projectDir string, vars map[string]interface{}, err error) {
	projectDir = strings.TrimSuffix(cfgPath, "/verikt.yaml")
	projectDir = strings.TrimSuffix(projectDir, "\\verikt.yaml")
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", nil, fmt.Errorf("resolve path: %w", err)
	}
	serviceName := filepath.Base(absDir)
	modulePath := fmt.Sprintf("example.com/%s", serviceName)
	if goModData, readErr := os.ReadFile(filepath.Join(projectDir, "go.mod")); readErr == nil {
		for _, line := range strings.Split(string(goModData), "\n") {
			if strings.HasPrefix(line, "module ") {
				modulePath = strings.TrimSpace(strings.TrimPrefix(line, "module "))
				break
			}
		}
	}

	return projectDir, map[string]interface{}{
		"ServiceName": serviceName,
		"ModulePath":  modulePath,
	}, nil
}

// previewAdd reports what applyAdd would do without writing anything.
func previewAdd(tFS fs.FS, plan *scaffold.CompositionPlan, toAdd []string, projectDir string, autoDeps []string) error {
	created, skipped, err := scaffold.NewRenderer(tFS).PreviewAddedCapabilities(plan, projectDir, toAdd)
	if err != nil {
		return fmt.Errorf("preview capabilities: %w", err)
	}
	for _, f := range created {
		fmt.Printf("  Would create: %s\n", f)
	}
	for _, f := range skipped {
		fmt.Printf("  Would skip (exists): %s\n", f)
	}

	fmt.Printf("\nDry run: %d files would be created, %d skipped, %d capabilities added\n",
		len(created), len(skipped), len(toAdd))
	fmt.Println("verikt.yaml and the generated guides were not modified.")
	if len(autoDeps) > 0 {
		fmt.Printf("Auto-resolved dependencies: %s\n", strings.Join(autoDeps, ", "))
	}

	return nil
}

// applyAdd renders the new capabilities, records them in verikt.yaml, and
// regenerates the guides. Only the newly added capabilities are rendered,
// through the same path as `verikt new`: the architecture's path mappings,
// ArchPaths and partials all apply.
func applyAdd(tFS fs.FS, plan *scaffold.CompositionPlan, cfg *config.VeriktConfig, cfgPath, projectDir string, toAdd, finalCaps, autoDeps []string) error {
	result, skipped, err := scaffold.NewRenderer(tFS).RenderAddedCapabilities(plan, projectDir, toAdd)
	if err != nil {
		return fmt.Errorf("render capabilities: %w", err)
	}
	for _, f := range result.FilesCreated {
		fmt.Printf("  Created: %s\n", f)
	}
	for _, f := range skipped {
		fmt.Printf("  Skipped (exists): %s\n", f)
	}

	// Update verikt.yaml with new capabilities.
	cfg.Capabilities = finalCaps
	sort.Strings(cfg.Capabilities)
	if err := config.SaveVeriktYAML(cfgPath, cfg); err != nil {
		return fmt.Errorf("save verikt.yaml: %w", err)
	}

	// Regenerate guides.
	if guideErr := guide.GenerateFromConfig(projectDir, cfg, "all", tFS); guideErr != nil {
		return fmt.Errorf("regenerate guide: %w", guideErr)
	}

	fmt.Printf("\nDone: %d files created, %d files skipped, %d capabilities added\n",
		len(result.FilesCreated), len(skipped), len(toAdd))
	if len(autoDeps) > 0 {
		fmt.Printf("Auto-resolved dependencies: %s\n", strings.Join(autoDeps, ", "))
	}

	return nil
}

// validateArchitecture fails with a clear error when verikt.yaml names no
// architecture, or one the provider does not ship.
func validateArchitecture(tFS fs.FS, architecture string) error {
	archs, err := listArchitectures(tFS)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(archs))
	for _, a := range archs {
		if a.name == architecture {
			return nil
		}
		names = append(names, a.name)
	}
	if strings.TrimSpace(architecture) == "" {
		return fmt.Errorf("verikt.yaml declares no architecture — set `architecture:` to one of: %s", strings.Join(names, ", "))
	}
	return fmt.Errorf("unknown architecture %q in verikt.yaml — available: %s", architecture, strings.Join(names, ", "))
}

// listAvailableCapabilities returns the names of all capabilities in the template FS.
func listAvailableCapabilities(tFS fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(tFS, path.Join("templates", "capabilities"))
	if err != nil {
		return nil, fmt.Errorf("read capabilities dir: %w", err)
	}
	caps := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Verify it has a capability.yaml.
		if _, statErr := fs.Stat(tFS, path.Join("templates", "capabilities", entry.Name(), "capability.yaml")); statErr != nil {
			continue
		}
		caps = append(caps, entry.Name())
	}
	return caps, nil
}

// resolveNewDeps resolves transitive dependencies for the new capabilities,
// excluding those already installed.
func resolveNewDeps(tFS fs.FS, newCaps []string, capSet map[string]bool, installedSet map[string]bool) []string {
	queue := make([]string, len(newCaps))
	copy(queue, newCaps)
	visited := make(map[string]bool, len(newCaps))
	var ordered []string

	for len(queue) > 0 {
		cap := queue[0]
		queue = queue[1:]
		if visited[cap] {
			continue
		}
		visited[cap] = true

		capDir := path.Join("templates", "capabilities", cap)
		data, err := fs.ReadFile(tFS, path.Join(capDir, "capability.yaml"))
		if err != nil {
			ordered = append(ordered, cap)
			continue
		}
		cm, err := scaffold.ParseCapabilityManifest(data)
		if err != nil {
			ordered = append(ordered, cap)
			continue
		}

		for _, req := range cm.Requires {
			if !capSet[req] && !installedSet[req] {
				capSet[req] = true
				queue = append(queue, req)
			}
		}
		ordered = append(ordered, cap)
	}

	// Dependencies first, then explicitly requested.
	newCapsSet := make(map[string]bool, len(newCaps))
	for _, c := range newCaps {
		newCapsSet[c] = true
	}
	var deps, orig []string
	for _, c := range ordered {
		if newCapsSet[c] {
			orig = append(orig, c)
		} else {
			deps = append(deps, c)
		}
	}
	return append(deps, orig...)
}
