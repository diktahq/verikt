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
//go:embed bin
var engineFS embed.FS

// engineBinary is the engine for the platform this binary was built for, or nil
// when none was embedded.
//
// A nil binary is a supported configuration, not an error: EnginePath reports it
// and callers fall back to Go-native analysis. That was already true for platforms
// with no engine build; it is now also true for a local build that has not produced
// one.
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
