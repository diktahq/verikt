# Engine binaries

The Rust analysis engine is embedded into the `verikt` binary from this directory by
`//go:embed bin` in `../embed.go`. The binaries themselves are **not committed** —
they are build artefacts, and they are gitignored.

This file is committed so the embed pattern always matches at least one file. Without
it, `//go:embed` fails at compile time and nothing in the module builds on a fresh
clone.

Build one for your platform with:

```bash
mise run build-engine
```

Filenames must be `verikt-engine-<GOOS>-<GOARCH>`, for example
`verikt-engine-darwin-arm64`. A platform with no binary present falls back to
Go-native analysis, which is a supported configuration — see `EnginePath`.
