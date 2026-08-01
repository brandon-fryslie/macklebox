package fileops

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// fileMode / dirMode are the clamp targets of appspec/06: owner-only, no group
// or world bits. A round-tripped file can come back less permissive than it
// started — that is the intended post-condition, not a bug.
const (
	fileMode = 0o600
	dirMode  = 0o700
)

// Copy copies src to dst with the semantics of appspec/06 "copy(src, dst)": the
// parent of dst is created, a regular file is copied as a file and a directory
// recursively (merging into any existing dst — same-named files overwritten,
// dst-only files kept), and the result is permission-clamped. Anything that is
// neither a regular file nor a directory is an error.
func Copy(src, dst string) error {
	info, err := os.Stat(src) // follow symlinks: classify by what src resolves to
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), dirMode); err != nil {
		return err
	}
	switch {
	case info.Mode().IsRegular():
		if err := copyFile(src, dst); err != nil {
			return err
		}
	case info.IsDir():
		if err := copyTree(src, dst); err != nil {
			return err
		}
	default:
		return fmt.Errorf("cannot copy %s: not a regular file or directory", src)
	}
	return Clamp(dst)
}

// copyTree walks src and merges it into dst: directories are created, regular
// files are copied, and anything else (symlinks, devices) inside the tree is
// skipped rather than failing the whole copy.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, dirMode)
		case d.Type().IsRegular():
			return copyFile(p, target)
		default:
			return nil // skip symlinks and special files within the tree
		}
	})
}

// copyFile copies one regular file's contents, overwriting dst if present.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Delete removes path with the semantics of appspec/06 "delete(path)":
// attributes are stripped first, then a regular file or symlink is removed as
// itself (a symlink as the link, not its target) and a directory recursively.
func Delete(path string) error {
	removeAttributes(path)
	return os.RemoveAll(path)
}

// Link creates linkPath as a symbolic link to the absolute path target
// (appspec/06 "link"): target's permissions are clamped recursively first, the
// parent of linkPath is created, then the symlink is made.
func Link(target, linkPath string) error {
	if err := Clamp(target); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(linkPath), dirMode); err != nil {
		return err
	}
	return os.Symlink(target, linkPath)
}

// Clamp sets path's mode recursively (appspec/06 "Permissions"): regular files
// to 0600, directories to 0700. Attributes are stripped first so an immutable
// or ACL-protected file does not block the chmod. Symlinks encountered while
// walking — broken or live — are skipped: chmod follows links, so touching them
// would either fail (dangling) or alter the target through the link.
func Clamp(path string) error {
	removeAttributes(path)
	return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil // skip symlinks rather than chmod-ing through them
		}
		if d.IsDir() {
			return os.Chmod(p, dirMode)
		}
		return os.Chmod(p, fileMode)
	})
}
