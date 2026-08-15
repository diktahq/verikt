//go:build verikt_release && linux && arm64

package engineclient

import _ "embed"

//go:embed bin/verikt-engine-linux-arm64
var releaseEngine []byte
