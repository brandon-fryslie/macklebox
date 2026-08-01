package fileops

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// The AlreadyLinked accept/reject shape table, made executable.
func TestAlreadyLinked(t *testing.T) {
	t.Run("accept: live symlink to existing mackup copy", func(t *testing.T) {
		dir := t.TempDir()
		mackup := filepath.Join(dir, "storage", "f")
		home := filepath.Join(dir, "home", "f")
		writeFile(t, mackup, "x")
		symlink(t, mackup, home)
		if !AlreadyLinked(home, mackup) {
			t.Error("want true for a live symlink resolving to the mackup copy")
		}
	})

	reject := map[string]func(t *testing.T, dir string) (home, mackup string){
		"real file at home": func(t *testing.T, dir string) (string, string) {
			home := filepath.Join(dir, "home", "f")
			mackup := filepath.Join(dir, "storage", "f")
			writeFile(t, home, "x")
			writeFile(t, mackup, "x")
			return home, mackup
		},
		"real directory at home": func(t *testing.T, dir string) (string, string) {
			home := filepath.Join(dir, "home", "f")
			mackup := filepath.Join(dir, "storage", "f")
			if err := os.MkdirAll(home, 0o755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, mackup, "x")
			return home, mackup
		},
		"home absent": func(t *testing.T, dir string) (string, string) {
			mackup := filepath.Join(dir, "storage", "f")
			writeFile(t, mackup, "x")
			return filepath.Join(dir, "home", "f"), mackup
		},
		"dangling symlink at home": func(t *testing.T, dir string) (string, string) {
			home := filepath.Join(dir, "home", "f")
			mackup := filepath.Join(dir, "storage", "f")
			writeFile(t, mackup, "x")
			symlink(t, filepath.Join(dir, "nowhere"), home)
			return home, mackup
		},
		"symlink to another target": func(t *testing.T, dir string) (string, string) {
			home := filepath.Join(dir, "home", "f")
			mackup := filepath.Join(dir, "storage", "f")
			other := filepath.Join(dir, "other")
			writeFile(t, mackup, "x")
			writeFile(t, other, "y")
			symlink(t, other, home)
			return home, mackup
		},
		"symlink to mackup but mackup absent": func(t *testing.T, dir string) (string, string) {
			home := filepath.Join(dir, "home", "f")
			mackup := filepath.Join(dir, "storage", "f") // never created
			symlink(t, mackup, home)
			return home, mackup
		},
	}
	for name, setup := range reject {
		t.Run("reject: "+name, func(t *testing.T) {
			home, mackup := setup(t, t.TempDir())
			if AlreadyLinked(home, mackup) {
				t.Errorf("want false for %q", name)
			}
		})
	}
}

func TestState(t *testing.T) {
	cases := map[string]struct {
		setup func(t *testing.T, dir string) (home, mackup string)
		want  LinkState
	}{
		"already-linked": {
			want: StateAlreadyLinked,
			setup: func(t *testing.T, dir string) (string, string) {
				home := filepath.Join(dir, "home", "f")
				mackup := filepath.Join(dir, "storage", "f")
				writeFile(t, mackup, "x")
				symlink(t, mackup, home)
				return home, mackup
			},
		},
		"real-file-present: real file": {
			want: StateRealFilePresent,
			setup: func(t *testing.T, dir string) (string, string) {
				home := filepath.Join(dir, "home", "f")
				writeFile(t, home, "x")
				return home, filepath.Join(dir, "storage", "f")
			},
		},
		"real-file-present: live foreign symlink": {
			want: StateRealFilePresent,
			setup: func(t *testing.T, dir string) (string, string) {
				home := filepath.Join(dir, "home", "f")
				other := filepath.Join(dir, "other")
				writeFile(t, other, "y")
				symlink(t, other, home)
				return home, filepath.Join(dir, "storage", "f")
			},
		},
		"broken-link": {
			want: StateBrokenLink,
			setup: func(t *testing.T, dir string) (string, string) {
				home := filepath.Join(dir, "home", "f")
				symlink(t, filepath.Join(dir, "nowhere"), home)
				return home, filepath.Join(dir, "storage", "f")
			},
		},
		"absent": {
			want: StateAbsent,
			setup: func(t *testing.T, dir string) (string, string) {
				return filepath.Join(dir, "home", "f"), filepath.Join(dir, "storage", "f")
			},
		},
		"mackup-only": {
			want: StateMackupOnly,
			setup: func(t *testing.T, dir string) (string, string) {
				mackup := filepath.Join(dir, "storage", "f")
				writeFile(t, mackup, "x")
				return filepath.Join(dir, "home", "f"), mackup
			},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			home, mackup := c.setup(t, t.TempDir())
			if got := State(home, mackup); got != c.want {
				t.Errorf("State = %v, want %v", got, c.want)
			}
		})
	}
}
