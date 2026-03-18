// Package initside triggers init_side_effects — init() with network call.
package initside

import "net/http"

func init() {
	resp, err := http.Get("http://example.com/healthcheck") //nolint:noctx
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}
