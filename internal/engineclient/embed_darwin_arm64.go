//go:build darwin && arm64

package engineclient

import _ "embed"

//go:embed bin/verikt-engine-darwin-arm64
var engineBinary []byte
