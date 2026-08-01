package drift

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func xmlPlist(name string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>Name</key><string>` + name + `</string></dict></plist>`)
}

func TestEitherSymlinkDiffersWithNoDetail(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	link := filepath.Join(dir, "link")
	writeFile(t, real, []byte("x"))
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	// source is a symlink; dest a real file.
	if got := Compare(link, real); got.Identical || got.Detail != "" {
		t.Errorf("symlink source = %+v, want {false, \"\"}", got)
	}
	// dest is a symlink; source a real file.
	if got := Compare(real, link); got.Identical || got.Detail != "" {
		t.Errorf("symlink dest = %+v, want {false, \"\"}", got)
	}
}

func TestTypeMismatch(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	folder := filepath.Join(dir, "folder")
	writeFile(t, file, []byte("x"))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Compare(folder, file); got.Identical || got.Detail != "type mismatch: folder vs file" {
		t.Errorf("folder vs file = %+v", got)
	}
	if got := Compare(file, folder); got.Identical || got.Detail != "type mismatch: file vs folder" {
		t.Errorf("file vs folder = %+v", got)
	}
}

func TestPlistComparison(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.plist")
	b := filepath.Join(dir, "b.plist")

	writeFile(t, a, xmlPlist("Vim"))
	writeFile(t, b, xmlPlist("Vim"))
	if got := Compare(a, b); !got.Identical {
		t.Errorf("equal plists: %+v, want identical", got)
	}

	writeFile(t, b, xmlPlist("Emacs"))
	got := Compare(a, b)
	if got.Identical || !strings.Contains(got.Detail, "@@") {
		t.Errorf("differing plists: %+v, want a unified diff", got)
	}
	// The diff is of the parsed structure (JSON), not the raw XML.
	if !strings.Contains(got.Detail, "Vim") || !strings.Contains(got.Detail, "Emacs") {
		t.Errorf("plist diff does not show the changed value: %q", got.Detail)
	}
}

func TestTextComparison(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")

	writeFile(t, a, []byte("line one\nline two\n"))
	writeFile(t, b, []byte("line one\nline two\n"))
	if got := Compare(a, b); !got.Identical {
		t.Errorf("equal text: %+v, want identical", got)
	}

	writeFile(t, b, []byte("line one\nline CHANGED\n"))
	got := Compare(a, b)
	if got.Identical || !strings.Contains(got.Detail, "-line two") || !strings.Contains(got.Detail, "+line CHANGED") {
		t.Errorf("differing text: %+v, want a unified line diff", got)
	}
}

func TestBinaryComparison(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")

	writeFile(t, a, []byte{0xff, 0xfe, 0x00, 0x01})
	writeFile(t, b, []byte{0xff, 0xfe, 0x00, 0x01})
	if got := Compare(a, b); !got.Identical {
		t.Errorf("equal binary: %+v, want identical", got)
	}

	writeFile(t, b, []byte{0xff, 0xfe, 0x00, 0x02})
	if got := Compare(a, b); got.Identical || got.Detail != "binary contents differ" {
		t.Errorf("differing binary: %+v, want \"binary contents differ\"", got)
	}
}

func TestDirectoryComparison(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	// identical trees
	writeFile(t, filepath.Join(src, "same.txt"), []byte("s"))
	writeFile(t, filepath.Join(dst, "same.txt"), []byte("s"))
	if got := Compare(src, dst); !got.Identical {
		t.Errorf("identical dirs: %+v, want identical", got)
	}

	// introduce a change, a source-only file, and a target-only file
	writeFile(t, filepath.Join(src, "same.txt"), []byte("changed"))
	writeFile(t, filepath.Join(src, "only_src.txt"), []byte("x"))
	writeFile(t, filepath.Join(dst, "only_dst.txt"), []byte("y"))

	got := Compare(src, dst)
	want := "changed: same.txt\nonly in source: only_src.txt\nonly in target: only_dst.txt\n"
	if got.Identical || got.Detail != want {
		t.Errorf("differing dirs detail =\n%q\nwant\n%q", got.Detail, want)
	}
}

func TestDirectoryComparisonNestedAndEmptySubdir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	// A nested file present on both, identical.
	writeFile(t, filepath.Join(src, "a", "b", "c.txt"), []byte("x"))
	writeFile(t, filepath.Join(dst, "a", "b", "c.txt"), []byte("x"))
	// An empty subdirectory present only on the source side — an entry the
	// target lacks, so the trees are not identical.
	if err := os.MkdirAll(filepath.Join(src, "emptysub"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := Compare(src, dst)
	if got.Identical {
		t.Error("dirs differing only by an empty subdirectory reported identical")
	}
	if !strings.Contains(got.Detail, "only in source: emptysub") {
		t.Errorf("detail = %q, want it to flag the source-only empty subdir", got.Detail)
	}
}

func TestDirectoryComparisonUnreadableTreeIsNotIdentical(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	writeFile(t, filepath.Join(src, "sub", "f"), []byte("x"))
	writeFile(t, filepath.Join(dst, "sub", "f"), []byte("x"))

	// Make a source subdirectory unreadable so the walk cannot see its contents.
	unreadable := filepath.Join(src, "sub")
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(unreadable, 0o755)

	if got := Compare(src, dst); got.Identical {
		t.Error("a tree that could not be fully read was reported identical")
	}
}

func TestDirectoryComparisonUnreadableFileIsCannotRead(t *testing.T) {
	// The directory is traversable and the entry is listed, but a regular file
	// present on both sides cannot be read — that is "cannot read", distinct
	// from a verified "changed", and never a false claim of identity.
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	writeFile(t, filepath.Join(src, "f"), []byte("x"))
	writeFile(t, filepath.Join(dst, "f"), []byte("x"))
	if err := os.Chmod(filepath.Join(src, "f"), 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(src, "f"), 0o644)

	got := Compare(src, dst)
	if got.Identical {
		t.Error("a tree with an unreadable file reported identical")
	}
	if !strings.Contains(got.Detail, "cannot read: f") {
		t.Errorf("detail = %q, want it to flag the unreadable file as \"cannot read\"", got.Detail)
	}
	if strings.Contains(got.Detail, "changed: f") {
		t.Errorf("detail = %q, must not label an unreadable file as a verified change", got.Detail)
	}
}

func TestParsePlistRejectsOpenStepAndNonPlist(t *testing.T) {
	// Only XML/binary plists count, matching plistlib; an OpenStep text plist
	// and plain text are not plists.
	if _, ok := parsePlist([]byte(`{ Name = "Vim"; }`)); ok {
		t.Error("OpenStep-format text parsed as a plist; want rejected")
	}
	if _, ok := parsePlist([]byte("just some config text\n")); ok {
		t.Error("plain text parsed as a plist")
	}
	if _, ok := parsePlist(xmlPlist("Vim")); !ok {
		t.Error("XML plist not recognized as a plist")
	}
}
