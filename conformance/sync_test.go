package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These transcribe the appspec/06 backup/restore done-claims at the process
// boundary: idempotency, partial failure, single-app scoping, and dry-run.

// seedApp sets up a working-storage home with one application definition and its
// home config file, and returns (home, the Mackup folder, the home file path).
func seedApp(t *testing.T) (home, mackupFolder, homeFile string) {
	t.Helper()
	home = workingHome(t) // file_system storage root at ~/storage
	dropDef(t, home, "myapp", "[application]\nname = MyApp\n[configuration_files]\n.myapprc\n")
	homeFile = filepath.Join(home, ".myapprc")
	writeHome(t, home, ".myapprc", "config v1\n")
	return home, filepath.Join(home, "storage", "Mackup"), homeFile
}

func TestBackupCopiesToMackupFolderAt0600(t *testing.T) {
	home, mackup, _ := seedApp(t)
	r := runEnv(t, home, nil, "--force", "backup", "myapp")
	if r.Exit != 0 {
		t.Fatalf("backup exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	copied := filepath.Join(mackup, ".myapprc")
	info, err := os.Stat(copied)
	if err != nil {
		t.Fatalf("backup did not copy the file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("copied mode = %o, want 600", info.Mode().Perm())
	}
	if !strings.Contains(stripANSI(r.Stdout), "Backing up .myapprc") {
		t.Errorf("stdout = %q, want the progress line", r.Stdout)
	}
}

func TestSecondIdenticalBackupDoesNothingAndPromptsForNothing(t *testing.T) {
	home, _, _ := seedApp(t)
	runEnv(t, home, nil, "--force", "backup", "myapp") // first run
	// Second run with no force flag: if it tried to prompt, EOF on stdin would
	// make it fail. Identical content must skip without prompting.
	r := runEnv(t, home, nil, "backup", "myapp")
	if r.Exit != 0 {
		t.Errorf("second backup exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if stripANSI(r.Stdout) != "" {
		t.Errorf("second backup stdout = %q, want empty (idempotent skip)", r.Stdout)
	}
}

func TestBackupUnknownApplicationFailsBeforeAnyFolder(t *testing.T) {
	home := workingHome(t)
	r := runEnv(t, home, nil, "--force", "backup", "nope")
	if r.Exit != 1 {
		t.Errorf("exit = %d, want 1", r.Exit)
	}
	if got := stripANSI(r.Stderr); !strings.Contains(got, "Unsupported application: nope") {
		t.Errorf("stderr = %q, want the Unsupported application line", got)
	}
	if _, err := os.Stat(filepath.Join(home, "storage", "Mackup")); !os.IsNotExist(err) {
		t.Error("the Mackup folder was created despite the unknown application")
	}
}

func TestBackupNamedAppOverridesIgnoreList(t *testing.T) {
	home, mackup, _ := seedApp(t)
	// Ignore myapp in config; naming it must still act on it.
	writeHome(t, home, ".mackup.cfg",
		"[storage]\nengine = file_system\npath = storage\n[applications_to_ignore]\nmyapp\n")
	r := runEnv(t, home, nil, "--force", "backup", "myapp")
	if r.Exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if _, err := os.Stat(filepath.Join(mackup, ".myapprc")); err != nil {
		t.Error("named application in the ignore list was not backed up")
	}
}

func TestDryRunMutatesNoConfigFile(t *testing.T) {
	home, mackup, _ := seedApp(t)
	r := runEnv(t, home, nil, "--force", "--dry-run", "backup", "myapp")
	if r.Exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if _, err := os.Stat(filepath.Join(mackup, ".myapprc")); !os.IsNotExist(err) {
		t.Error("dry-run copied a config file")
	}
	if !strings.Contains(stripANSI(r.Stdout), "Backing up .myapprc") {
		t.Errorf("dry-run stdout = %q, want the would-do progress line", r.Stdout)
	}
}

func TestBackupPartialFailureReportsAndExitsOne(t *testing.T) {
	home, mackup, _ := seedApp(t)
	// Pre-create the Mackup folder read-only so the copy cannot write into it.
	if err := os.MkdirAll(mackup, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(mackup, 0o700); err != nil {
			t.Logf("cleanup chmod failed: %v", err)
		}
	}()

	r := runEnv(t, home, nil, "--force", "backup", "myapp")
	if r.Exit != 1 {
		t.Errorf("exit = %d, want 1 (partial failure)", r.Exit)
	}
	stderr := stripANSI(r.Stderr)
	if !strings.Contains(stderr, "Error: Unable to copy") {
		t.Errorf("stderr = %q, want the per-file copy-failure line", stderr)
	}
	if !strings.Contains(stderr, "Backup incomplete: 1 file(s) could not be copied:") {
		t.Errorf("stderr = %q, want the incomplete summary", stderr)
	}
}

func TestReplaceOverwritesWithNewContentOnForce(t *testing.T) {
	// dst exists and differs; --force replaces it with the current source.
	home, mackup, homeFile := seedApp(t)
	runEnv(t, home, nil, "--force", "backup", "myapp") // mackup/.myapprc = v1
	if err := os.WriteFile(homeFile, []byte("config v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runEnv(t, home, nil, "--force", "backup", "myapp")
	if r.Exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	got, err := os.ReadFile(filepath.Join(mackup, ".myapprc"))
	if err != nil || string(got) != "config v2\n" {
		t.Errorf("replaced copy = %q, %v; want the new content", got, err)
	}
	if m, _ := os.Stat(filepath.Join(mackup, ".myapprc")); m.Mode().Perm() != 0o600 {
		t.Errorf("replaced mode = %o, want 600", m.Mode().Perm())
	}
}

func TestFailedReplaceLeavesTheOldCopyIntact(t *testing.T) {
	// A replace whose copy fails must not leave the destination missing
	// (appspec/07: a failing operation makes no filesystem change). Seed a
	// backup, diverge the source, then make the Mackup folder read-only so the
	// staged copy cannot be written — the old copy must survive.
	home, mackup, homeFile := seedApp(t)
	runEnv(t, home, nil, "--force", "backup", "myapp") // seed mackup/.myapprc = v1
	if err := os.WriteFile(homeFile, []byte("config v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(mackup, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(mackup, 0o700); err != nil {
			t.Logf("cleanup chmod failed: %v", err)
		}
	}()

	r := runEnv(t, home, nil, "--force", "backup", "myapp")
	if r.Exit != 1 {
		t.Errorf("exit = %d, want 1 (replace failed)", r.Exit)
	}
	got, err := os.ReadFile(filepath.Join(mackup, ".myapprc"))
	if err != nil || string(got) != "config v1\n" {
		t.Errorf("old copy = %q, %v; want the original preserved after a failed replace", got, err)
	}
}

func TestBackupFolderCreationDeclineIsFatal(t *testing.T) {
	home, _, _ := seedApp(t)
	// --force-no answers the folder-creation prompt with no.
	r := runEnv(t, home, nil, "--force-no", "backup", "myapp")
	if r.Exit != 1 {
		t.Errorf("exit = %d, want 1", r.Exit)
	}
	if !strings.Contains(stripANSI(r.Stderr), "Mackup can't do anything without a home") {
		t.Errorf("stderr = %q, want the declined-folder fatal", stripANSI(r.Stderr))
	}
}

func TestBackupEndOfInputAtPromptIsNonzero(t *testing.T) {
	// Seed a backup, then diverge the home file so the next backup reaches the
	// replace prompt. With no force flag and no stdin, EOF at the prompt is the
	// unguarded regime: nonzero exit, nothing on stdout beyond the progress line.
	home, _, homeFile := seedApp(t)
	runEnv(t, home, nil, "--force", "backup", "myapp")
	if err := os.WriteFile(homeFile, []byte("config v2 changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runEnv(t, home, nil, "backup", "myapp")
	if r.Exit == 0 {
		t.Errorf("exit = 0, want nonzero (EOF at a confirmation prompt)")
	}
}

func TestRestoreMissingMackupFolderIsFatal(t *testing.T) {
	home := workingHome(t)
	dropDef(t, home, "myapp", "[application]\nname = MyApp\n[configuration_files]\n.myapprc\n")
	// No Mackup folder exists; restore requires it.
	r := runEnv(t, home, nil, "--force", "restore", "myapp")
	if r.Exit != 1 {
		t.Errorf("exit = %d, want 1", r.Exit)
	}
	if !strings.Contains(stripANSI(r.Stderr), "Unable to find the Mackup folder") {
		t.Errorf("stderr = %q, want the missing-Mackup-folder fatal", stripANSI(r.Stderr))
	}
}
