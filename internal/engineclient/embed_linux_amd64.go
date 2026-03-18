//go:build linux && amd64

package engineclient

import _ "embed"

//go:embed bin/archway-engine-linux-amd64
var engineBinary []byte
