package guide

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrSymlinkInProject is returned when a project path verikt reads or writes,
// or one of its parent directories inside the project, is a symlink.
//
// Every file verikt guide touches is one it owns. Following a link let a
// checkout overwrite the file a planted hook-script link pointed to, and copy a
// linked AGENTS.md's target into the repository. os.Root stops links that
// escape the project; the Lstat walk in checkNoSymlink also stops links that
// stay inside it, which would otherwise modify some other project file.
var ErrSymlinkInProject = errors.New("refusing to follow a symlink")

// Project paths below are relative to the project root and use the host's
// separator. Every access goes through an os.Root opened on projectDir.

// withProjectRoot opens projectDir as an os.Root, checks that rel and each of
// its parents inside the root is not a symlink, and runs fn.
func withProjectRoot(projectDir, rel string, fn func(root *os.Root, rel string) error) error {
	rel = filepath.Clean(rel)
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s: path is outside the project", rel)
	}
	root, err := os.OpenRoot(projectDir)
	if err != nil {
		return fmt.Errorf("open project %s: %w", projectDir, err)
	}
	// Close errors on a read-only directory handle carry no information.
	defer func() { _ = root.Close() }()
	if err := checkNoSymlink(root, rel); err != nil {
		return err
	}
	return fn(root, rel)
}

// checkNoSymlink walks rel one component at a time and refuses the first one
// that is a symlink. A missing component ends the walk: nothing below it exists
// yet, and MkdirAll creates real directories.
func checkNoSymlink(root *os.Root, rel string) error {
	if rel == "." {
		return nil
	}
	cur := ""
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		info, err := root.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect %s: %w", filepath.ToSlash(cur), err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s is a symlink; verikt writes only regular files inside the project, so remove the link and re-run",
				ErrSymlinkInProject, filepath.ToSlash(cur))
		}
	}
	return nil
}

// readProjectFile reads rel inside projectDir. A missing file returns an error
// satisfying errors.Is(err, fs.ErrNotExist).
func readProjectFile(projectDir, rel string) ([]byte, error) {
	var data []byte
	err := withProjectRoot(projectDir, rel, func(root *os.Root, rel string) error {
		var err error
		data, err = root.ReadFile(rel)
		return err
	})
	return data, err
}

// removeProjectFile removes rel inside projectDir. A missing file is not an error.
func removeProjectFile(projectDir, rel string) error {
	return withProjectRoot(projectDir, rel, func(root *os.Root, rel string) error {
		if err := root.Remove(rel); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", filepath.ToSlash(rel), err)
		}
		return nil
	})
}

// WriteProjectFile atomically writes data to rel inside projectDir, creating
// parent directories. An existing file keeps its permissions; a new one gets
// 0o644. It refuses with ErrSymlinkInProject when rel or a parent inside the
// project is a symlink.
func WriteProjectFile(projectDir, rel string, data []byte) error {
	return writeProjectFileMode(projectDir, rel, data, 0o644, true)
}

// writeProjectScript atomically writes an executable (0o755) script.
func writeProjectScript(projectDir, rel string, data []byte) error {
	return writeProjectFileMode(projectDir, rel, data, 0o755, false)
}

func writeProjectFileMode(projectDir, rel string, data []byte, perm os.FileMode, keepMode bool) error {
	return withProjectRoot(projectDir, rel, func(root *os.Root, rel string) error {
		if dir := filepath.Dir(rel); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create %s: %w", filepath.ToSlash(dir), err)
			}
		}
		if keepMode {
			if info, err := root.Lstat(rel); err == nil {
				perm = info.Mode().Perm()
			}
		}
		return writeAtomicInRoot(root, rel, data, perm)
	})
}

// writeAtomicInRoot writes data to a temp file next to rel and renames it into
// place, so an interrupted write never leaves a truncated file.
func writeAtomicInRoot(root *os.Root, rel string, data []byte, perm os.FileMode) error {
	tmp := rel + ".tmp-" + rand.Text()
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", filepath.ToSlash(rel), err)
	}
	// Removing the temp file fails harmlessly once the rename has moved it.
	defer func() { _ = root.Remove(tmp) }()

	if _, err := f.Write(data); err != nil {
		_ = f.Close() // the write error is the one worth reporting
		return fmt.Errorf("write %s: %w", filepath.ToSlash(rel), err)
	}
	// OpenFile's mode is filtered by the umask; set it explicitly.
	if err := f.Chmod(perm); err != nil {
		_ = f.Close() // the chmod error is the one worth reporting
		return fmt.Errorf("chmod temp for %s: %w", filepath.ToSlash(rel), err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write %s: %w", filepath.ToSlash(rel), err)
	}
	if err := root.Rename(tmp, rel); err != nil {
		return fmt.Errorf("replace %s: %w", filepath.ToSlash(rel), err)
	}
	return nil
}

// listProjectDir returns the names of the entries in directory rel inside
// projectDir, or none when it does not exist.
func listProjectDir(projectDir, rel string) ([]string, error) {
	var names []string
	err := withProjectRoot(projectDir, rel, func(root *os.Root, rel string) error {
		dir, err := root.Open(rel)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("open %s: %w", filepath.ToSlash(rel), err)
		}
		// Close errors on a directory opened for reading carry no information.
		defer func() { _ = dir.Close() }()
		names, err = dir.Readdirnames(-1)
		if err != nil {
			return fmt.Errorf("list %s: %w", filepath.ToSlash(rel), err)
		}
		return nil
	})
	return names, err
}
