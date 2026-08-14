# detector-coverage

One package per anti-pattern detector, each written to trigger exactly the
detector it is named for.

Every detector in the engine must fire on this fixture. Ten of them previously
had no CI coverage at all: the Go tests that covered them were deleted with the
Go analysis path, and the replacement lived in `internal/engineclient/experiment`,
which every CI job filters out of `go list`.

The fixture is analysed by passing this directory as the project root, which is
how the engine reaches code under `testdata/` at all (INV-004).
