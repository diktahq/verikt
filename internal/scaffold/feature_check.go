package scaffold

import "fmt"

// CheckRequiredFeatures validates that all required features are available.
// Returns nil if all requirements are met.
// Returns a user-friendly error listing missing features and what version provides them.
func CheckRequiredFeatures(
	required []string,
	resolved map[string]bool,
	matrix *FeatureMatrix,
	detectedVersion string,
) error {
	if len(required) == 0 {
		return nil
	}

	missing := make([]string, 0, len(required))
	for _, req := range required {
		if !resolved[req] {
			missing = append(missing, req)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	msg := fmt.Sprintf("requires features not available in version %s:\n\n  Missing features:\n", detectedVersion)
	for _, m := range missing {
		since := "unknown"
		if matrix != nil {
			for _, f := range matrix.Features {
				if f.Name == m {
					since = f.Since
					break
				}
			}
		}
		msg += fmt.Sprintf("    - %s (requires %s+)\n", m, since)
	}
	msg += fmt.Sprintf("\n  Your detected version: %s\n", detectedVersion)
	msg += "\n  Options:\n"
	msg += "    1. Upgrade your language toolchain\n"
	msg += "    2. Use --go-version flag to target a newer version\n"

	return fmt.Errorf("%s", msg)
}
