//go:build unix

package engineclient

import (
	"fmt"
	"os"
	"syscall"
)

// privateDirProblem reports why a cache directory is not private to euid: a
// directory another user owns, or one a group or everyone can write to, lets
// someone else replace the engine between its hash check and exec.
func privateDirProblem(path string, info os.FileInfo, euid int) error {
	if perm := info.Mode().Perm(); perm&0o022 != 0 {
		return fmt.Errorf("engine cache dir %s is group- or world-writable (mode %04o); refusing to use it", path, perm)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("engine cache dir %s: cannot determine its owner", path)
	}
	if int64(st.Uid) != int64(euid) {
		return fmt.Errorf("engine cache dir %s is owned by uid %d, not the current user (uid %d); refusing to use it", path, st.Uid, euid)
	}
	return nil
}
