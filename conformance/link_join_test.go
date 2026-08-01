package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedMackupOnly sets up a working home whose Mackup folder already holds a
// definition's content, with nothing at the home path — the join-an-existing-sync
// starting state for `link`. Returns (home, mackup folder, home path, mackup copy).
func seedMackupOnly(t *testing.T) (home, mackup, homeFile, mackupCopy string) {
	t.Helper()
	home = workingHome(t)
	dropDef(t, home, "myapp", "[application]\nname = MyApp\n[configuration_files]\n.myapprc\n")
	mackup = filepath.Join(home, "storage", "Mackup")
	mackupCopy = filepath.Join(mackup, ".myapprc")
	writeHome(t, home, filepath.Join("storage", "Mackup", ".myapprc"), "from mackup\n")
	return home, mackup, filepath.Join(home, ".myapprc"), mackupCopy
}

func TestLinkCreatesSymlinkWhenNothingAtHome(t *testing.T) {
	home, _, homeFile, mackupCopy := seedMackupOnly(t)
	r := runEnv(t, home, nil, "--force", "link", "myapp")
	if r.Exit != 0 {
		t.Fatalf("link exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	target, err := os.Readlink(homeFile)
	if err != nil || target != mackupCopy {
		t.Errorf("home path = (%q, %v); want a symlink into the Mackup folder", target, err)
	}
	// The Mackup copy is not modified by link.
	if got, _ := os.ReadFile(mackupCopy); string(got) != "from mackup\n" {
		t.Errorf("mackup copy = %q, want it unmodified by link", got)
	}
	if !strings.Contains(stripANSI(r.Stdout), "Restoring .myapprc") {
		t.Errorf("stdout = %q, want the restore progress line", r.Stdout)
	}
}

func TestLinkReplacesExistingHomeFileOnConfirm(t *testing.T) {
	home, _, homeFile, mackupCopy := seedMackupOnly(t)
	writeHome(t, home, ".myapprc", "local edit\n") // a real file already at home

	r := runStdin(t, home, "yes\n", nil, "link", "myapp")
	if r.Exit != 0 {
		t.Fatalf("link exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if !strings.Contains(stripANSI(r.Stdout), "Do you want to replace it with your backup?") {
		t.Errorf("stdout = %q, want the replace-home prompt", stripANSI(r.Stdout))
	}
	target, err := os.Readlink(homeFile)
	if err != nil || target != mackupCopy {
		t.Errorf("home path = (%q, %v); want a symlink after replace", target, err)
	}
}

func TestLinkReRunSkipsAlreadyLinked(t *testing.T) {
	home, _, _, _ := seedMackupOnly(t)
	runEnv(t, home, nil, "--force", "link", "myapp") // first link
	// Second run with no force: an already-linked file is skipped without a
	// prompt (EOF would fail otherwise) and prints nothing.
	r := runEnv(t, home, nil, "link", "myapp")
	if r.Exit != 0 || stripANSI(r.Stdout) != "" {
		t.Errorf("re-run: exit=%d stdout=%q, want exit 0 and empty stdout", r.Exit, r.Stdout)
	}
}

func TestLinkDryRunMutatesNothing(t *testing.T) {
	home, _, homeFile, _ := seedMackupOnly(t)
	r := runEnv(t, home, nil, "--force", "--dry-run", "link", "myapp")
	if r.Exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if _, err := os.Lstat(homeFile); !os.IsNotExist(err) {
		t.Error("dry-run created a symlink at the home path")
	}
	if !strings.Contains(stripANSI(r.Stdout), "Restoring .myapprc") {
		t.Errorf("dry-run stdout = %q, want the would-do progress line", r.Stdout)
	}
}

func TestLinkMissingMackupFolderIsFatal(t *testing.T) {
	home := workingHome(t)
	dropDef(t, home, "myapp", "[application]\nname = MyApp\n[configuration_files]\n.myapprc\n")
	// No Mackup folder exists; link requires it.
	r := runEnv(t, home, nil, "--force", "link", "myapp")
	if r.Exit != 1 || !strings.Contains(stripANSI(r.Stderr), "Unable to find the Mackup folder") {
		t.Errorf("exit=%d stderr=%q, want the missing-Mackup-folder fatal", r.Exit, stripANSI(r.Stderr))
	}
}
