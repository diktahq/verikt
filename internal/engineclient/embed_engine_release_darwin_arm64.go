//go:build verikt_release && darwin && arm64

package engineclient

import _ "embed"

//go:embed bin/verikt-engine-darwin-arm64
var releaseEngine []byte
