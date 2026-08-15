package guide

import (
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/config"
	"github.com/stretchr/testify/assert"
)

const interviewHeading = "## AI Interview: Scaffold a New Service"

func normativeOpts() GenerateOptions {
	return GenerateOptions{
		Architecture: "hexagonal",
		Capabilities: []string{"http-api", "postgres"},
		Components: []config.Component{
			{Name: "domain", In: []string{"domain/**"}},
			{Name: "adapters", In: []string{"adapter/**"}, MayDependOn: []string{"domain"}},
		},
	}
}

// The scaffold interview was emitted into every guide, including guides generated
// from an existing verikt.yaml where there is nothing left to scaffold. At 664
// tokens it was the single largest section — 23% of the monolithic file — spent on
// content that does not steer work in an existing codebase.
//
// It belongs in catalog-only output, which is what a project without a verikt.yaml
// receives.
func TestBuildContent_InterviewProtocolOnlyInCatalogOnly(t *testing.T) {
	normative := buildContent(normativeOpts())
	assert.NotContains(t, normative, interviewHeading,
		"a guide generated from verikt.yaml has nothing to scaffold")

	catalogOpts := normativeOpts()
	catalogOpts.CatalogOnly = true
	assert.Contains(t, buildContent(catalogOpts), interviewHeading,
		"a project with no verikt.yaml still needs the setup interview")
}

// The Claude target receives split rules files, which are steering rules rather
// than onboarding instructions.
func TestBuildSplitContent_ExcludesInterviewProtocol(t *testing.T) {
	assert.NotContains(t, buildSplitContent(normativeOpts()).Index, interviewHeading)
}

// `verikt init --ai` composes the protocol itself and must be unaffected: gating the
// copy inside the guide must not remove the capability from where it is used.
func TestInterviewProtocol_StillAvailableStandalone(t *testing.T) {
	protocol := InterviewProtocol()
	assert.Contains(t, protocol, interviewHeading)
	assert.Contains(t, protocol, "Greenfield Flow")
}

// The two ways idiomatic Go panics are worth steering against proactively, not only
// reporting after the fact.
func TestWriteAntiPatterns_CoversPanicSources(t *testing.T) {
	var b strings.Builder
	writeAntiPatterns(&b, "hexagonal")
	content := b.String()

	assert.Contains(t, content, "make(map[K]V)", "nil map write guidance")
	assert.Contains(t, content, "v, ok := x.(T)", "type assertion guidance")
}
