package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The tests here drive Load through real files under scratch homes — the
// same observation surface a process-level caller has, minus the process.
// [LAW:behavior-not-structure]

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fsConfig is a config file selecting the file_system engine with the given
// storage path. Distinct paths per candidate file make Root() reveal which
// candidate discovery chose.
func fsConfig(path string) string {
	return "[storage]\nengine = file_system\npath = " + path + "\n"
}

func load(t *testing.T, env Env, explicit string) Config {
	t.Helper()
	cfg, err := Load(env, explicit)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestDiscoveryHomeFileAlwaysWins(t *testing.T) {
	// appspec/03 observed fact: ~/.mackup.cfg wins even when MACKUP_CONFIG
	// and XDG_CONFIG_HOME both point at other existing config files.
	home := t.TempDir()
	write(t, filepath.Join(home, ".mackup.cfg"), fsConfig("/from-home"))
	write(t, filepath.Join(home, "mc.cfg"), fsConfig("/from-mackup-config"))
	write(t, filepath.Join(home, "xdg", "mackup", "mackup.cfg"), fsConfig("/from-xdg"))
	env := Env{Home: home, MackupConfig: filepath.Join(home, "mc.cfg"), XDGConfigHome: filepath.Join(home, "xdg")}

	if got := load(t, env, "").Root(); got != "/from-home" {
		t.Errorf("Root = %q, want the ~/.mackup.cfg candidate", got)
	}
}

func TestDiscoveryMackupConfigBeatsXDG(t *testing.T) {
	// appspec/03 observed fact: with no ~/.mackup.cfg, MACKUP_CONFIG wins
	// over an existing XDG candidate.
	home := t.TempDir()
	write(t, filepath.Join(home, "mc.cfg"), fsConfig("/from-mackup-config"))
	write(t, filepath.Join(home, "xdg", "mackup", "mackup.cfg"), fsConfig("/from-xdg"))
	env := Env{Home: home, MackupConfig: "~/mc.cfg", XDGConfigHome: filepath.Join(home, "xdg")}

	if got := load(t, env, "").Root(); got != "/from-mackup-config" {
		t.Errorf("Root = %q, want the MACKUP_CONFIG candidate", got)
	}
}

func TestDiscoveryXDGDefaultsToDotConfig(t *testing.T) {
	// appspec/03: with XDG_CONFIG_HOME unset the third candidate is
	// ~/.config/mackup/mackup.cfg — note the dotless filename.
	home := t.TempDir()
	write(t, filepath.Join(home, ".config", "mackup", "mackup.cfg"), fsConfig("/from-xdg-default"))

	if got := load(t, Env{Home: home}, "").Root(); got != "/from-xdg-default" {
		t.Errorf("Root = %q, want the ~/.config candidate", got)
	}
}

func TestNoConfigAnywhereAppliesDefaults(t *testing.T) {
	// appspec/03 "Absent / empty": nothing set means engine dropbox, so on a
	// bare home the failure is Dropbox detection — proof the default engine
	// applied — and it is the guarded regime (an error, not a panic).
	_, err := Load(Env{Home: t.TempDir()}, "")
	if err == nil || !strings.Contains(err.Error(), "Dropbox install") {
		t.Errorf("err = %v, want the Dropbox provider fatal", err)
	}
}

func TestDefaultsDirectoryAndScopeOnEmptyConfig(t *testing.T) {
	// Same absent-config path but with a resolvable engine, to observe the
	// remaining defaults: directory Mackup, no allow, no ignore.
	home := t.TempDir()
	write(t, filepath.Join(home, ".mackup.cfg"), fsConfig("store"))
	cfg := load(t, Env{Home: home}, "")

	if cfg.Directory() != "Mackup" {
		t.Errorf("Directory = %q, want Mackup", cfg.Directory())
	}
	if want := filepath.Join(home, "store", "Mackup"); cfg.MackupFolder() != want {
		t.Errorf("MackupFolder = %q, want %q", cfg.MackupFolder(), want)
	}
	all := []string{"git", "vim", "zsh"}
	if got := cfg.Scope(all); !reflect.DeepEqual(got, all) {
		t.Errorf("Scope = %v, want all keys %v", got, all)
	}
}

func TestExplicitConfigTildeAndRelativeResolveToHome(t *testing.T) {
	// appspec/03 -c rules: ~ expands to home; a relative path is
	// home-relative, not CWD-relative.
	home := t.TempDir()
	write(t, filepath.Join(home, "sub", "alt.cfg"), fsConfig("/alt"))

	for _, explicit := range []string{"~/sub/alt.cfg", "sub/alt.cfg"} {
		if got := load(t, Env{Home: home}, explicit).Root(); got != "/alt" {
			t.Errorf("-c %q: Root = %q, want /alt", explicit, got)
		}
	}
}

func TestExplicitConfigSkipsDiscovery(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".mackup.cfg"), fsConfig("/from-home"))
	write(t, filepath.Join(home, "alt.cfg"), fsConfig("/from-explicit"))

	if got := load(t, Env{Home: home}, "alt.cfg").Root(); got != "/from-explicit" {
		t.Errorf("Root = %q, want the -c file to shadow discovery", got)
	}
}

func TestExplicitConfigMustExist(t *testing.T) {
	home := t.TempDir()
	_, err := Load(Env{Home: home}, "missing.cfg")
	want := "The config file '" + filepath.Join(home, "missing.cfg") + "' does not exist. Aborting."
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

func TestContainmentRejectsPathsOutsideHome(t *testing.T) {
	// appspec/03: containment applies to explicit and discovered paths alike.
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "evil.cfg")
	write(t, outside, fsConfig("/evil"))
	want := "The config file '" + outside + "' is not in your home directory. Aborting."

	if _, err := Load(Env{Home: home}, outside); err == nil || err.Error() != want {
		t.Errorf("-c outside home: err = %v, want %q", err, want)
	}
	if _, err := Load(Env{Home: home, MackupConfig: outside}, ""); err == nil || err.Error() != want {
		t.Errorf("MACKUP_CONFIG outside home: err = %v, want %q", err, want)
	}
}

func TestLegacySectionsAreRejected(t *testing.T) {
	// appspec/03 "Legacy config rejection": guarded, multi-line, and it
	// blocks the run before any storage resolution could succeed.
	for _, section := range []string{"[Allowed Applications]", "[Ignored Applications]"} {
		home := t.TempDir()
		write(t, filepath.Join(home, ".mackup.cfg"), fsConfig("store")+section+"\nvim\n")
		_, err := Load(Env{Home: home}, "")
		if err == nil || !strings.Contains(err.Error(), "Old config file detected") {
			t.Errorf("%s: err = %v, want the old-config rejection", section, err)
		}
	}
}

func TestApplicationListKeysAreCaseNormalized(t *testing.T) {
	// The config half of the appspec/03 case-policy pair: listed names are
	// lowercased, so they match the lowercase database keys.
	home := t.TempDir()
	write(t, filepath.Join(home, ".mackup.cfg"),
		fsConfig("store")+"[applications_to_sync]\nLibreWolf\nVim\n")
	cfg := load(t, Env{Home: home}, "")

	got := cfg.Scope([]string{"git", "librewolf", "vim"})
	if want := []string{"librewolf", "vim"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Scope = %v, want %v", got, want)
	}
}

func TestScopeCombinedPrecedence(t *testing.T) {
	// appspec/03 "Combined precedence": allowlist when non-empty, minus the
	// denylist — a key in both lists is ignored.
	home := t.TempDir()
	write(t, filepath.Join(home, ".mackup.cfg"), fsConfig("store")+
		"[applications_to_sync]\nvim\ngit\n[applications_to_ignore]\ngit\nzsh\n")
	cfg := load(t, Env{Home: home}, "")

	got := cfg.Scope([]string{"git", "tmux", "vim", "zsh"})
	if want := []string{"vim"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Scope = %v, want %v (deny wins over allow)", got, want)
	}
}

func TestScopeDenylistAloneSubtractsFromAll(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".mackup.cfg"), fsConfig("store")+"[applications_to_ignore]\ngit\n")
	cfg := load(t, Env{Home: home}, "")

	got := cfg.Scope([]string{"git", "vim", "zsh"})
	if want := []string{"vim", "zsh"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Scope = %v, want %v", got, want)
	}
}

func TestScopeEmptyAllowSectionMeansAll(t *testing.T) {
	// appspec/03: the allowlist narrows only when present AND non-empty.
	home := t.TempDir()
	write(t, filepath.Join(home, ".mackup.cfg"), fsConfig("store")+"[applications_to_sync]\n")
	cfg := load(t, Env{Home: home}, "")

	all := []string{"git", "vim"}
	if got := cfg.Scope(all); !reflect.DeepEqual(got, all) {
		t.Errorf("Scope = %v, want all keys %v", got, all)
	}
}

func TestHomeUnsetIsUnguarded(t *testing.T) {
	// appspec/03 env table: HOME must be set; home-relative operations fail
	// with an uncaught error otherwise.
	mustPanic(t, "HOME", func() { _, _ = Load(Env{}, "") })
}
