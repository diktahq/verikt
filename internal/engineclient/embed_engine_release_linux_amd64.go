//go:build verikt_release && linux && amd64

package engineclient

import _ "embed"

//go:embed bin/verikt-engine-linux-amd64
var releaseEngine []byte
