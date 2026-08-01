package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These transcribe appspec/06 "Whole-Mackup mode": the no-argument `link` and
// `link uninstall` ceremonies.

func TestFullLinkReloadsConfigMidRun(t *testing.T) {
	// The done-claim: a full link picks up a scope change carried by the
	// just-linked shared config for the remaining apps of the SAME run. The
	// initial ~/.mackup.cfg ignores extraapp; the shared config in Mackup does
	// not. After linking mackup (which links ~/.mackup.cfg to the shared config),
	// the reload must bring extraapp into scope and link it.
	home := t.TempDir()
	writeHome(t, home, ".mackup.cfg",
		"[storage]\nengine = file_system\npath = storage\n[applications_to_ignore]\nextraapp\n")
	writeHome(t, home, filepath.Join("storage", "Mackup", ".mackup.cfg"),
		"[storage]\nengine = file_system\npath = storage\n") // shared config: no ignore
	dropDef(t, home, "extraapp", "[application]\nname = Extra\n[configuration_files]\n.extrarc\n")
	writeHome(t, home, filepath.Join("storage", "Mackup", ".extrarc"), "extra content\n")

	r := runEnv(t, home, nil, "--force", "link")
	if r.Exit != 0 {
		t.Fatalf("full link exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	// extraapp was linked only because the reload saw the shared config.
	target, err := os.Readlink(filepath.Join(home, ".extrarc"))
	if err != nil || target != filepath.Join(home, "storage", "Mackup", ".extrarc") {
		t.Errorf("home/.extrarc = (%q, %v); want a symlink — the reload must bring extraapp into scope", target, err)
	}
}

func TestFullUninstallRevertsKeepsStorageAndPrintsClosing(t *testing.T) {
	home := t.TempDir()
	writeHome(t, home, ".mackup.cfg", "[storage]\nengine = file_system\npath = storage\n")
	dropDef(t, home, "myapp", "[application]\nname = MyApp\n[configuration_files]\n.myapprc\n")
	mackupCopy := filepath.Join(home, "storage", "Mackup", ".myapprc")
	writeHome(t, home, filepath.Join("storage", "Mackup", ".myapprc"), "stored content\n")
	if err := os.Symlink(mackupCopy, filepath.Join(home, ".myapprc")); err != nil {
		t.Fatal(err)
	}

	r := runEnv(t, home, nil, "--force", "link", "uninstall")
	if r.Exit != 0 {
		t.Fatalf("full uninstall exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	// The linked file reverted to a real file with the stored content.
	if info, err := os.Lstat(filepath.Join(home, ".myapprc")); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("home/.myapprc is missing or still a symlink after full uninstall: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(home, ".myapprc")); string(got) != "stored content\n" {
		t.Errorf("home/.myapprc = %q, want the stored content copied back", got)
	}
	// The storage folder is deliberately NOT deleted (cross-machine safety).
	if _, err := os.Stat(filepath.Join(home, "storage", "Mackup")); err != nil {
		t.Errorf("the Mackup folder was removed: %v", err)
	}
	if _, err := os.Stat(mackupCopy); err != nil {
		t.Errorf("the storage copy was removed: %v", err)
	}
	if !strings.Contains(stripANSI(r.Stdout), "You can safely uninstall Mackup now") {
		t.Errorf("stdout = %q, want the closing message", stripANSI(r.Stdout))
	}
}

func TestFullUninstallGlobalConfirmationDeclinedDoesNothing(t *testing.T) {
	home := t.TempDir()
	writeHome(t, home, ".mackup.cfg", "[storage]\nengine = file_system\npath = storage\n")
	dropDef(t, home, "myapp", "[application]\nname = MyApp\n[configuration_files]\n.myapprc\n")
	mackupCopy := filepath.Join(home, "storage", "Mackup", ".myapprc")
	writeHome(t, home, filepath.Join("storage", "Mackup", ".myapprc"), "stored\n")
	if err := os.Symlink(mackupCopy, filepath.Join(home, ".myapprc")); err != nil {
		t.Fatal(err)
	}

	// --force-no declines the global confirmation: nothing happens, exit 0.
	r := runEnv(t, home, nil, "--force-no", "link", "uninstall")
	if r.Exit != 0 {
		t.Errorf("declined full uninstall exit = %d, want 0", r.Exit)
	}
	if info, err := os.Lstat(filepath.Join(home, ".myapprc")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("declined uninstall changed the link (err=%v)", err)
	}
}

func TestNamedLinkUninstallSkipsCeremony(t *testing.T) {
	home := t.TempDir()
	writeHome(t, home, ".mackup.cfg", "[storage]\nengine = file_system\npath = storage\n")
	dropDef(t, home, "myapp", "[application]\nname = MyApp\n[configuration_files]\n.myapprc\n")
	mackupCopy := filepath.Join(home, "storage", "Mackup", ".myapprc")
	writeHome(t, home, filepath.Join("storage", "Mackup", ".myapprc"), "stored\n")
	if err := os.Symlink(mackupCopy, filepath.Join(home, ".myapprc")); err != nil {
		t.Fatal(err)
	}

	// A named app with no force and no stdin must not reach the global
	// confirmation (which would EOF-fail); the plain per-app procedure runs.
	r := runEnv(t, home, nil, "link", "uninstall", "myapp")
	if r.Exit != 0 {
		t.Fatalf("named uninstall exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	out := stripANSI(r.Stdout)
	if strings.Contains(out, "Are you sure") {
		t.Error("named uninstall showed the global confirmation")
	}
	if strings.Contains(out, "safely uninstall Mackup") {
		t.Error("named uninstall showed the closing ceremony message")
	}
}
