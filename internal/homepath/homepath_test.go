package homepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWithinHome(t *testing.T) {
	home := filepath.Clean("/home/bob")
	cases := []struct {
		p    string
		want bool
	}{
		{"/home/bob", true},                // home itself is within home
		{"/home/bob/.config", true},        // a descendant
		{"/home/bob/.config/mackup", true}, // a deeper descendant
		{"/home/bob/..", false},            // the parent
		{"/home", false},                   // an ancestor
		{"/home/bobby", false},             // a sibling with a shared string prefix
		{"/home/bob-other/x", false},       // another shared-prefix sibling
		{"/tmp/elsewhere", false},          // wholly outside
	}
	for _, c := range cases {
		if got := WithinHome(home, filepath.Clean(c.p)); got != c.want {
			t.Errorf("WithinHome(%q, %q) = %v, want %v", home, c.p, got, c.want)
		}
	}
}

func TestIsRegularFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "regular")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsRegularFile(file) {
		t.Error("IsRegularFile(regular file) = false, want true")
	}
	if IsRegularFile(dir) {
		t.Error("IsRegularFile(directory) = true, want false")
	}
	if IsRegularFile(filepath.Join(dir, "does-not-exist")) {
		t.Error("IsRegularFile(nonexistent) = true, want false")
	}
}

func TestXDGBaseDefaultsToDotConfig(t *testing.T) {
	if got := XDGBase("/home/bob", ""); got != filepath.Join("/home/bob", ".config") {
		t.Errorf("XDGBase unset = %q, want ~/.config", got)
	}
	if got := XDGBase("/home/bob", "/somewhere/xdg"); got != "/somewhere/xdg" {
		t.Errorf("XDGBase set = %q, want the set value verbatim", got)
	}
}
