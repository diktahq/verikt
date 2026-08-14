package checker

// AntiPattern is a detector finding.
//
// The detectors themselves live in the Rust engine
// (engine/crates/engine-bin/src/antipatterns.rs). This type is the Go-side shape the
// engine's findings are converted into; the json tags match Violation so a single
// `verikt check --output json` document does not use two key casings.
type AntiPattern struct {
	Name     string `json:"name"`     // Short identifier: "global_state", "init_abuse", etc.
	Category string `json:"category"` // "code", "architecture", "security"
	Severity string `json:"severity"` // "error", "warning", "info"
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}
