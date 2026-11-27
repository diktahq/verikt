package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// SeverityOverride configures a path-scoped severity for a violation rule.
// Severity must be one of: "must", "should", "ignore".
// Reason is required and is shown in guide output and check results.
// Paths are glob patterns (same syntax as component in: fields).
// An empty Paths slice is a catch-all and matches every file.
type SeverityOverride struct {
	Severity string   `yaml:"severity"`
	Reason   string   `yaml:"reason"`
	Paths    []string `yaml:"paths,omitempty"`
}

// SeverityOverrides maps an override key to an ordered list of overrides.
// Keys match violation Rule or Name fields directly — not capability names.
//
// Override key namespace:
//   - Dependency violations:   violation.Rule  → "dependency", "arch/domain", "arch/handler", …
//   - Architecture violations: violation.Rule  → "orphan_package", "missing_component"
//   - Structure violations:    violation.Rule  → "required_dir", "forbidden_dir",
//     "cmd_allowlist", "live_dot_claude_fixture", "silent_ci_swallow",
//     "mutable_install_url", "invalid_allow_directive"
//   - Function violations:     violation.Rule  → "max_lines", "max_params", "max_return_values"
//   - Proxy rules:             violation.RuleID → user-defined IDs from .verikt/rules/ YAML files
//   - Anti-patterns:           antipattern.Name → "sql_concatenation", "god_package", …
//
// First match wins when evaluating a file path.
type SeverityOverrides map[string][]SeverityOverride

type VeriktConfig struct {
	Language          string               `yaml:"language" json:"language"`
	Architecture      string               `yaml:"architecture" json:"architecture"`
	Capabilities      []string             `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	Components        []Component          `yaml:"components" json:"components"`
	Rules             RulesConfig          `yaml:"rules,omitempty" json:"rules,omitempty"`
	Check             CheckConfig          `yaml:"check,omitempty" json:"check,omitempty"`
	Extends           []string             `yaml:"extends,omitempty" json:"extends,omitempty"`
	Templates         TemplateSourceConfig `yaml:"templates,omitempty" json:"templates,omitempty"`
	Decisions         []Decision           `yaml:"decisions,omitempty" json:"decisions,omitempty"`
	Guide             GuideConfig          `yaml:"guide" json:"guide"`
	SeverityOverrides SeverityOverrides    `yaml:"severity_overrides,omitempty" json:"severity_overrides,omitempty"`
}

// CheckConfig controls behaviour of `verikt check`.
type CheckConfig struct {
	// Exclude lists path globs that the checker should ignore entirely.
	// Use this for generated code, vendored dependencies, or tooling dirs
	// that legitimately don't belong to any declared component.
	// Example: ["generated/**", "vendor/**", "tools/**"]
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`

	// AllowedEntrypoints turns on the cmd_allowlist detector (Go projects).
	// Each entry is a project-relative directory holding a `package main`,
	// such as "cmd/server", or "." for a main package in the project root.
	//
	// nil (key absent or null) leaves the detector off. A present but empty
	// list turns it on with nothing approved, so every entrypoint is reported.
	// The pointer is what keeps those two apart through a save and load.
	AllowedEntrypoints *[]string `yaml:"allowed_entrypoints,omitempty" json:"allowed_entrypoints,omitempty"`

	// FailOn is the lowest finding severity that fails `verikt check`: "error"
	// (the default, also when empty) or "warning". The --strict flag sets
	// "warning" for one run and takes precedence over this field.
	FailOn string `yaml:"fail_on,omitempty" json:"fail_on,omitempty"`

	// SilentCISwallow tunes the silent_ci_swallow check, which is always on.
	SilentCISwallow SilentCISwallowConfig `yaml:"silent_ci_swallow,omitempty" json:"silent_ci_swallow,omitempty"`

	// MutableInstallURL tunes the mutable_install_url check, which is always
	// on for CI, script, build and container files.
	MutableInstallURL MutableInstallURLConfig `yaml:"mutable_install_url,omitempty" json:"mutable_install_url,omitempty"`
}

// SilentCISwallowConfig tunes the silent_ci_swallow check.
type SilentCISwallowConfig struct {
	// ExtraCommands are command names, or a command followed by fixed
	// arguments ("deno test"), treated as verification commands in addition
	// to the built-in list, so `<command> || true` on them is reported.
	ExtraCommands []string `yaml:"extra_commands,omitempty" json:"extra_commands,omitempty"`
}

// MutableInstallURLConfig tunes the mutable_install_url check.
type MutableInstallURLConfig struct {
	// IncludeDocs also scans fenced code blocks in README.md and
	// docs/**/*.md. Off by default: install docs routinely show a
	// convenience one-liner.
	IncludeDocs bool `yaml:"include_docs,omitempty" json:"include_docs,omitempty"`
}

// EntrypointAllowlist returns the normalised allowed entrypoints and whether
// the cmd_allowlist detector is on.
func (c CheckConfig) EntrypointAllowlist() ([]string, bool) {
	if c.AllowedEntrypoints == nil {
		return nil, false
	}
	out := make([]string, 0, len(*c.AllowedEntrypoints))
	for _, e := range *c.AllowedEntrypoints {
		out = append(out, NormalizeEntrypoint(e))
	}
	return out, true
}

// NormalizeEntrypoint converts an entrypoint path to its canonical form:
// slash-separated and cleaned, with no leading "./" or trailing "/". The
// project root is ".". An empty or blank entry stays empty.
func NormalizeEntrypoint(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" {
		return ""
	}
	return path.Clean(p)
}

// GuideConfig controls how `verikt guide` instructs AI agents.
type GuideConfig struct {
	// Mode controls the agent instruction style.
	// "passive"  — background context, architecture notes appended at end (default).
	// "audit"    — agent reads codebase on every session and leads with gap analysis.
	// "prompted" — passive mode + suggested prompts appended at the end of the guide.
	Mode             string   `yaml:"mode,omitempty" json:"mode,omitempty"`
	SuggestedPrompts bool     `yaml:"suggested-prompts,omitempty" json:"suggested_prompts,omitempty"`
	Targets          []string `yaml:"targets,omitempty" json:"targets,omitempty"`
}

// GuideMode returns the resolved guide mode, defaulting to "passive".
func (g GuideConfig) GuideMode() string {
	switch g.Mode {
	case "audit", "prompted":
		return g.Mode
	default:
		return "passive"
	}
}

// Decision represents an architecture decision gate entry.
type Decision struct {
	Topic     string `yaml:"topic" json:"topic"`
	Tier      int    `yaml:"tier" json:"tier"`
	Status    string `yaml:"status" json:"status"`
	Choice    string `yaml:"choice,omitempty" json:"choice,omitempty"`
	Rationale string `yaml:"rationale,omitempty" json:"rationale,omitempty"`
	DecidedBy string `yaml:"decided-by,omitempty" json:"decided_by,omitempty"`
	DecidedAt string `yaml:"decided-at,omitempty" json:"decided_at,omitempty"`
}

// Component defines a named architectural component with dependency rules.
type Component struct {
	Name        string   `yaml:"name" json:"name"`
	In          []string `yaml:"in" json:"in"`
	MayDependOn []string `yaml:"may_depend_on" json:"may_depend_on"`
	// Optional marks a component a project may legitimately not have yet, such
	// as platform code that only some capabilities write. An optional component
	// with no code is not reported as missing_component; its dependency rules
	// still apply once code exists.
	Optional bool `yaml:"optional,omitempty" json:"optional,omitempty"`
}

type RulesConfig struct {
	Naming    []NamingRule    `yaml:"naming,omitempty" json:"naming,omitempty"`
	Structure StructureConfig `yaml:"structure,omitempty" json:"structure,omitempty"`
	Functions FunctionRules   `yaml:"functions,omitempty" json:"functions,omitempty"`
}

type NamingRule struct {
	Pattern       string `yaml:"pattern" json:"pattern"`
	MustEndWith   string `yaml:"must_end_with,omitempty" json:"must_end_with,omitempty"`
	MustStartWith string `yaml:"must_start_with,omitempty" json:"must_start_with,omitempty"`
}

type StructureConfig struct {
	RequiredDirs  []string `yaml:"required_dirs,omitempty" json:"required_dirs,omitempty"`
	ForbiddenDirs []string `yaml:"forbidden_dirs,omitempty" json:"forbidden_dirs,omitempty"`
}

type FunctionRules struct {
	MaxLines        int `yaml:"max_lines,omitempty" json:"max_lines,omitempty"`
	MaxParams       int `yaml:"max_params,omitempty" json:"max_params,omitempty"`
	MaxReturnValues int `yaml:"max_return_values,omitempty" json:"max_return_values,omitempty"`
}

type TemplateSourceConfig struct {
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
}

func LoadVeriktYAML(path string) (*VeriktConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read verikt.yaml: %w", err)
	}
	cfg := &VeriktConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse verikt.yaml: %w", err)
	}
	return cfg, nil
}

func SaveVeriktYAML(path string, cfg *VeriktConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if errs := ValidateVeriktYAML(cfg); len(errs) > 0 {
		return errs[0]
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal verikt.yaml: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write verikt.yaml: %w", err)
	}
	return nil
}

func ValidateVeriktYAML(cfg *VeriktConfig) []error {
	if cfg == nil {
		return []error{fmt.Errorf("config is nil")}
	}
	var errs []error
	if strings.TrimSpace(cfg.Language) == "" {
		errs = append(errs, fmt.Errorf("language is required"))
	}
	if strings.TrimSpace(cfg.Architecture) == "" {
		errs = append(errs, fmt.Errorf("architecture is required"))
	}
	seen := map[string]bool{}
	for i, comp := range cfg.Components {
		name := strings.TrimSpace(comp.Name)
		if name == "" {
			errs = append(errs, fmt.Errorf("components[%d].name is required", i))
			continue
		}
		if seen[name] {
			errs = append(errs, fmt.Errorf("components[%d].name %q is duplicated", i, name))
		}
		seen[name] = true
		for _, dep := range comp.MayDependOn {
			if dep == name {
				errs = append(errs, fmt.Errorf("components[%d] %q must not reference itself in may_depend_on", i, name))
			}
		}
	}
	// Validate may_depend_on references exist.
	for i, comp := range cfg.Components {
		for _, dep := range comp.MayDependOn {
			if !seen[dep] {
				errs = append(errs, fmt.Errorf("components[%d] %q references unknown component %q in may_depend_on", i, comp.Name, dep))
			}
		}
	}
	errs = append(errs, ValidateAllowedEntrypoints(cfg.Check)...)
	if err := ValidateFailOn(cfg.Check.FailOn); err != nil {
		errs = append(errs, err)
	}
	errs = append(errs, ValidateSilentCISwallow(cfg.Check.SilentCISwallow)...)
	errs = append(errs, validateSeverityOverrides(cfg.SeverityOverrides)...)
	return errs
}

// ValidateSilentCISwallow rejects check.silent_ci_swallow.extra_commands
// entries that are blank, duplicated, or contain shell syntax. An entry is a
// command name ("migrate-db") or a command followed by fixed arguments
// ("deno test"), never a pipeline or a quoted string.
func ValidateSilentCISwallow(c SilentCISwallowConfig) []error {
	var errs []error
	seen := map[string]bool{}
	for i, raw := range c.ExtraCommands {
		cmd := strings.Join(strings.Fields(raw), " ")
		switch {
		case cmd == "":
			errs = append(errs, fmt.Errorf("check.silent_ci_swallow.extra_commands[%d] is empty", i))
		case strings.ContainsAny(cmd, "|;&$()<>`'\"\\#="):
			errs = append(errs, fmt.Errorf("check.silent_ci_swallow.extra_commands[%d] %q must be a command name, not shell syntax", i, raw))
		case seen[cmd]:
			errs = append(errs, fmt.Errorf("check.silent_ci_swallow.extra_commands[%d] %q is duplicated", i, raw))
		}
		seen[cmd] = true
	}
	return errs
}

// validateSeverityOverrides checks every severity_overrides entry: a known
// severity, a reason, and no catch-all ahead of entries it would shadow.
func validateSeverityOverrides(all SeverityOverrides) []error {
	var errs []error
	for key, overrides := range all {
		for i, o := range overrides {
			if err := validOverrideSeverity(key, i, o); err != nil {
				errs = append(errs, err)
			}
			if strings.TrimSpace(o.Reason) == "" {
				errs = append(errs, overrideReasonRequired(key, i))
			}
			// Warn if a catch-all (no paths) is not the last entry — subsequent entries are unreachable.
			if len(o.Paths) == 0 && i < len(overrides)-1 {
				errs = append(errs, fmt.Errorf("severity_overrides[%q][%d] is a catch-all (no paths) but is not the last entry — subsequent entries are unreachable", key, i))
			}
		}
	}
	return errs
}

// ValidateCheckOverrides is what `verikt check` enforces on severity_overrides
// before it runs: every severity is must, should or ignore, and every waiver
// (ignore) has a reason.
//
// check used to apply overrides unvalidated. `severity: info` turned an
// error-level finding into info, which never fails — not even under --strict —
// and, not being "ignore", left no trace in the WAIVED section. A waiver
// without a reason was accepted too. Both quietly checked less than the file
// appears to say.
//
// It is narrower than the save-time rules on purpose: a must or should entry
// without a reason, or a catch-all ahead of other entries, hides no finding,
// and the published examples write `severity: must` with no reason, so
// refusing to run on them would break configs for no safety gain.
func ValidateCheckOverrides(all SeverityOverrides) []error {
	keys := make([]string, 0, len(all))
	for key := range all {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var errs []error
	for _, key := range keys {
		for i, o := range all[key] {
			if err := validOverrideSeverity(key, i, o); err != nil {
				errs = append(errs, err)
				continue
			}
			if o.Severity == "ignore" && strings.TrimSpace(o.Reason) == "" {
				errs = append(errs, overrideReasonRequired(key, i))
			}
		}
	}
	return errs
}

// validOverrideSeverity rejects anything but must, should and ignore. "info",
// "error" and "warning" are finding severities, not override values.
func validOverrideSeverity(key string, i int, o SeverityOverride) error {
	switch o.Severity {
	case "must", "should", "ignore":
		return nil
	default:
		return fmt.Errorf("severity_overrides[%q][%d].severity %q is invalid (must be one of: must, should, ignore)", key, i, o.Severity)
	}
}

func overrideReasonRequired(key string, i int) error {
	return fmt.Errorf("severity_overrides[%q][%d].reason is required", key, i)
}

// ValidateAllowedEntrypoints rejects allowlist entries that cannot name a
// directory inside the project: empty, absolute, or escaping it with "..".
func ValidateAllowedEntrypoints(c CheckConfig) []error {
	if c.AllowedEntrypoints == nil {
		return nil
	}
	var errs []error
	for i, raw := range *c.AllowedEntrypoints {
		e := NormalizeEntrypoint(raw)
		switch {
		case e == "":
			errs = append(errs, fmt.Errorf("check.allowed_entrypoints[%d] is empty", i))
		case path.IsAbs(e) || filepath.IsAbs(raw):
			errs = append(errs, fmt.Errorf("check.allowed_entrypoints[%d] %q must be a project-relative path", i, raw))
		case e == ".." || strings.HasPrefix(e, "../"):
			errs = append(errs, fmt.Errorf("check.allowed_entrypoints[%d] %q must not leave the project", i, raw))
		}
	}
	return errs
}

// ValidateCapabilities reports capabilities declared in verikt.yaml that the
// language provider does not define. known is injected by the caller because
// config must not depend on the provider package.
//
// An empty known set means the provider exposed no templates — validating
// against it would reject everything, so validation is skipped.
func ValidateCapabilities(cfg *VeriktConfig, known []string) []error {
	if cfg == nil || len(known) == 0 {
		return nil
	}

	knownSet := make(map[string]bool, len(known))
	for _, k := range known {
		knownSet[k] = true
	}

	var errs []error
	for i, c := range cfg.Capabilities {
		name := strings.TrimSpace(c)
		if name == "" {
			errs = append(errs, fmt.Errorf("capabilities[%d] is empty", i))
			continue
		}
		if !knownSet[name] {
			errs = append(errs, fmt.Errorf("capabilities[%d] %q is not a known %s capability", i, name, cfg.Language))
		}
	}
	return errs
}

// projectManifests mark the root of a distinct project. Finding one while
// searching upwards means the search has left the project it started in.
var projectManifests = []string{"go.mod", "package.json"}

// FindVeriktYAML locates the verikt.yaml governing startDir by searching it and
// then its ancestors.
//
// The search stops at a project boundary: a directory holding a module manifest
// (go.mod, package.json) but no verikt.yaml is the root of its own project, and
// the parent's configuration does not describe it. Walking past such a boundary
// checked nested projects against an unrelated config, where none of the declared
// components matched and every package looked orphaned.
//
// A config beside a manifest in the same directory is still found — a project
// root normally holds both.
func FindVeriktYAML(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve start dir: %w", err)
	}
	for {
		candidate := filepath.Join(dir, "verikt.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		if isProjectRoot(dir) {
			return "", os.ErrNotExist
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// isProjectRoot reports whether dir holds a manifest that marks its own project.
func isProjectRoot(dir string) bool {
	for _, manifest := range projectManifests {
		if _, err := os.Stat(filepath.Join(dir, manifest)); err == nil {
			return true
		}
	}
	return false
}

func DefaultVeriktConfig(language, architecture string) *VeriktConfig {
	language = strings.TrimSpace(language)
	if language == "" {
		language = "go"
	}
	architecture = strings.TrimSpace(architecture)
	if architecture == "" {
		architecture = "hexagonal"
	}

	cfg := &VeriktConfig{
		Language:     language,
		Architecture: architecture,
		Rules: RulesConfig{
			Functions: FunctionRules{MaxLines: 80, MaxParams: 4, MaxReturnValues: 2},
		},
	}

	if language == "typescript" {
		return defaultTypeScriptConfig(cfg, architecture)
	}

	applyGoArchitecture(cfg, architecture)
	return cfg
}

// applyGoArchitecture sets the components, structure rules and template source
// of a Go architecture's default config. Unknown names get hexagonal.
func applyGoArchitecture(cfg *VeriktConfig, architecture string) {
	forbidden := []string{"utils/", "helpers/"}
	switch architecture {
	case "flat":
		cfg.Components = nil
		cfg.Rules.Structure = StructureConfig{ForbiddenDirs: forbidden}
		cfg.Templates = TemplateSourceConfig{Source: "verikt/cli"}
	case "clean":
		cfg.Components = goComponents("entity",
			Component{Name: "entity", In: []string{"internal/entity/**"}, MayDependOn: []string{}},
			Component{Name: "usecase", In: []string{"internal/usecase/**"}, MayDependOn: []string{"entity"}},
			Component{Name: "interface", In: []string{"internal/interface/**"}, MayDependOn: []string{"usecase", "entity", "platform"}},
			Component{Name: "infrastructure", In: []string{"internal/infrastructure/**"}, MayDependOn: []string{"interface", "usecase", "entity", "platform"}},
		)
		cfg.Rules.Structure = StructureConfig{
			RequiredDirs:  []string{"cmd/", "internal/entity/", "internal/usecase/", "internal/interface/", "internal/infrastructure/"},
			ForbiddenDirs: forbidden,
		}
		cfg.Templates = TemplateSourceConfig{Source: "verikt/api"}
	case "layered":
		cfg.Components = goComponents("model",
			Component{Name: "handler", In: []string{"internal/handler/**"}, MayDependOn: []string{"service", "model", "platform"}},
			Component{Name: "service", In: []string{"internal/service/**"}, MayDependOn: []string{"repository", "model"}},
			Component{Name: "repository", In: []string{"internal/repository/**"}, MayDependOn: []string{"model", "platform"}},
			Component{Name: "model", In: []string{"internal/model/**"}, MayDependOn: []string{}},
		)
		cfg.Rules.Structure = StructureConfig{
			RequiredDirs:  []string{"cmd/", "internal/handler/", "internal/service/", "internal/repository/", "internal/model/"},
			ForbiddenDirs: forbidden,
		}
		cfg.Templates = TemplateSourceConfig{Source: "verikt/api"}
	default: // hexagonal
		cfg.Components = goComponents("domain",
			Component{Name: "domain", In: []string{"domain/**"}, MayDependOn: []string{}},
			Component{Name: "ports", In: []string{"port/**"}, MayDependOn: []string{"domain"}},
			Component{Name: "service", In: []string{"service/**"}, MayDependOn: []string{"domain", "ports"}},
			Component{Name: "adapters", In: []string{"adapter/**"}, MayDependOn: []string{"ports", "domain", "platform"}},
		)
		cfg.Rules.Structure = StructureConfig{
			RequiredDirs:  []string{"cmd/", "domain/", "port/", "adapter/"},
			ForbiddenDirs: forbidden,
		}
		cfg.Templates = TemplateSourceConfig{Source: "verikt/api"}
	}
}

// platformDirs are the directories Go capability templates write cross-cutting
// code into (observability, lifecycle, resilience, event bus, config, ...).
var platformDirs = []string{"platform/**", "config/**", "internal/lifecycle/**", "internal/migrate/**", "internal/validation/**", "internal/testutil/**"}

// goComponents appends the two components every layered Go architecture
// shares to its layers. innermost names the only layer platform code may use.
//
//   - platform: capability-written cross-cutting code. Optional, because a
//     project scaffolded without such capabilities has none.
//   - app: the composition root (cmd/ entrypoints and internal/bootstrap). It
//     wires everything together, so it may depend on every component and
//     nothing may depend on it. It replaces `check.exclude: [cmd/**]`, which
//     hid entrypoints from orphan detection instead of classifying them.
//
// Component patterns match at any depth and the first matching component
// wins, so platform comes after the layers: internal/infrastructure/config
// must stay infrastructure rather than fall to platform's config/** glob.
func goComponents(innermost string, layers ...Component) []Component {
	all := make([]string, 0, len(layers)+1)
	for _, l := range layers {
		all = append(all, l.Name)
	}
	all = append(all, "platform")
	out := make([]Component, 0, len(layers)+2)
	out = append(out, layers...)
	return append(out,
		Component{Name: "platform", In: append([]string{}, platformDirs...), MayDependOn: []string{innermost}, Optional: true},
		Component{Name: "app", In: []string{"cmd/**", "internal/bootstrap/**"}, MayDependOn: all},
	)
}

func defaultTypeScriptConfig(cfg *VeriktConfig, architecture string) *VeriktConfig {
	switch architecture {
	case "flat":
		cfg.Components = nil
		cfg.Rules.Structure = StructureConfig{
			ForbiddenDirs: []string{"utils/", "helpers/"},
		}
		cfg.Templates = TemplateSourceConfig{Source: "verikt/typescript/flat"}
	default: // hexagonal
		cfg.Components = []Component{
			{Name: "domain", In: []string{"src/domain/**"}, MayDependOn: []string{}},
			{Name: "application", In: []string{"src/application/**"}, MayDependOn: []string{"domain"}},
			{Name: "infrastructure", In: []string{"src/infrastructure/**"}, MayDependOn: []string{"domain", "application"}},
			{Name: "transport", In: []string{"src/transport/**"}, MayDependOn: []string{"domain", "application", "infrastructure"}},
		}
		cfg.Rules.Structure = StructureConfig{
			RequiredDirs:  []string{"src/domain/", "src/application/", "src/infrastructure/", "src/transport/"},
			ForbiddenDirs: []string{"utils/", "helpers/"},
		}
		cfg.Templates = TemplateSourceConfig{Source: "verikt/typescript/hexagonal"}
	}
	return cfg
}
