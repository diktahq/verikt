package scaffold

import (
	"context"
	"fmt"
	"regexp"
	"strings"

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
	if err != nil {
		return nil, fmt.Errorf("load feature matrix: %w", err)
	}
	if matrixData == nil {
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
	if err != nil {
		return nil, fmt.Errorf("detect language version: %w", err)
	}
	if version == "" {
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

// VersionSource records where a target language version came from, so an
// error can tell the user which knob to turn.
type VersionSource string

const (
	// VersionExplicit is a version the user set, e.g. --set GoVersion=1.24.
	VersionExplicit VersionSource = "explicit"
	// VersionProject is the version an existing project declares, e.g. the
	// go directive in go.mod.
	VersionProject VersionSource = "project"
	// VersionDetected is the version of the installed toolchain.
	VersionDetected VersionSource = "detected"
	// VersionDefault is the architecture manifest's default, used when no
	// toolchain could be detected.
	VersionDefault VersionSource = "default"
)

// VersionCandidate is a version pinned by the caller, with its origin.
type VersionCandidate struct {
	Version string
	Source  VersionSource
}

// Target is the language version a scaffold targets and the features that
// version provides.
type Target struct {
	Version  string // as written to project files, e.g. "1.26" or "1.24.3"
	Source   VersionSource
	Features map[string]bool // never nil
}

// releaseVersion matches a dotted release version with an optional Go-style
// pre-release suffix: 1.22, 1.24.3, 1.23rc1.
var releaseVersion = regexp.MustCompile(`^(\d+\.\d+(?:\.\d+)?)(?:(?:rc|beta)\d+)?$`)

// featureVersion returns the numeric part of v that the feature matrix is
// compared against, or "" when v is not a release version. A pre-release
// counts as its release: 1.23rc1 already ships the 1.23 language features.
func featureVersion(v string) string {
	m := releaseVersion.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return ""
	}
	return m[1]
}

// ResolveTarget picks the version a scaffold targets and resolves the
// provider's feature matrix against it (ADR-007). Precedence: the pinned
// version when set, then the installed toolchain, then fallback. A pinned
// version that is not a release version is an error; an undetectable or
// unparseable toolchain version is not, because a missing toolchain is
// exactly the case the fallback exists for.
func (r *FeatureResolver) ResolveTarget(ctx context.Context, p provider.LanguageProvider, pinned VersionCandidate, fallback string) (*Target, error) {
	target, err := selectVersion(ctx, p, pinned, fallback)
	if err != nil || target.Version == "" {
		return target, err
	}

	fmp, ok := p.(provider.FeatureMatrixProvider)
	if !ok {
		return target, nil
	}
	matrixData, err := fmp.GetFeatureMatrix()
	if err != nil {
		return nil, fmt.Errorf("load feature matrix: %w", err)
	}
	numeric := featureVersion(target.Version)
	if matrixData == nil || numeric == "" {
		return target, nil
	}
	features, err := r.ResolveWithVersion(numeric, matrixData)
	if err != nil {
		return nil, err
	}
	target.Features = features
	return target, nil
}

// selectVersion applies ResolveTarget's precedence; the returned Target has
// no features yet.
func selectVersion(ctx context.Context, p provider.LanguageProvider, pinned VersionCandidate, fallback string) (*Target, error) {
	target := &Target{Features: map[string]bool{}}
	if v := strings.TrimSpace(pinned.Version); v != "" {
		if featureVersion(v) == "" {
			return nil, fmt.Errorf("invalid language version %q: want a release version such as 1.24 or 1.24.3", pinned.Version)
		}
		target.Version, target.Source = v, pinned.Source
		return target, nil
	}
	if v := detectedVersion(ctx, p); v != "" {
		target.Version, target.Source = v, VersionDetected
		return target, nil
	}
	if v := strings.TrimSpace(fallback); v != "" {
		target.Version, target.Source = v, VersionDefault
	}
	return target, nil
}

// detectedVersion returns the provider's installed toolchain version, or ""
// when the provider cannot detect one, detection fails, or the result is not a
// release version (a devel build, for instance). Every one of those cases means
// "fall back", so the detection error is deliberately not propagated.
func detectedVersion(ctx context.Context, p provider.LanguageProvider) string {
	vd, ok := p.(provider.VersionDetector)
	if !ok {
		return ""
	}
	v, err := vd.DetectVersion(ctx)
	if err != nil || featureVersion(v) == "" {
		return ""
	}
	return strings.TrimSpace(v)
}
