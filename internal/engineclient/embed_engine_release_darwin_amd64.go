//go:build verikt_release && darwin && amd64

package engineclient

import _ "embed"

//go:embed bin/verikt-engine-darwin-amd64
var releaseEngine []byte
