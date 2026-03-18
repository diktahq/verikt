//go:build !((darwin && amd64) || (darwin && arm64) || (linux && amd64) || (linux && arm64))

package engineclient

// engineBinary is nil on unsupported platforms; the engine client will be unavailable
// and checks will fall back to Go-native analysis.
var engineBinary []byte
