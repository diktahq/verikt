//go:build !unix

package engineclient

import "os"

// privateDirProblem has no POSIX owner or mode bits to check on this platform;
// the symlink and directory checks in checkCacheDir still apply.
func privateDirProblem(string, os.FileInfo, int) error { return nil }
