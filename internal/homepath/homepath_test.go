package homepath

import (
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

func TestXDGBaseDefaultsToDotConfig(t *testing.T) {
	if got := XDGBase("/home/bob", ""); got != filepath.Join("/home/bob", ".config") {
		t.Errorf("XDGBase unset = %q, want ~/.config", got)
	}
	if got := XDGBase("/home/bob", "/somewhere/xdg"); got != "/somewhere/xdg" {
		t.Errorf("XDGBase set = %q, want the set value verbatim", got)
	}
}
