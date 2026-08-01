package fileops

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
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

func TestCopyOverwritesExistingDestination(t *testing.T) {
	// The incremental-sync path: dst already exists and Copy replaces it with the
	// current source content, clamped.
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFile(t, src, "new content")
	dst := filepath.Join(dir, "dst")
	writeFile(t, dst, "stale content that is longer than the new one")

	if err := Copy(src, dst); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "new content" {
		t.Errorf("dst = %q, %v; want the new content with the old gone", got, err)
	}
	if m := mode(t, dst); m != fileMode {
		t.Errorf("dst mode = %o, want %o", m, fileMode)
	}
}

func TestCopyDirTreeSkipsDanglingInnerSymlink(t *testing.T) {
	// A dangling symlink inside the tree is genuinely nothing to copy; it is
	// skipped and the copy of the real files still succeeds.
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFile(t, filepath.Join(src, "real.txt"), "r")
	symlink(t, filepath.Join(dir, "nowhere"), filepath.Join(src, "dangling"))
	dst := filepath.Join(dir, "dst")

	if err := Copy(src, dst); err != nil {
		t.Fatalf("Copy failed on a tree with a dangling symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "real.txt")); err != nil {
		t.Errorf("real file not copied: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "dangling")); !os.IsNotExist(err) {
		t.Error("dangling symlink should not have been copied")
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

func TestCopySymlinkToFileCopiesTargetContent(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	writeFile(t, real, "content")
	src := filepath.Join(dir, "srclink")
	symlink(t, real, src)
	dst := filepath.Join(dir, "dst")

	if err := Copy(src, dst); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "content" {
		t.Errorf("dst = %q, %v; want the symlink target's content", got, err)
	}
	if m := mode(t, dst); m != fileMode {
		t.Errorf("dst mode = %o, want %o", m, fileMode)
	}
}

func TestCopySymlinkToDirectoryMergesTree(t *testing.T) {
	// The regression guard for the WalkDir bug: a symlink-to-directory src must
	// copy the target's contents, not silently succeed with nothing.
	dir := t.TempDir()
	realDir := filepath.Join(dir, "realdir")
	writeFile(t, filepath.Join(realDir, "a.txt"), "a")
	writeFile(t, filepath.Join(realDir, "sub", "b.txt"), "b")
	src := filepath.Join(dir, "dirlink")
	symlink(t, realDir, src)
	dst := filepath.Join(dir, "dst")

	if err := Copy(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"a.txt", "sub/b.txt"} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing %s: symlink-to-dir src copied nothing", rel)
		}
	}
	if m := mode(t, filepath.Join(dst, "a.txt")); m != fileMode {
		t.Errorf("copied file mode = %o, want %o", m, fileMode)
	}
}

func TestCopyDirTreeWithInnerSymlinkCopiesItsContent(t *testing.T) {
	// A symlink to a regular file inside the tree is dereferenced: its content
	// lands as a real file in dst, rather than being silently dropped.
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFile(t, filepath.Join(src, "plain.txt"), "plain")
	target := filepath.Join(dir, "outside")
	writeFile(t, target, "linked")
	symlink(t, target, filepath.Join(src, "inner")) // inner symlink to a file
	dst := filepath.Join(dir, "dst")

	if err := Copy(src, dst); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "inner")); err != nil || string(got) != "linked" {
		t.Errorf("dst/inner = %q, %v; want the symlink target content copied", got, err)
	}
}

func TestCopyDirTreeWithSymlinkCycleTerminates(t *testing.T) {
	// A symlink pointing back into an ancestor must not send copyTree into
	// infinite recursion; the copy completes and the real files land once.
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFile(t, filepath.Join(src, "real.txt"), "r")
	writeFile(t, filepath.Join(src, "sub", "deep.txt"), "d")
	symlink(t, src, filepath.Join(src, "sub", "loop_to_root")) // -> ancestor src
	symlink(t, "..", filepath.Join(src, "sub", "loop_to_parent"))
	dst := filepath.Join(dir, "dst")

	done := make(chan error, 1)
	go func() { done <- Copy(src, dst) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Copy returned an error on a cyclic tree: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Copy did not terminate on a symlink cycle (infinite recursion)")
	}
	for _, rel := range []string{"real.txt", "sub/deep.txt"} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel))); err != nil {
			t.Errorf("real file %s missing: %v", rel, err)
		}
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
