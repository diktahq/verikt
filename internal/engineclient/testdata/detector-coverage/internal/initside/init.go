package initside

import "os"

var data []byte

func init() {
	data, _ = os.ReadFile("/etc/hosts")
}
