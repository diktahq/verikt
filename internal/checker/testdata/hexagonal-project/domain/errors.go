package domain

import "errors"

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

// GlobalCache is a mutable global — should trigger global_mutable_state.
//
// The write below is what makes it one. Declared and never written, this is a
// lookup table, and Go has no const map to express that instead — so reporting
// the declaration alone named a construct the language requires (INV-005). The
// fixture asserted the detector fired while demonstrating the case where it
// should not, which is what a fixture written from imagination does.
var GlobalCache = map[string]interface{}{}

func cacheResult(key string, value interface{}) {
	GlobalCache[key] = value
}
