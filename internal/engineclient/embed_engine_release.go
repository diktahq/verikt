//go:build verikt_release

// Release builds embed exactly one engine: the one for the platform being built.
//
// The development build embeds the whole bin/ directory, because the binaries are
// gitignored and a per-platform pattern fails to compile when the file is absent
// — a fresh clone could not `go build` at all. That tolerance is the right
// trade-off locally, where at most one engine is present, and the wrong one for a
// release: the workflow stages all four platform engines for a single GoReleaser
// run, so every published artefact carried every engine, roughly four times the
// payload it needs.
//
// GoReleaser builds targets in parallel from one source tree, so the selection
// cannot be done by staging files per target. It is done here, at compile time.
package engineclient

import _ "embed"

// engineBinary is the engine for the platform this binary was built for.
var engineBinary = releaseEngine
