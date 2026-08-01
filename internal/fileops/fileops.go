package fileops

import (
	"errors"
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
//
// A single-file copy is atomic (see copyFile). A directory-tree copy is not: it
// merges file by file and cannot be swapped atomically without discarding the
// dst-only files the merge must preserve, so a failure partway leaves a
// partially merged tree. That is recoverable by re-running, which is the sync
// engine's recovery model (appspec/00 promise 3, appspec/01 §5 per-file
// independence) — no rollback, re-run converges.
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

// copyTree recursively merges the directory src into dst. It classifies each
// entry by os.Stat, which follows symlinks, so a symlinked file has its target
// content copied and a symlinked subdirectory is descended into — the reference's
// dereferencing behavior — rather than dropped. os.ReadDir likewise follows src
// itself when src is a symlink to a directory. An entry whose target does not
// exist — a dangling symlink or one that vanished between readdir and stat — is
// genuinely nothing to copy and is skipped, consistent with the clamp's
// broken-symlink rule; a special file (device, socket) has no content and is
// skipped too. Any other stat error — a permission or I/O failure on a file that
// really is there — is returned, never swallowed, so a backup cannot silently
// omit real content. Existing dst-only files are left in place (merge).
//
// Because it follows symlinks, a symlink pointing back at an ancestor directory
// would recurse forever; ancestors tracks the canonical (symlink-resolved) path
// of every directory on the current descent, and a back-edge into one is skipped
// to break the cycle. The cyclic target is that ancestor, already copied, so
// nothing is lost.
func copyTree(src, dst string) error {
	real, err := filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}
	return copyTreeInto(src, dst, map[string]bool{real: true})
}

func copyTreeInto(src, dst string, ancestors map[string]bool) error {
	if err := os.MkdirAll(dst, dirMode); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		s := filepath.Join(src, entry.Name())
		d := filepath.Join(dst, entry.Name())
		info, err := os.Stat(s) // follow symlinks; classify by the target
		if errors.Is(err, fs.ErrNotExist) {
			continue // dangling symlink or vanished entry: nothing to copy
		}
		if err != nil {
			return err // a real failure (permission, I/O): surface it
		}
		switch {
		case info.IsDir():
			real, err := filepath.EvalSymlinks(s)
			if err != nil {
				return err
			}
			if ancestors[real] {
				continue // a symlink back into an ancestor: skip to break the cycle
			}
			ancestors[real] = true
			err = copyTreeInto(s, d, ancestors)
			delete(ancestors, real) // backtrack: the set is the current path only
			if err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := copyFile(s, d); err != nil {
				return err
			}
		}
		// A device/socket/etc. has no file content to copy; skipped.
	}
	return nil
}

// copyFile copies one regular file's contents, overwriting dst if present. The
// write goes to a temporary file in dst's own directory and is renamed into
// place only on success, so a failure partway through — a read error, a full
// disk — leaves any existing dst untouched rather than truncated. That keeps the
// overwrite atomic: a failing copy makes no filesystem change (appspec/07). The
// temp lives in dst's directory so the rename stays within one filesystem.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".mackup-copy-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
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
