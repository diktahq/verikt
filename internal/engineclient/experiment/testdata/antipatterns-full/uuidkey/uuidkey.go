// Package uuidkey triggers uuid_v4_as_key.
package uuidkey

// uuid is a local stub that mirrors the github.com/google/uuid API
// so the AST detector fires without requiring the external dependency.
var uuid uuidPkg

type uuidPkg struct{}

func (uuidPkg) New() [16]byte     { return [16]byte{} }
func (uuidPkg) NewString() string { return "" }

// Entity carries a UUID primary key.
type Entity struct {
	ID   [16]byte
	Name string
}

// CreateEntity assigns a UUIDv4 primary key — should use UUIDv7, because a
// random key fragments the B-tree index it is stored in.
func CreateEntity(name string) *Entity {
	return &Entity{ID: uuid.New(), Name: name}
}

// CreateRecord assigns a UUIDv4 string key — same problem.
func CreateRecord() string {
	recordID := uuid.NewString()
	return recordID
}

// ObjectPath composes a storage path. The UUID is a file name here, not a key,
// so no index exists to fragment and the detector must stay silent — every
// `uuid.New()` used to be reported, which caught exactly this shape on a real
// repository (INV-005).
func ObjectPath(dir, ext string) string {
	return dir + "/" + uuid.NewString() + ext
}
