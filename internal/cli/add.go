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

	"github.com/dcsg/archway/internal/config"
	"github.com/dcsg/archway/internal/guide"
	"github.com/dcsg/archway/internal/provider"
	"github.com/dcsg/archway/internal/scaffold"
	"github.com/spf13/cobra"
)

func newAddCommand(_ *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <capability> [capability...]",
		Short: "Add capabilities to an existing project",
		Long: `Add one or more capabilities to an existing Archway project.

Capabilities are validated against the provider's template FS, conflicts are checked,
and transitive dependencies are auto-resolved. Existing files are never overwritten.`,
		Example: `  archway add redis
  archway add kafka-consumer observability
  archway add grpc --dry-run`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runAdd(args)
		},
	}
	return cmd
}

func runAdd(capabilities []string) error {
	cfgPath, err := config.FindArchwayYAML(".")
	if err != nil {
		return fmt.Errorf("no archway.yaml found — run `archway new` first: %w", err)
	}

	cfg, err := config.LoadArchwayYAML(cfgPath)
	if err != nil {
		return fmt.Errorf("load archway.yaml: %w", err)
	}

	providerImpl, err := provider.Get(cfg.Language)
	if err != nil {
		return fmt.Errorf("get provider: %w", err)
	}
	tFS := providerImpl.GetTemplateFS()

	// Build set of already-installed capabilities.
	installedSet := make(map[string]bool, len(cfg.Capabilities))
	for _, c := range cfg.Capabilities {
		installedSet[c] = true
	}

	// Filter out already-installed capabilities.
	var newCaps []string
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
		fmt.Println("All requested capabilities are already installed.")
		return nil
	}

	// Validate each capability exists in the provider's template FS.
	available, err := listAvailableCapabilities(tFS)
	if err != nil {
		return fmt.Errorf("list available capabilities: %w", err)
	}
	availableSet := make(map[string]bool, len(available))
	for _, a := range available {
		availableSet[a] = true
	}
	for _, c := range newCaps {
		if !availableSet[c] {
			return fmt.Errorf("unknown capability %q — available: %s", c, strings.Join(available, ", "))
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
				return fmt.Errorf("capability %q conflicts with %q", c, conflict)
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
	var autoDeps []string
	for _, c := range resolved {
		if !newCapsSet[c] && !installedSet[c] {
			autoDeps = append(autoDeps, c)
		}
	}

	// Merge: auto-deps + explicitly requested (all that are truly new).
	var toAdd []string
	for _, c := range resolved {
		if !installedSet[c] {
			toAdd = append(toAdd, c)
		}
	}

	// Show what will be added.
	fmt.Println("Adding capabilities:")
	for _, c := range toAdd {
		suffix := ""
		if !newCapsSet[c] {
			suffix = " (auto-dependency)"
		}
		fmt.Printf("  + %s%s\n", c, suffix)
	}

	// Infer ServiceName from directory name, ModulePath from go.mod.
	projectDir := strings.TrimSuffix(cfgPath, "/archway.yaml")
	projectDir = strings.TrimSuffix(projectDir, "\\archway.yaml")
	absDir, absErr := filepath.Abs(projectDir)
	if absErr != nil {
		return fmt.Errorf("resolve path: %w", absErr)
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

	vars := map[string]interface{}{
		"ServiceName": serviceName,
		"ModulePath":  modulePath,
	}
	// Use ComposeProject to get proper vars with Has* flags for all capabilities.
	finalCaps := make([]string, 0, len(cfg.Capabilities)+len(toAdd))
	finalCaps = append(finalCaps, cfg.Capabilities...)
	finalCaps = append(finalCaps, toAdd...)

	plan, err := scaffold.ComposeProject(tFS, cfg.Architecture, finalCaps, vars)
	if err != nil {
		return fmt.Errorf("compose project: %w", err)
	}

	renderer := scaffold.NewRenderer(tFS)
	var totalCreated, totalSkipped int

	// Only render files from newly-added capabilities.
	for _, c := range toAdd {
		capDir := path.Join("templates", "capabilities", c)
		result, skipped, renderErr := renderer.RenderCapabilityFiles(capDir, projectDir, plan.Vars)
		if renderErr != nil {
			return fmt.Errorf("render capability %q: %w", c, renderErr)
		}
		for _, f := range result.FilesCreated {
			fmt.Printf("  Created: %s\n", f)
			totalCreated++
		}
		for _, f := range skipped {
			fmt.Printf("  Skipped (exists): %s\n", f)
			totalSkipped++
		}
	}

	// Update archway.yaml with new capabilities.
	cfg.Capabilities = finalCaps
	sort.Strings(cfg.Capabilities)
	if err := config.SaveArchwayYAML(cfgPath, cfg); err != nil {
		return fmt.Errorf("save archway.yaml: %w", err)
	}

	// Regenerate guides.
	if guideErr := guide.GenerateFromConfig(projectDir, cfg, "all", tFS); guideErr != nil {
		return fmt.Errorf("regenerate guide: %w", guideErr)
	}

	fmt.Printf("\nDone: %d files created, %d files skipped, %d capabilities added\n",
		totalCreated, totalSkipped, len(toAdd))
	if len(autoDeps) > 0 {
		fmt.Printf("Auto-resolved dependencies: %s\n", strings.Join(autoDeps, ", "))
	}

	return nil
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
