//go:build verikt_release && !(linux && amd64) && !(linux && arm64) && !(darwin && amd64) && !(darwin && arm64)

package engineclient

// releaseEngine is empty on platforms with no engine build (Windows today).
//
// EnginePath reports the absence and `verikt check` fails with ErrEngineRequired,
// rather than falling back to a second implementation that would disagree with
// this one (ADR-006, ADR-011).
var releaseEngine []byte
