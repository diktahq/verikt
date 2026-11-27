package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"go.yaml.in/yaml/v3"
)

// CapabilityManifest describes a composable capability module.
type CapabilityManifest struct {
	Name             string                     `yaml:"name" json:"name"`
	Description      string                     `yaml:"description" json:"description"`
	Variables        []VariableDefinition       `yaml:"variables,omitempty" json:"variables,omitempty"`
	Requires         []string                   `yaml:"requires,omitempty" json:"requires,omitempty"`
	Suggests         []string                   `yaml:"suggests,omitempty" json:"suggests,omitempty"`
	Conflicts        []string                   `yaml:"conflicts,omitempty" json:"conflicts,omitempty"`
	RequiresFeatures []string                   `yaml:"requires_features,omitempty" json:"requires_features,omitempty"`
	Conditional      map[string]ConditionalRule `yaml:"conditional,omitempty" json:"conditional,omitempty"`
}

// CompositionPlan holds the merged result of architecture + capabilities.
type CompositionPlan struct {
	Architecture string
	Capabilities []string
	Manifest     *Manifest              // architecture manifest
	CapManifests []CapabilityManifest   // loaded capability manifests
	Vars         map[string]interface{} // merged variables
	Partials     map[string][]string    // partial_name → rendered snippets
	ArchDir      string                 // e.g. "templates/architectures/hexagonal"
	CapDirs      []string               // e.g. ["templates/capabilities/http-api", ...]
	Warnings     []string               // non-fatal warnings (e.g. capability conflicts)
}

// ParseCapabilityManifest parses a capability.yaml file.
func ParseCapabilityManifest(data []byte) (*CapabilityManifest, error) {
	m := &CapabilityManifest{}
	if err := yaml.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("parse capability manifest: %w", err)
	}
	if strings.TrimSpace(m.Name) == "" {
		return nil, fmt.Errorf("capability manifest missing name")
	}
	return m, nil
}

// ComposeProject builds a CompositionPlan from an architecture and a set of capabilities.
func ComposeProject(templateFS fs.FS, architecture string, capabilities []string, vars map[string]interface{}) (*CompositionPlan, error) {
	if vars == nil {
		vars = map[string]interface{}{}
	}

	archDir := path.Join("templates", "architectures", architecture)

	// Load architecture manifest.
	archManifestData, err := fs.ReadFile(templateFS, path.Join(archDir, "manifest.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read architecture manifest %q: %w", architecture, err)
	}
	archManifest, err := ParseManifest(archManifestData)
	if err != nil {
		return nil, fmt.Errorf("parse architecture manifest %q: %w", architecture, err)
	}

	// Apply architecture defaults.
	for k, v := range archManifest.Defaults() {
		if _, exists := vars[k]; !exists {
			vars[k] = v
		}
	}

	// Resolve transitive dependencies and order every capability after the
	// ones it requires, then load the manifests in that order.
	capSet := make(map[string]bool, len(capabilities))
	for _, c := range capabilities {
		capSet[c] = true
	}
	capabilities, err = resolveCapabilityDeps(templateFS, capabilities, capSet)
	if err != nil {
		return nil, err
	}
	capManifests := make([]CapabilityManifest, 0, len(capabilities))
	capDirs := make([]string, 0, len(capabilities))
	for _, cap := range capabilities {
		capDir := path.Join("templates", "capabilities", cap)
		data, err := fs.ReadFile(templateFS, path.Join(capDir, "capability.yaml"))
		if err != nil {
			return nil, fmt.Errorf("read capability manifest %q: %w", cap, err)
		}
		cm, err := ParseCapabilityManifest(data)
		if err != nil {
			return nil, fmt.Errorf("capability %q: %w", cap, err)
		}
		capManifests = append(capManifests, *cm)
		capDirs = append(capDirs, capDir)
	}

	// Validate requirements and detect conflicts.
	var warnings []string
	for _, cm := range capManifests {
		for _, req := range cm.Requires {
			if !capSet[req] {
				return nil, fmt.Errorf("capability %q requires %q which is not selected", cm.Name, req)
			}
		}
		for _, conflict := range cm.Conflicts {
			if capSet[conflict] {
				warnings = append(warnings, fmt.Sprintf("capability %q conflicts with %q — is this intentional?", cm.Name, conflict))
			}
		}
	}

	if err := EnforceRequiredFeatures(templateFS, archManifest, capManifests, vars); err != nil {
		return nil, err
	}

	// Apply capability variable defaults.
	for _, cm := range capManifests {
		for _, v := range cm.Variables {
			if v.Default != "" {
				if _, exists := vars[v.Name]; !exists {
					if v.Type == "bool" {
						vars[v.Name] = strings.EqualFold(v.Default, "true")
					} else {
						vars[v.Name] = v.Default
					}
				}
			}
		}
	}

	// Coerce string booleans for architecture variables.
	for _, def := range archManifest.Variables {
		if def.Type == "bool" {
			if v, ok := vars[def.Name]; ok {
				if s, isStr := v.(string); isStr {
					vars[def.Name] = strings.EqualFold(s, "true")
				}
			}
		}
	}

	// Validate required architecture variables.
	for _, def := range archManifest.Variables {
		if def.Required {
			if v, ok := vars[def.Name]; !ok || strings.TrimSpace(fmt.Sprint(v)) == "" {
				return nil, fmt.Errorf("missing required variable %q", def.Name)
			}
		}
	}

	// Set Has* boolean flags for backward compatibility with conditional templates.
	capFlagMap := map[string]string{
		"http-api":        "HasHTTP",
		"grpc":            "HasGRPC",
		"kafka-consumer":  "HasKafka",
		"mysql":           "HasMySQL",
		"redis":           "HasRedis",
		"platform":        "HasPlatform",
		"bootstrap":       "HasBootstrap",
		"postgres":        "HasPostgreSQL",
		"health":          "HasHealth",
		"cors":            "HasCORS",
		"validation":      "HasValidation",
		"migrations":      "HasMigrations",
		"event-bus":       "HasEventBus",
		"circuit-breaker": "HasCircuitBreaker",
		"retry":           "HasRetry",
		"idempotency":     "HasIdempotency",
		"observability":   "HasObservability",
		"request-id":      "HasRequestID",
		"audit-log":       "HasAuditLog",
		"worker":          "HasWorker",
		"scheduler":       "HasScheduler",
		"websocket":       "HasWebSocket",
		"api-versioning":  "HasAPIVersioning",
		"ci-bitbucket":    "HasCIBitbucket",
		"cqrs":            "HasCQRS",
		"ddd":             "HasDDD",
		"graceful":        "HasGraceful",
		"outbox":          "HasOutbox",
		"repository":      "HasRepository",
		"uuid":            "HasUUID",
		"i18n":            "HasI18n",
		"mailpit":         "HasMailpit",
		"bff":             "HasBFF",
		"nats":            "HasNATS",
		"elasticsearch":   "HasElasticsearch",
		"timeout":         "HasTimeout",
		"bulkhead":        "HasBulkhead",
		"oauth2":          "HasOAuth2",
		"encryption":      "HasEncryption",
		"sse":             "HasSSE",
		"graphql":         "HasGraphQL",
		"mongodb":         "HasMongoDB",
		"dynamodb":        "HasDynamoDB",
		"s3":              "HasS3",
		"sqlite":          "HasSQLite",
		"saga":            "HasSaga",
		"email-gateway":   "HasEmail",
		"auth-jwt":        "HasAuthJWT",
		"rate-limiting":   "HasRateLimit",
		"http-client":     "HasHTTPClient",
		"ci-github":       "HasCIGitHub",
		"pre-commit":      "HasPreCommit",
		"devcontainer":    "HasDevContainer",
	}
	for _, c := range capabilities {
		if flag, ok := capFlagMap[c]; ok {
			vars[flag] = true
		}
	}

	// Inject ArchPaths so partials can reference architecture-aware paths.
	if archManifest.PathMappings != nil {
		pm := NewPathMapper(archManifest.PathMappings)
		vars["ArchPaths"] = pm.ArchPaths()
	}

	// Collect partials from each capability.
	partials, err := collectPartials(templateFS, capDirs, vars)
	if err != nil {
		return nil, err
	}

	return &CompositionPlan{
		Architecture: architecture,
		Capabilities: capabilities,
		Manifest:     archManifest,
		CapManifests: capManifests,
		Vars:         vars,
		Partials:     partials,
		ArchDir:      archDir,
		CapDirs:      capDirs,
		Warnings:     warnings,
	}, nil
}

// collectPartials reads _partials/ directories from each capability and renders them.
func collectPartials(templateFS fs.FS, capDirs []string, vars map[string]interface{}) (map[string][]string, error) {
	partials := map[string][]string{}
	for _, capDir := range capDirs {
		partialsDir := path.Join(capDir, "_partials")
		entries, err := fs.ReadDir(templateFS, partialsDir)
		if err != nil {
			continue // no partials for this capability
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			// Strip .go.tmpl or .tmpl extension to get the partial key.
			key := strings.TrimSuffix(strings.TrimSuffix(name, ".tmpl"), ".go")
			data, err := fs.ReadFile(templateFS, path.Join(partialsDir, name))
			if err != nil {
				return nil, fmt.Errorf("read partial %s/%s: %w", capDir, name, err)
			}
			rendered, err := executeTemplate(string(data), vars)
			if err != nil {
				return nil, fmt.Errorf("render partial %s/%s: %w", capDir, name, err)
			}
			content := strings.TrimSpace(string(rendered))
			if content != "" {
				partials[key] = append(partials[key], content)
			}
		}
	}
	return partials, nil
}

// resolveCapabilityDeps walks the requires graph and adds missing transitive
// dependencies. It returns every capability ordered so each one follows all the
// capabilities it requires: partials render in this order, and a partial may
// use identifiers that a required capability's partial declares. Among
// capabilities that do not depend on each other the user's order is kept, with
// auto-added dependencies ahead of it. It fails naming the requiring
// capability when a requirement has no manifest in templateFS, and naming the
// cycle when requires form one.
func resolveCapabilityDeps(templateFS fs.FS, capabilities []string, capSet map[string]bool) ([]string, error) {
	// BFS to discover all transitive requires.
	queue := make([]string, len(capabilities))
	copy(queue, capabilities)
	discovered := make([]string, 0, len(capabilities))
	requires := map[string][]string{}
	requiredBy := map[string]string{}

	for len(queue) > 0 {
		cap := queue[0]
		queue = queue[1:]
		if _, seen := requires[cap]; seen {
			continue
		}

		cm, err := readDependencyManifest(templateFS, cap, requiredBy[cap])
		if err != nil {
			return nil, err
		}
		requires[cap] = append([]string{}, cm.Requires...)

		// Add requires that aren't yet selected.
		for _, req := range cm.Requires {
			if !capSet[req] {
				capSet[req] = true
				requiredBy[req] = cap
				queue = append(queue, req)
			}
		}
		discovered = append(discovered, cap)
	}

	// Auto-added dependencies first, then the user's selection.
	var deps, orig []string
	origSet := map[string]bool{}
	for _, c := range capabilities {
		origSet[c] = true
	}
	for _, c := range discovered {
		if origSet[c] {
			orig = append(orig, c)
		} else {
			deps = append(deps, c)
		}
	}
	return sortByRequires(append(deps, orig...), requires)
}

// sortByRequires orders caps so every capability follows the capabilities it
// requires. It is a depth-first topological sort over caps in their given
// order, so capabilities with no requires path between them keep that order.
// requires must hold an entry for every capability in caps.
func sortByRequires(caps []string, requires map[string][]string) ([]string, error) {
	const (
		visiting = 1
		done     = 2
	)
	state := make(map[string]int, len(caps))
	sorted := make([]string, 0, len(caps))
	var path []string

	var visit func(c string) error
	visit = func(c string) error {
		switch state[c] {
		case done:
			return nil
		case visiting:
			start := 0
			for i, p := range path {
				if p == c {
					start = i
					break
				}
			}
			cycle := append(append([]string{}, path[start:]...), c)
			return fmt.Errorf("capability requires form a cycle: %s", strings.Join(cycle, " -> "))
		}
		state[c] = visiting
		path = append(path, c)
		for _, req := range requires[c] {
			if err := visit(req); err != nil {
				return err
			}
		}
		path = path[:len(path)-1]
		state[c] = done
		sorted = append(sorted, c)
		return nil
	}

	for _, c := range caps {
		if err := visit(c); err != nil {
			return nil, err
		}
	}
	return sorted, nil
}

// readDependencyManifest loads the manifest for name during dependency
// resolution. requiredBy names the capability whose requires pulled name in,
// or is empty when name was selected directly.
func readDependencyManifest(templateFS fs.FS, name, requiredBy string) (*CapabilityManifest, error) {
	data, err := fs.ReadFile(templateFS, path.Join("templates", "capabilities", name, "capability.yaml"))
	if err != nil {
		if requiredBy != "" && errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("capability %q requires %q, which is not a known capability", requiredBy, name)
		}
		return nil, fmt.Errorf("read capability manifest %q: %w", name, err)
	}
	cm, err := ParseCapabilityManifest(data)
	if err != nil {
		return nil, fmt.Errorf("capability %q: %w", name, err)
	}
	return cm, nil
}

// Suggestions returns capabilities suggested by the selected set but not yet included.
func Suggestions(templateFS fs.FS, capabilities []string) []string {
	capSet := make(map[string]bool, len(capabilities))
	for _, c := range capabilities {
		capSet[c] = true
	}
	seen := map[string]bool{}
	var suggestions []string
	for _, cap := range capabilities {
		capDir := path.Join("templates", "capabilities", cap)
		data, err := fs.ReadFile(templateFS, path.Join(capDir, "capability.yaml"))
		if err != nil {
			continue
		}
		cm, err := ParseCapabilityManifest(data)
		if err != nil {
			continue
		}
		for _, s := range cm.Suggests {
			if !capSet[s] && !seen[s] {
				seen[s] = true
				suggestions = append(suggestions, s)
			}
		}
	}
	return suggestions
}

// MissingFeature is one requires_features entry the target language version
// does not provide.
type MissingFeature struct {
	Requirer string // e.g. `capability "iter"` or `architecture "hexagonal"`
	Feature  string
	Since    string // version that introduced the feature; "" if the matrix does not define it
}

// MissingFeaturesError reports requires_features entries the target language
// version lacks (ADR-007). Callers that know the version and how it was chosen
// add that context; the error itself names each requirer, feature and the
// version that provides it.
type MissingFeaturesError struct {
	Missing []MissingFeature
}

func (e *MissingFeaturesError) Error() string {
	parts := make([]string, 0, len(e.Missing))
	for _, m := range e.Missing {
		if m.Since == "" {
			parts = append(parts, fmt.Sprintf("%s requires feature %q, which the feature matrix does not define", m.Requirer, m.Feature))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s requires feature %q, available from version %s", m.Requirer, m.Feature, m.Since))
	}
	return strings.Join(parts, "; ")
}

// MinimumVersion returns the lowest version that provides every missing
// feature the matrix defines, or "" when none is defined.
func (e *MissingFeaturesError) MinimumVersion() string {
	best := ""
	var bestParts []int
	for _, m := range e.Missing {
		parts, err := parseVersion(m.Since)
		if err != nil {
			continue
		}
		if best == "" || !versionGTE(bestParts, parts) {
			best, bestParts = m.Since, parts
		}
	}
	return best
}

// EnforceRequiredFeatures fails when the architecture or any capability
// declares a requires_features entry that vars["Features"] does not enable.
// With no Features in vars, nothing can be shown to satisfy a requirement, so
// every requirement is reported missing rather than rendering code that may
// not compile. The feature matrix is read from templates/features.yaml in
// templateFS, only to name the version that provides each feature.
func EnforceRequiredFeatures(templateFS fs.FS, arch *Manifest, caps []CapabilityManifest, vars map[string]interface{}) error {
	type requirement struct{ requirer, feature string }
	var reqs []requirement
	if arch != nil {
		for _, f := range arch.RequiresFeatures {
			reqs = append(reqs, requirement{fmt.Sprintf("architecture %q", arch.Name), f})
		}
	}
	for _, cm := range caps {
		for _, f := range cm.RequiresFeatures {
			reqs = append(reqs, requirement{fmt.Sprintf("capability %q", cm.Name), f})
		}
	}
	if len(reqs) == 0 {
		return nil
	}

	features, _ := vars["Features"].(map[string]bool) // absent: every requirement is missing
	since := featureSinceVersions(templateFS)
	var missing []MissingFeature
	for _, r := range reqs {
		if !features[r.feature] {
			missing = append(missing, MissingFeature{Requirer: r.requirer, Feature: r.feature, Since: since[r.feature]})
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &MissingFeaturesError{Missing: missing}
}

// featureSinceVersions maps feature name to the version that introduced it,
// from templates/features.yaml. A missing or malformed matrix yields an empty
// map: the caller still fails the requirement and reports the feature as
// undefined, which is the accurate description of that state.
func featureSinceVersions(templateFS fs.FS) map[string]string {
	out := map[string]string{}
	data, err := fs.ReadFile(templateFS, "templates/features.yaml")
	if err != nil {
		return out
	}
	matrix, err := ParseFeatureMatrix(data)
	if err != nil {
		return out
	}
	for _, f := range matrix.Features {
		out[f.Name] = f.Since
	}
	return out
}
