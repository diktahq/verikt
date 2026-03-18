// Package initabuse triggers init_abuse — init() with > 5 statements.
package initabuse

var cfg map[string]string

func init() {
	cfg = make(map[string]string)
	cfg["host"] = "localhost"
	cfg["port"] = "5432"
	cfg["user"] = "admin"
	cfg["pass"] = "secret"
	cfg["db"] = "mydb"
	cfg["ssl"] = "disable"
}
