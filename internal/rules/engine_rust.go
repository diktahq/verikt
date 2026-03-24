package rules

import (
	"context"
	"fmt"

	"github.com/diktahq/verikt/internal/engineclient"
	pb "github.com/diktahq/verikt/internal/engineclient/pb"
)

// runGrepViaRust sends all grep rules to the Rust engine in a single call and
// returns violations and updated rule statuses. It is a drop-in replacement for
// running grep rules one-by-one via RunGrep.
func runGrepViaRust(client *engineclient.Client, grepRules []Rule, projectRoot string, targetFiles []string) ([]RuleViolation, map[string]pb.RuleStatus_Status, error) {
	pbRules := make([]*pb.Rule, 0, len(grepRules))
	for _, r := range grepRules {
		pbRules = append(pbRules, ruleToProto(r))
	}

	result, err := client.Check(context.Background(), projectRoot, pbRules, targetFiles)
	if err != nil {
		return nil, nil, fmt.Errorf("rust engine check: %w", err)
	}

	violations := make([]RuleViolation, 0, len(result.Findings))
	for _, f := range result.Findings {
		violations = append(violations, findingToViolation(f))
	}

	statuses := make(map[string]pb.RuleStatus_Status, len(result.Summary.RuleStatuses))
	for _, s := range result.Summary.RuleStatuses {
		statuses[s.RuleId] = s.Status
	}

	return violations, statuses, nil
}

// ruleToProto converts a Rule to its protobuf representation.
func ruleToProto(r Rule) *pb.Rule {
	return &pb.Rule{
		Id:       r.ID,
		Severity: severityToProto(r.Severity),
		Message:  r.Description,
		Engine:   pb.EngineType_GREP,
		Scope: &pb.RuleScope{
			Include: r.Scope,
			Exclude: r.Exclude,
		},
		Spec: &pb.Rule_Grep{
			Grep: &pb.GrepSpec{
				Pattern:         r.Pattern,
				MustContain:     r.MustContain,
				MustNotContain:  r.MustNotContain,
				FileMustContain: r.FileMustContain,
			},
		},
	}
}

// findingToViolation converts a protobuf Finding to a RuleViolation.
func findingToViolation(f *pb.Finding) RuleViolation {
	return RuleViolation{
		RuleID:      f.RuleId,
		Engine:      "grep",
		Description: f.Message,
		Severity:    severityFromProto(f.Severity),
		File:        f.File,
		Line:        int(f.Line),
		Match:       f.Match,
	}
}

func severityToProto(s string) pb.Severity {
	switch s {
	case "error":
		return pb.Severity_ERROR
	case "warning":
		return pb.Severity_WARNING
	default:
		return pb.Severity_INFO
	}
}

func severityFromProto(s pb.Severity) string {
	switch s {
	case pb.Severity_ERROR:
		return "error"
	case pb.Severity_WARNING:
		return "warning"
	default:
		return "info"
	}
}
