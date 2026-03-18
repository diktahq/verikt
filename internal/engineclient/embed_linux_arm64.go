//go:build linux && arm64

package engineclient

import _ "embed"

//go:embed bin/archway-engine-linux-arm64
var engineBinary []byte
