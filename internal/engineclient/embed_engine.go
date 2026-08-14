//go:build !verikt_release

package engineclient

import (
	"embed"
	"fmt"
	"runtime"
)

// engineFS holds the engine binaries placed under bin/ at build time.
//
// The binaries are gitignored build artefacts, so the pattern is the directory
// rather than a specific file: a per-platform `//go:embed bin/verikt-engine-...`
// fails at compile time when the file is absent, which meant a fresh clone could
// not run `go build`, `go vet` or `go test` at all, and `go install` could never
// work. bin/README.md is committed so the pattern always matches.
//
// The cost is that every file in bin/ is embedded, so a tree holding all four
// platform binaries produces a binary carrying all four. That is fine locally,
// where at most one is present, and wrong for a release, where the workflow
// stages all of them for a single parallel GoReleaser run — hence the
// verikt_release build tag, which switches to embed_engine_release.go and
// embeds exactly the target platform's engine.
//
//go:embed bin
var engineFS embed.FS

// engineBinary is the engine for the platform this binary was built for, or nil
// when none was embedded.
//
// A nil binary is a supported configuration, not an error: EnginePath reports it
// and `verikt check` fails with ErrEngineRequired rather than falling back to a
// second, divergent implementation (ADR-006, ADR-011). That was already true for
// platforms with no engine build; it is now also true for a local build that has
// not produced one.
var engineBinary = embeddedEngineFor(runtime.GOOS, runtime.GOARCH)

// embeddedEngineFor returns the embedded engine for a platform, or nil if absent.
func embeddedEngineFor(goos, goarch string) []byte {
	data, err := engineFS.ReadFile(engineBinaryName(goos, goarch))
	if err != nil {
		return nil
	}
	return data
}

// engineBinaryName is the embedded path for a platform's engine. The release
// workflow writes exactly these names.
func engineBinaryName(goos, goarch string) string {
	return fmt.Sprintf("bin/verikt-engine-%s-%s", goos, goarch)
}
