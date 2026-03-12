package scaffold

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Name             string                     `yaml:"name" json:"name"`
	Description      string                     `yaml:"description" json:"description"`
	Language         string                     `yaml:"language" json:"language"`
	Version          string                     `yaml:"version" json:"version"`
	Variables        []VariableDefinition       `yaml:"variables" json:"variables"`
	Hooks            []string                   `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	RequiresFeatures []string                   `yaml:"requires_features,omitempty" json:"requires_features,omitempty"`
	Conditional      map[string]ConditionalRule `yaml:"conditional,omitempty" json:"conditional,omitempty"`
	PathMappings     map[string]string          `yaml:"path_mappings,omitempty" json:"path_mappings,omitempty"`
}

// ConditionalRule gates file inclusion on a feature flag.
// If the feature is true: include-list files are included, exclude-list files are skipped.
// If the feature is false: include-list files are skipped, exclude-list files are included.
type ConditionalRule struct {
	Include []string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// ValidateConditionalRules checks for files that appear in both include and exclude
// lists of the same feature, which is always a bug.
func ValidateConditionalRules(conditionals map[string]ConditionalRule) []string {
	var warnings []string
	for feature, rule := range conditionals {
		includeSet := make(map[string]bool, len(rule.Include))
		for _, f := range rule.Include {
			includeSet[f] = true
		}
		for _, f := range rule.Exclude {
			if includeSet[f] {
				warnings = append(warnings, fmt.Sprintf("feature %q: file %q in both include and exclude", feature, f))
			}
		}
	}
	return warnings
}

// shouldIncludeFile checks conditional rules against resolved features.
// relPath is the template-relative path (pre-render).
// Precedence: exclude wins over include if conflicting.
// Returns true if conditionals or features are nil (backward compat).
func shouldIncludeFile(relPath string, conditionals map[string]ConditionalRule, features map[string]bool) bool {
	if conditionals == nil || features == nil {
		return true
	}
	for featureName, rule := range conditionals {
		active := features[featureName]
		// If feature is active and file is in exclude list → skip
		if active {
			for _, p := range rule.Exclude {
				if p == relPath {
					return false
				}
			}
		}
		// If feature is NOT active and file is in include list → skip
		if !active {
			for _, p := range rule.Include {
				if p == relPath {
					return false
				}
			}
		}
	}
	return true
}

type VariableDefinition struct {
	Name        string   `yaml:"name" json:"name"`
	Type        string   `yaml:"type" json:"type"`
	Description string   `yaml:"description" json:"description"`
	Default     string   `yaml:"default,omitempty" json:"default,omitempty"`
	Required    bool     `yaml:"required" json:"required"`
	Choices     []string `yaml:"choices,omitempty" json:"choices,omitempty"`
}

func ParseManifest(data []byte) (*Manifest, error) {
	m := &Manifest{}
	if err := yaml.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if strings.TrimSpace(m.Name) == "" {
		return nil, fmt.Errorf("manifest missing name")
	}
	for i := range m.Variables {
		v := &m.Variables[i]
		if strings.TrimSpace(v.Type) == "" {
			v.Type = "string"
		}
	}
	return m, nil
}

func (m *Manifest) Defaults() map[string]interface{} {
	out := make(map[string]interface{}, len(m.Variables))
	for _, variable := range m.Variables {
		if variable.Default == "" {
			continue
		}
		switch variable.Type {
		case "bool":
			out[variable.Name] = strings.EqualFold(variable.Default, "true")
		default:
			out[variable.Name] = variable.Default
		}
	}
	return out
}
