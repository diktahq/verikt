// Package ginkgorunner exists so a production file in this fixture imports a
// path containing "ginkgo".
//
// The BDD signal is a substring match on the import path, and it used to be
// taken from pkg.Imports — production imports — and merged with the test-file
// signal. Without an import like this anywhere in the fixture, the test claiming
// to guard that behaviour could not tell the two implementations apart: it
// passed either way.
package ginkgorunner

// Run is a plain function. The package name is the whole point.
func Run() string {
	return "not a test framework"
}
