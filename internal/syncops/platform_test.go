package syncops

import "testing"

func TestSyncAllowedOnPlatform(t *testing.T) {
	home := "/home/bob"
	underLibrary := "/home/bob/Library/Preferences/app"
	elsewhere := "/home/bob/.vimrc"

	// macOS: no restriction — everything, including ~/Library, is allowed.
	if !syncAllowedOnPlatform("darwin", home, underLibrary) {
		t.Error("darwin should allow a ~/Library path")
	}
	// Linux: a ~/Library path is skipped; anything else is allowed.
	if syncAllowedOnPlatform("linux", home, underLibrary) {
		t.Error("linux should skip a ~/Library path")
	}
	if !syncAllowedOnPlatform("linux", home, elsewhere) {
		t.Error("linux should allow a non-Library path")
	}
	// A sibling that merely shares the "Library" name prefix is not under it.
	if !syncAllowedOnPlatform("linux", home, "/home/bob/LibraryNotes") {
		t.Error("linux should allow a path that only shares the Library prefix")
	}
}
