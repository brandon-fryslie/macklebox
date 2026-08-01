package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinkUninstallRevertsLinkToRealFile(t *testing.T) {
	home, mackup, homeFile := seedApp(t)
	runEnv(t, home, nil, "--force", "link", "install", "myapp") // home → symlink into Mackup

	r := runEnv(t, home, nil, "--force", "link", "uninstall", "myapp")
	if r.Exit != 0 {
		t.Fatalf("uninstall exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	info, err := os.Lstat(homeFile)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("home path is still a symlink (or missing): %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("reverted home mode = %o, want 600", info.Mode().Perm())
	}
	if got, _ := os.ReadFile(homeFile); string(got) != "config v1\n" {
		t.Errorf("home content = %q, want the Mackup content copied back", got)
	}
	// The Mackup copy still exists — uninstall copies out, it does not remove it.
	if _, err := os.Stat(filepath.Join(mackup, ".myapprc")); err != nil {
		t.Errorf("Mackup copy removed by uninstall: %v", err)
	}
	if !strings.Contains(stripANSI(r.Stdout), "Reverting .myapprc") {
		t.Errorf("stdout = %q, want the revert progress line", r.Stdout)
	}
}

func TestLinkUninstallVerboseProgressShowsFullPaths(t *testing.T) {
	home, mackup, _ := seedApp(t)
	runEnv(t, home, nil, "--force", "link", "install", "myapp") // home → symlink

	r := runEnv(t, home, nil, "--force", "--verbose", "link", "uninstall", "myapp")
	if r.Exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	out := stripANSI(r.Stdout)
	// The verbose form names the Mackup path and the home path, not just <f>.
	mackupCopy := filepath.Join(mackup, ".myapprc")
	if !strings.Contains(out, "Reverting "+mackupCopy) || !strings.Contains(out, " at "+filepath.Join(home, ".myapprc")) {
		t.Errorf("verbose stdout = %q, want the full-path revert form", out)
	}
}

func TestLinkUninstallProtectsForeignFileWithWarningOnStdout(t *testing.T) {
	home, _, homeFile, _ := seedMackupOnly(t)       // Mackup copy present, no home entry
	writeHome(t, home, ".myapprc", "my own file\n") // a foreign real file at home

	r := runEnv(t, home, nil, "--force", "link", "uninstall", "myapp")
	if r.Exit != 0 {
		t.Fatalf("uninstall exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	// The user's own file is untouched.
	if got, _ := os.ReadFile(homeFile); string(got) != "my own file\n" {
		t.Errorf("foreign file = %q, want it left untouched", got)
	}
	if info, _ := os.Lstat(homeFile); info.Mode()&os.ModeSymlink != 0 {
		t.Error("foreign file was clobbered into a symlink")
	}
	// The warning is on stdout, NOT stderr (appspec/06 stream note).
	if !strings.Contains(stripANSI(r.Stdout), "does not point to the original file in Mackup") {
		t.Errorf("stdout = %q, want the protect-foreign warning", stripANSI(r.Stdout))
	}
	if strings.Contains(stripANSI(r.Stderr), "does not point") {
		t.Errorf("stderr = %q, warning must go to stdout not stderr", stripANSI(r.Stderr))
	}
}

func TestLinkUninstallLeavesStorageOnlyFileAlone(t *testing.T) {
	home, mackup, homeFile, _ := seedMackupOnly(t) // Mackup copy, nothing at home
	r := runEnv(t, home, nil, "--force", "link", "uninstall", "myapp")
	if r.Exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if _, err := os.Lstat(homeFile); !os.IsNotExist(err) {
		t.Error("uninstall created a home copy for a storage-only file")
	}
	if _, err := os.Stat(filepath.Join(mackup, ".myapprc")); err != nil {
		t.Errorf("Mackup copy removed: %v", err)
	}
}

func TestLinkUninstallDryRunMutatesNothing(t *testing.T) {
	home, _, homeFile := seedApp(t)
	runEnv(t, home, nil, "--force", "link", "install", "myapp") // home → symlink
	r := runEnv(t, home, nil, "--force", "--dry-run", "link", "uninstall", "myapp")
	if r.Exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if info, _ := os.Lstat(homeFile); info.Mode()&os.ModeSymlink == 0 {
		t.Error("dry-run reverted the symlink to a real file")
	}
}

func TestLinkUninstallMissingMackupFolderIsFatal(t *testing.T) {
	home := workingHome(t)
	dropDef(t, home, "myapp", "[application]\nname = MyApp\n[configuration_files]\n.myapprc\n")
	r := runEnv(t, home, nil, "--force", "link", "uninstall", "myapp")
	if r.Exit != 1 || !strings.Contains(stripANSI(r.Stderr), "Unable to find the Mackup folder") {
		t.Errorf("exit=%d stderr=%q, want the missing-Mackup-folder fatal", r.Exit, stripANSI(r.Stderr))
	}
}
