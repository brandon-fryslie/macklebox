package appdb

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// The tests drive Assemble through real user directories under scratch homes
// and an in-memory built-in tier — the same three-source shape a process sees,
// minus the process. [LAW:behavior-not-structure]

// def renders a definition file. Empty section slices are omitted, so a
// definition with neither file section can be expressed.
func def(name string, configFiles, xdgFiles []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[application]\nname = %s\n", name)
	if len(configFiles) > 0 {
		b.WriteString("[configuration_files]\n")
		for _, p := range configFiles {
			b.WriteString(p + "\n")
		}
	}
	if len(xdgFiles) > 0 {
		b.WriteString("[xdg_configuration_files]\n")
		for _, p := range xdgFiles {
			b.WriteString(p + "\n")
		}
	}
	return b.String()
}

// writeDef writes one *.cfg into a user directory under home.
func writeDef(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func legacyDir(home string) string { return filepath.Join(home, ".mackup") }
func xdgAppsDir(home string) string {
	return filepath.Join(home, ".config", "mackup", "applications")
}

// builtin is the in-memory built-in tier for tests: filename → definition text.
func builtin(defs map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, content := range defs {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

func TestUnionOfConfigAndHomeRelativizedXDGFiles(t *testing.T) {
	// appspec/05 example: [configuration_files] verbatim, [xdg_configuration_files]
	// joined under the default ~/.config base and rendered home-relative, unioned
	// into one sorted case-exact set.
	home := t.TempDir()
	writeDef(t, legacyDir(home), "git.cfg", def("Git", []string{".gitconfig"}, []string{"git/config", "git/ignore"}))

	db := Assemble(home, "", builtin(nil))
	app, ok := db.Lookup("git")
	if !ok {
		t.Fatal("git not found")
	}
	if app.Name() != "Git" {
		t.Errorf("Name = %q, want Git", app.Name())
	}
	want := []string{".config/git/config", ".config/git/ignore", ".gitconfig"}
	if !reflect.DeepEqual(app.Files(), want) {
		t.Errorf("Files = %v, want %v", app.Files(), want)
	}
}

func TestPathsAreCaseExact(t *testing.T) {
	// The case-preserving half of the case-policy pair: definition paths are not
	// lowercased, unlike config application-list keys (appspec/05).
	home := t.TempDir()
	writeDef(t, legacyDir(home), "karabiner.cfg", def("Karabiner", []string{".config/Karabiner"}, nil))

	app, _ := Assemble(home, "", builtin(nil)).Lookup("karabiner")
	if got := app.Files(); len(got) != 1 || got[0] != ".config/Karabiner" {
		t.Errorf("Files = %v, want exact-case [.config/Karabiner]", got)
	}
}

func TestKeyIsFilenameNotDisplayName(t *testing.T) {
	// appspec/05: the key is the basename without .cfg; the display name is used
	// nowhere for matching.
	home := t.TempDir()
	writeDef(t, legacyDir(home), "myapp.cfg", def("Some Fancy Name", []string{".myapprc"}, nil))

	db := Assemble(home, "", builtin(nil))
	if _, ok := db.Lookup("myapp"); !ok {
		t.Error("Lookup(myapp) missing — key should be the filename basename")
	}
	if _, ok := db.Lookup("some fancy name"); ok {
		t.Error("Lookup matched the display name — names must not key the database")
	}
}

func TestPrecedenceLegacyBeatsXDGBeatsBuiltin(t *testing.T) {
	// appspec/05 three-tier precedence, decided by filename.
	home := t.TempDir()
	// vim: present in all three; legacy must win.
	writeDef(t, legacyDir(home), "vim.cfg", def("Legacy Vim", []string{".vimrc"}, nil))
	writeDef(t, xdgAppsDir(home), "vim.cfg", def("XDG Vim", []string{".vim-xdg"}, nil))
	// tmux: present in XDG and built-in; XDG must win.
	writeDef(t, xdgAppsDir(home), "tmux.cfg", def("XDG Tmux", []string{".tmux-xdg"}, nil))
	b := builtin(map[string]string{
		"vim.cfg":  def("Builtin Vim", []string{".vim-builtin"}, nil),
		"tmux.cfg": def("Builtin Tmux", []string{".tmux-builtin"}, nil),
		"bash.cfg": def("Builtin Bash", []string{".bashrc"}, nil),
	})

	db := Assemble(home, "", b)
	assertName := func(key, want string) {
		app, ok := db.Lookup(key)
		if !ok {
			t.Fatalf("%s not found", key)
		}
		if app.Name() != want {
			t.Errorf("%s Name = %q, want %q", key, app.Name(), want)
		}
	}
	assertName("vim", "Legacy Vim")    // tier 1 over 2 and 3
	assertName("tmux", "XDG Tmux")     // tier 2 over 3
	assertName("bash", "Builtin Bash") // tier 3 alone
}

func TestUserDefinitionFullyShadowsBuiltin(t *testing.T) {
	// appspec/05 observed effect: dropping ~/.mackup/vim.cfg replaces the
	// built-in entirely — the built-in file is not read at all for that key.
	home := t.TempDir()
	writeDef(t, legacyDir(home), "vim.cfg", def("Legacy Vim", []string{".vimrc"}, nil))
	b := builtin(map[string]string{"vim.cfg": def("Builtin Vim", []string{".vim/builtin", ".vim/extra"}, nil)})

	app, _ := Assemble(home, "", b).Lookup("vim")
	if got := app.Files(); len(got) != 1 || got[0] != ".vimrc" {
		t.Errorf("Files = %v, want only the user file [.vimrc] — the built-in must not leak", got)
	}
}

func TestKeysAreSortedAndCounted(t *testing.T) {
	home := t.TempDir()
	writeDef(t, legacyDir(home), "zebra.cfg", def("Zebra", nil, nil))
	b := builtin(map[string]string{
		"alpha.cfg": def("Alpha", nil, nil),
		"mike.cfg":  def("Mike", nil, nil),
	})
	got := Assemble(home, "", b).Keys()
	want := []string{"alpha", "mike", "zebra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Keys = %v, want %v (sorted ascending)", got, want)
	}
}

func TestDefinitionWithNeitherSectionHasEmptyFileSet(t *testing.T) {
	// appspec/05: a definition with neither section still appears, with an empty
	// file set.
	home := t.TempDir()
	writeDef(t, legacyDir(home), "empty.cfg", def("Empty App", nil, nil))
	app, ok := Assemble(home, "", builtin(nil)).Lookup("empty")
	if !ok {
		t.Fatal("empty not found — a section-less definition must still appear")
	}
	if len(app.Files()) != 0 {
		t.Errorf("Files = %v, want empty", app.Files())
	}
}

func TestNonCfgFilesAndSubdirsIgnored(t *testing.T) {
	home := t.TempDir()
	writeDef(t, legacyDir(home), "real.cfg", def("Real", nil, nil))
	writeDef(t, legacyDir(home), "notes.txt", "ignore me")
	if err := os.MkdirAll(filepath.Join(legacyDir(home), "sub.cfg"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := Assemble(home, "", builtin(nil)).Keys()
	if !reflect.DeepEqual(got, []string{"real"}) {
		t.Errorf("Keys = %v, want only [real]", got)
	}
}

func TestMissingUserDirectoriesAreSkipped(t *testing.T) {
	// A bare home has neither ~/.mackup nor the XDG apps dir; assembly must not
	// fail, it just reads the built-in tier.
	home := t.TempDir()
	b := builtin(map[string]string{"solo.cfg": def("Solo", nil, nil)})
	got := Assemble(home, "", b).Keys()
	if !reflect.DeepEqual(got, []string{"solo"}) {
		t.Errorf("Keys = %v, want [solo]", got)
	}
}

func TestCustomXDGBaseRelativization(t *testing.T) {
	// With a custom in-home XDG base, xdg entries relativize against it, and the
	// XDG apps discovery directory is under it too.
	home := t.TempDir()
	xdg := filepath.Join(home, "xdgroot")
	writeDef(t, filepath.Join(xdg, "mackup", "applications"), "app.cfg",
		def("App", nil, []string{"app/settings"}))

	app, ok := Assemble(home, xdg, builtin(nil)).Lookup("app")
	if !ok {
		t.Fatal("app not found under custom XDG apps dir")
	}
	want := filepath.Join("xdgroot", "app", "settings")
	if got := app.Files(); len(got) != 1 || got[0] != want {
		t.Errorf("Files = %v, want [%s]", got, want)
	}
}

func TestFilesReturnsAnOwnedCopy(t *testing.T) {
	// The assembled database is the authoritative source; mutating a slice
	// handed out by Files() must not corrupt the stored file set.
	home := t.TempDir()
	writeDef(t, legacyDir(home), "vim.cfg", def("Vim", []string{".vimrc"}, nil))
	db := Assemble(home, "", builtin(nil))

	first, _ := db.Lookup("vim")
	first.Files()[0] = "hacked"

	second, _ := db.Lookup("vim")
	if got := second.Files(); len(got) != 1 || got[0] != ".vimrc" {
		t.Errorf("Files = %v, want the stored set unchanged by a caller's mutation", got)
	}
}

func TestUnsupportedApplicationLookupIsTypedAbsence(t *testing.T) {
	db := Assemble(t.TempDir(), "", builtin(map[string]string{"x.cfg": def("X", nil, nil)}))
	if _, ok := db.Lookup("nope"); ok {
		t.Error("Lookup(nope) reported found — an unknown key must be a typed absence")
	}
}

func TestAbsoluteConfigPathIsFatal(t *testing.T) {
	home := t.TempDir()
	writeDef(t, legacyDir(home), "bad.cfg", def("Bad", []string{"/etc/passwd"}, nil))
	mustPanic(t, "Unsupported absolute path: /etc/passwd", func() { Assemble(home, "", builtin(nil)) })
}

func TestAbsoluteXDGPathIsFatal(t *testing.T) {
	home := t.TempDir()
	writeDef(t, legacyDir(home), "bad.cfg", def("Bad", nil, []string{"/etc/shadow"}))
	mustPanic(t, "Unsupported absolute path: /etc/shadow", func() { Assemble(home, "", builtin(nil)) })
}

func TestDotDotEscapeInConfigPathIsFatal(t *testing.T) {
	// The home-relativity guarantee covers [configuration_files] too: a `..`
	// sequence that leaves home, though it carries no leading slash, must be
	// fatal — otherwise the sync engine writes outside home.
	home := t.TempDir()
	writeDef(t, legacyDir(home), "bad.cfg", def("Bad", []string{"../../rachel/data"}, nil))
	mustPanic(t, "escapes the home directory", func() { Assemble(home, "", builtin(nil)) })
}

func TestDotDotEscapeInXDGPathIsFatal(t *testing.T) {
	// Same escape through an [xdg_configuration_files] entry, after XDG
	// relativization.
	home := t.TempDir()
	writeDef(t, legacyDir(home), "bad.cfg", def("Bad", nil, []string{"../../../rachel/data"}))
	mustPanic(t, "escapes the home directory", func() { Assemble(home, "", builtin(nil)) })
}

func TestInternalDotDotThatStaysUnderHomeIsAllowed(t *testing.T) {
	// A `..` that resolves back to a path still under home is legitimate and
	// must not be rejected — only escapes are fatal.
	home := t.TempDir()
	writeDef(t, legacyDir(home), "app.cfg", def("App", []string{".config/../local/foo"}, nil))
	app, ok := Assemble(home, "", builtin(nil)).Lookup("app")
	if !ok || len(app.Files()) != 1 || app.Files()[0] != ".config/../local/foo" {
		t.Errorf("Files = %v, want the in-home path stored verbatim", app.Files())
	}
}

func TestOutOfHomeXDGBaseIsFatal(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir() // a sibling temp dir, not under home
	mustPanic(t, "within your home directory", func() { Assemble(home, outside, builtin(nil)) })
}

func TestMissingApplicationNameIsFatal(t *testing.T) {
	home := t.TempDir()
	writeDef(t, legacyDir(home), "noname.cfg", "[configuration_files]\n.noname\n")
	mustPanic(t, "no [application] name", func() { Assemble(home, "", builtin(nil)) })
}

// mustPanic asserts fn panics with a message containing want — the unguarded
// failure regime of appspec/01 §6, observed at the package boundary.
func mustPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("no panic; want one mentioning %q", want)
		}
		if msg := fmt.Sprint(r); !strings.Contains(msg, want) {
			t.Fatalf("panic = %q, want it to mention %q", msg, want)
		}
	}()
	fn()
}
