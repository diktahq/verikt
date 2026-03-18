package engineclient

import (
	"context"
	"fmt"

	pb "github.com/dcsg/archway/internal/engineclient/pb"
)

// AntiPattern mirrors checker.AntiPattern but lives here to avoid import cycles.
// The checker package defines its own AntiPattern type; the CLI converts between them.
type AntiPatternResult struct {
	Name     string
	Category string
	Severity string
	File     string
	Line     int
	Message  string
}

// CheckAntiPatterns runs all anti-pattern detectors (or a subset if detectors is non-nil)
// via the Rust engine and returns results in a form the checker package can consume.
//
// The checker.AntiPatternClient interface is satisfied by wrapping this in an adapter
// so that the checker package does not import engineclient directly.
func (c *Client) CheckAntiPatterns(projectPath string, detectors []string) ([]AntiPatternResult, error) {
	rule := &pb.Rule{
		Id:       "anti-patterns",
		Severity: pb.Severity_WARNING,
		Message:  "anti-pattern detected",
		Engine:   pb.EngineType_ANTI_PATTERN,
		Spec: &pb.Rule_AntiPattern{
			AntiPattern: &pb.AntiPatternSpec{
				Detectors: detectors,
			},
		},
	}

	result, err := c.Check(context.Background(), projectPath, []*pb.Rule{rule}, nil)
	if err != nil {
		return nil, fmt.Errorf("engine anti-pattern check: %w", err)
	}

	out := make([]AntiPatternResult, 0, len(result.Findings))
	for _, f := range result.Findings {
		out = append(out, AntiPatternResult{
			Name:     f.Match, // Match field carries the detector name.
			Category: detectorCategory(f.Match),
			Severity: severityString(f.Severity),
			File:     f.File,
			Line:     int(f.Line),
			Message:  f.Message,
		})
	}
	return out, nil
}

func detectorCategory(detector string) string {
	switch detector {
	case "sql_concatenation":
		return "security"
	case "god_package", "fat_handler", "domain_imports_adapter", "mvc_in_hexagonal":
		return "architecture"
	default:
		return "code"
	}
}

func severityString(s pb.Severity) string {
	switch s {
	case pb.Severity_ERROR:
		return "error"
	case pb.Severity_WARNING:
		return "warning"
	case pb.Severity_INFO:
		return "info"
	default:
		return "warning"
	}
}
