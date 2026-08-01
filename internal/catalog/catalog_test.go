package catalog

import (
	"errors"
	"io/fs"
	"testing"
)

func TestFSRootIsEmpty(t *testing.T) {
	entries, err := fs.ReadDir(FS(), ".")
	if err != nil {
		t.Fatalf("ReadDir(.) error = %v, want nil", err)
	}
	if len(entries) != 0 {
		t.Errorf("ReadDir(.) = %v entries, want 0", len(entries))
	}
}

func TestFSNonexistentPathErrors(t *testing.T) {
	// A path that does not exist must error, not report an empty directory.
	if _, err := fs.ReadDir(FS(), "nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadDir(nope) error = %v, want fs.ErrNotExist", err)
	}
	if _, err := FS().Open("nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open(nope) error = %v, want fs.ErrNotExist", err)
	}
}
