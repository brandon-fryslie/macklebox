package fileops

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestCopyRegularFileClampsTo0600AndCreatesParent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFile(t, src, "hello")
	dst := filepath.Join(dir, "new", "deep", "dst") // parent does not exist

	if err := Copy(src, dst); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "hello" {
		t.Fatalf("dst content = %q, %v; want hello", got, err)
	}
	if m := mode(t, dst); m != fileMode {
		t.Errorf("dst mode = %o, want %o", m, fileMode)
	}
}

func TestCopyDirectoryTreeClampsRecursivelyAndMerges(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFile(t, filepath.Join(src, "a.txt"), "a")
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "b")
	dst := filepath.Join(dir, "dst")
	writeFile(t, filepath.Join(dst, "keep.txt"), "keep") // pre-existing dst-only file

	if err := Copy(src, dst); err != nil {
		t.Fatal(err)
	}
	// Merge: src files land, dst-only file is kept.
	for _, rel := range []string{"a.txt", "sub/b.txt", "keep.txt"} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing %s after merge copy: %v", rel, err)
		}
	}
	// Recursive clamp: files 0600, directories 0700.
	if m := mode(t, filepath.Join(dst, "a.txt")); m != fileMode {
		t.Errorf("file mode = %o, want %o", m, fileMode)
	}
	if m := mode(t, filepath.Join(dst, "sub")); m != dirMode {
		t.Errorf("dir mode = %o, want %o", m, dirMode)
	}
}

func TestCopyLeavesDestinationUntouchedOnFailure(t *testing.T) {
	// The atomic overwrite: when the copy cannot complete, an existing dst keeps
	// its original content rather than being truncated (appspec/07: a failing
	// operation makes no filesystem change).
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFile(t, src, "new content")
	dstDir := filepath.Join(dir, "dstdir")
	dst := filepath.Join(dstDir, "dst")
	writeFile(t, dst, "original")

	// A read-only destination directory makes the temp write (and thus the copy)
	// fail, without failing the earlier parent MkdirAll on the existing dir.
	if err := os.Chmod(dstDir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dstDir, 0o700) // restore so t.TempDir cleanup can remove it

	if err := Copy(src, dst); err == nil {
		t.Error("Copy into a read-only directory should return an error")
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "original" {
		t.Errorf("dst = %q, %v; want the original content untouched", got, err)
	}
}

func TestCopyNonFileNonDirIsError(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("cannot create fifo on this platform: %v", err)
	}
	if err := Copy(fifo, filepath.Join(dir, "dst")); err == nil {
		t.Error("copying a fifo should be an error (neither regular file nor directory)")
	}
}

func TestDeleteRemovesSymlinkNotItsTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	writeFile(t, target, "x")
	symlink(t, target, link)

	if err := Delete(link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("symlink should be gone after Delete")
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("Delete removed the symlink's target; it must remove only the link")
	}
}

func TestDeleteRemovesDirectoryTree(t *testing.T) {
	dir := t.TempDir()
	tree := filepath.Join(dir, "tree")
	writeFile(t, filepath.Join(tree, "sub", "f"), "x")
	if err := Delete(tree); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Error("directory tree should be gone after Delete")
	}
}

func TestLinkClampsTargetCreatesParentAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "sub", "dir", "link") // parent does not exist

	if err := Link(target, link); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(link)
	if err != nil || got != target {
		t.Fatalf("Readlink = %q, %v; want %q", got, err, target)
	}
	if m := mode(t, target); m != fileMode {
		t.Errorf("target mode = %o, want %o (clamped before linking)", m, fileMode)
	}
}

func TestClampSkipsBrokenSymlinksWithoutFailing(t *testing.T) {
	dir := t.TempDir()
	tree := filepath.Join(dir, "tree")
	writeFile(t, filepath.Join(tree, "real"), "x")
	symlink(t, filepath.Join(dir, "nowhere"), filepath.Join(tree, "dangling"))

	if err := Clamp(tree); err != nil {
		t.Fatalf("Clamp failed on a tree with a broken symlink: %v", err)
	}
	if m := mode(t, filepath.Join(tree, "real")); m != fileMode {
		t.Errorf("real file mode = %o, want %o", m, fileMode)
	}
	if m := mode(t, tree); m != dirMode {
		t.Errorf("tree dir mode = %o, want %o", m, dirMode)
	}
}
