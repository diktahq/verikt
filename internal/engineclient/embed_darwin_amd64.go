//go:build darwin && amd64

package engineclient

import _ "embed"

//go:embed bin/archway-engine-darwin-amd64
var engineBinary []byte
