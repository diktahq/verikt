package rules

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/dcsg/archway/internal/engineclient"
	pb "github.com/dcsg/archway/internal/engineclient/pb"
)

// RunRules loads rules from rulesDir, validates them, and runs valid rules.
// If client is non-nil, all grep rules are batched into a single Rust engine
// call (faster, single file walk). If client is nil, the Go grep engine is used.
func RunRules(rulesDir, projectRoot string, allowedFiles []string, client *engineclient.Client) (*RunResult, error) {
	start := time.Now()

	result := &RunResult{}

	if _, err := os.Stat(rulesDir); errors.Is(err, fs.ErrNotExist) {
		result.Duration = time.Since(start)
		return result, nil
	}

	allRules, statuses, err := LoadRules(rulesDir, projectRoot)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}
	result.Statuses = statuses

	// Separate grep rules from AST rules, skipping stale ones.
	staleIDs := staleRuleIDs(statuses)
	var grepRules, astRules []Rule
	for _, rule := range allRules {
		if staleIDs[rule.ID] {
			continue
		}
		switch rule.Engine {
		case "grep":
			grepRules = append(grepRules, rule)
		case "ast":
			astRules = append(astRules, rule)
		}
	}

	// Run grep rules — via Rust engine (single walk) or Go fallback.
	if len(grepRules) > 0 {
		if client != nil {
			violations, engineStatuses, err := runGrepViaRust(client, grepRules, projectRoot, allowedFiles)
			if err != nil {
				// Engine unavailable — fall back to Go grep.
				result.Violations = append(result.Violations, runGrepFallback(grepRules, projectRoot, allowedFiles, result)...)
			} else {
				result.Violations = append(result.Violations, violations...)
				applyEngineStatuses(result, engineStatuses)
			}
		} else {
			result.Violations = append(result.Violations, runGrepFallback(grepRules, projectRoot, allowedFiles, result)...)
		}
	}

	// Run AST rules (Go only — Rust AST engine is v2.0).
	for _, rule := range astRules {
		violations, err := RunAST(rule, projectRoot, allowedFiles)
		if err != nil {
			markRuleError(result, rule.ID, err)
			continue
		}
		result.Violations = append(result.Violations, violations...)
	}

	result.Duration = time.Since(start)
	return result, nil
}

// runGrepFallback runs grep rules one-by-one using the Go engine.
func runGrepFallback(grepRules []Rule, projectRoot string, allowedFiles []string, result *RunResult) []RuleViolation {
	var violations []RuleViolation
	for _, rule := range grepRules {
		v, err := RunGrep(rule, projectRoot, allowedFiles)
		if err != nil {
			markRuleError(result, rule.ID, err)
			continue
		}
		violations = append(violations, v...)
	}
	return violations
}

// applyEngineStatuses updates result.Statuses with stale/invalid signals from the engine.
func applyEngineStatuses(result *RunResult, engineStatuses map[string]pb.RuleStatus_Status) {
	for i, s := range result.Statuses {
		engineStatus, ok := engineStatuses[s.Rule.ID]
		if !ok {
			continue
		}
		switch engineStatus {
		case pb.RuleStatus_STALE:
			result.Statuses[i].Status = "stale"
			result.Statuses[i].Error = "scope matches 0 files"
		case pb.RuleStatus_INVALID:
			result.Statuses[i].Status = "invalid"
			result.Statuses[i].Error = "engine reported invalid rule"
		}
	}
}

// staleRuleIDs returns a set of rule IDs whose status is stale.
func staleRuleIDs(statuses []RuleStatus) map[string]bool {
	ids := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		if s.Status == "stale" {
			ids[s.Rule.ID] = true
		}
	}
	return ids
}

// markRuleError sets a rule's status to invalid with the given error.
func markRuleError(result *RunResult, ruleID string, err error) {
	for i, s := range result.Statuses {
		if s.Rule.ID == ruleID {
			result.Statuses[i].Status = "invalid"
			result.Statuses[i].Error = fmt.Sprintf("execution error: %v", err)
			return
		}
	}
}
