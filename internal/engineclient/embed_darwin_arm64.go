//go:build darwin && arm64

package engineclient

import _ "embed"

//go:embed bin/archway-engine-darwin-arm64
var engineBinary []byte
