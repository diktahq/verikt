package scaffold

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Feature represents a language feature gated by a minimum version.
type Feature struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Since       string `yaml:"since"`
}

// FeatureMatrix holds a collection of version-gated features.
type FeatureMatrix struct {
	Features []Feature `yaml:"features"`
}

// ParseFeatureMatrix parses a features.yaml file into a FeatureMatrix.
func ParseFeatureMatrix(data []byte) (*FeatureMatrix, error) {
	fm := &FeatureMatrix{}
	if err := yaml.Unmarshal(data, fm); err != nil {
		return nil, fmt.Errorf("parse feature matrix: %w", err)
	}
	for _, f := range fm.Features {
		if strings.TrimSpace(f.Name) == "" {
			return nil, fmt.Errorf("feature missing name")
		}
		if strings.TrimSpace(f.Since) == "" {
			return nil, fmt.Errorf("feature %q missing since version", f.Name)
		}
	}
	return fm, nil
}

// ResolveFeatures returns a map of feature names to booleans, where each
// feature whose Since version is <= detectedVersion is set to true.
func ResolveFeatures(detectedVersion string, matrix *FeatureMatrix) (map[string]bool, error) {
	if matrix == nil {
		return make(map[string]bool), nil
	}
	detected, err := parseVersion(detectedVersion)
	if err != nil {
		return nil, fmt.Errorf("parse detected version %q: %w", detectedVersion, err)
	}
	result := make(map[string]bool, len(matrix.Features))
	for _, f := range matrix.Features {
		since, err := parseVersion(f.Since)
		if err != nil {
			return nil, fmt.Errorf("parse since version %q for feature %q: %w", f.Since, f.Name, err)
		}
		result[f.Name] = versionGTE(detected, since)
	}
	return result, nil
}

// parseVersion splits a dotted version string into integer segments.
func parseVersion(v string) ([]int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, fmt.Errorf("empty version string")
	}
	parts := strings.Split(v, ".")
	segments := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid version segment %q: %w", p, err)
		}
		segments = append(segments, n)
	}
	return segments, nil
}

// versionGTE returns true if a >= b using dotted numeric comparison.
// Missing segments are treated as 0 (e.g., "1" is equivalent to "1.0").
func versionGTE(a, b []int) bool {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	for i := range maxLen {
		av, bv := 0, 0
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av > bv {
			return true
		}
		if av < bv {
			return false
		}
	}
	return true // equal
}
