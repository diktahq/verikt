// Package uuidkey triggers uuid_v4_as_key.
package uuidkey

// uuid is a local stub that mirrors the github.com/google/uuid API
// so the AST detector fires without requiring the external dependency.
var uuid uuidPkg

type uuidPkg struct{}

func (uuidPkg) New() [16]byte    { return [16]byte{} }
func (uuidPkg) NewString() string { return "" }

// CreateEntity creates a new entity with a UUIDv4 key — should use UUIDv7.
func CreateEntity() [16]byte {
	return uuid.New()
}

// CreateRecord creates a record with a UUIDv4 string key — should use UUIDv7.
func CreateRecord() string {
	return uuid.NewString()
}
