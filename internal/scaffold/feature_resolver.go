package scaffold

import (
	"context"
	"fmt"

	"github.com/diktahq/verikt/internal/provider"
)

// FeatureResolver resolves language features for template rendering.
type FeatureResolver struct{}

// Resolve auto-detects version and resolves features.
// Uses type assertions for optional interfaces — gracefully returns empty map
// if provider doesn't implement VersionDetector or FeatureMatrixProvider.
func (r *FeatureResolver) Resolve(ctx context.Context, p provider.LanguageProvider) (map[string]bool, error) {
	fmp, ok := p.(provider.FeatureMatrixProvider)
	if !ok {
		return map[string]bool{}, nil
	}

	matrixData, err := fmp.GetFeatureMatrix()
	if err != nil || matrixData == nil {
		return map[string]bool{}, nil
	}

	matrix, err := ParseFeatureMatrix(matrixData)
	if err != nil {
		return nil, fmt.Errorf("parse feature matrix: %w", err)
	}

	vd, ok := p.(provider.VersionDetector)
	if !ok {
		return map[string]bool{}, nil
	}

	version, err := vd.DetectVersion(ctx)
	if err != nil || version == "" {
		return map[string]bool{}, nil
	}

	return ResolveFeatures(version, matrix)
}

// ResolveWithVersion resolves features against an explicit version.
// Used when the caller already knows the target version (e.g., from CLI flag).
func (r *FeatureResolver) ResolveWithVersion(version string, matrixData []byte) (map[string]bool, error) {
	if matrixData == nil {
		return map[string]bool{}, nil
	}

	matrix, err := ParseFeatureMatrix(matrixData)
	if err != nil {
		return nil, fmt.Errorf("parse feature matrix: %w", err)
	}

	return ResolveFeatures(version, matrix)
}
