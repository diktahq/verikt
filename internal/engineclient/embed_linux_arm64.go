//go:build linux && arm64

package engineclient

import _ "embed"

//go:embed bin/verikt-engine-linux-arm64
var engineBinary []byte
