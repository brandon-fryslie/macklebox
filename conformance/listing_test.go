package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These cases transcribe the appspec/05 Enumeration formats and the appspec/02
// observed fact that list/show sit behind the same universal gates as the sync
// commands. They run the real binary end-to-end through the whole startup
// pipeline of appspec/01 §4.

// workingHome returns a home whose config resolves to an existing file_system
// storage root, so the startup pipeline clears steps 2–4 and reaches list/show.
func workingHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	writeHome(t, home, ".mackup.cfg", "[storage]\nengine = file_system\npath = storage\n")
	if err := os.MkdirAll(filepath.Join(home, "storage"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

// dropDef writes an application definition into the legacy user directory
// (~/.mackup), the highest-precedence discovery tier — so the test controls the
// database without a shipped catalog.
func dropDef(t *testing.T, home, key, body string) {
	t.Helper()
	writeHome(t, home, filepath.Join(".mackup", key+".cfg"), body)
}

func TestDroppingAUserDefinitionAddsOneSortedKey(t *testing.T) {
	// appspec/05 observed effect: dropping ~/.mackup/<key>.cfg makes the key
	// appear in list and increments the count trailer by exactly one. Asserting
	// the delta rather than a fixed total keeps this test about list's behavior,
	// not the size of the shipped catalog.
	home := workingHome(t)
	before := runEnv(t, home, nil, "list")
	if before.Exit != 0 {
		t.Fatalf("baseline list exit = %d, want 0; stderr=%q", before.Exit, before.Stderr)
	}
	_, beforeCount := listedKeys(t, before.Stdout)

	dropDef(t, home, "zzz-user-app", "[application]\nname = ZZZ User App\n")
	after := runEnv(t, home, nil, "list")
	if after.Exit != 0 {
		t.Fatalf("list after drop exit = %d, want 0; stderr=%q", after.Exit, after.Stderr)
	}
	gotKeys, afterCount := listedKeys(t, after.Stdout)

	if afterCount != beforeCount+1 {
		t.Errorf("count trailer = %d, want %d (one more than baseline)", afterCount, beforeCount+1)
	}
	found := false
	for i, k := range gotKeys {
		if k == "zzz-user-app" {
			found = true
		}
		if i > 0 && gotKeys[i-1] > k {
			t.Errorf("list not sorted ascending: %q precedes %q", gotKeys[i-1], k)
		}
	}
	if !found {
		t.Error("dropped user definition zzz-user-app did not appear in list")
	}
	if len(gotKeys) != afterCount {
		t.Errorf("printed %d keys but count trailer says %d", len(gotKeys), afterCount)
	}
}

func TestListOutputIsColored(t *testing.T) {
	// appspec/07: all human-facing output is ANSI-colored by level; list is
	// info (yellow). The stream and text are contract; color is emitted too.
	home := workingHome(t)
	dropDef(t, home, "solo", "[application]\nname = Solo\n")
	r := runEnv(t, home, nil, "list")
	if r.Exit != 0 {
		t.Fatalf("list exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if !strings.HasPrefix(r.Stdout, "\x1b[33m") {
		t.Errorf("list stdout is not info-colored: %q", r.Stdout)
	}
}

func TestShowPrintsNameAndSortedFiles(t *testing.T) {
	home := workingHome(t)
	dropDef(t, home, "git",
		"[application]\nname = Git\n[configuration_files]\n.gitconfig\n[xdg_configuration_files]\ngit/config\n")

	r := runEnv(t, home, nil, "show", "git")
	if r.Exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if r.Stderr != "" {
		t.Errorf("stderr = %q, want empty", r.Stderr)
	}
	// appspec/05: display name, then sorted file set (config verbatim + XDG
	// home-relativized against the default ~/.config).
	want := "Name: Git\nConfiguration files:\n - .config/git/config\n - .gitconfig\n"
	if got := stripANSI(r.Stdout); got != want {
		t.Errorf("show stdout = %q, want %q", got, want)
	}
}

func TestShowUnknownApplicationIsTheContractLine(t *testing.T) {
	home := workingHome(t)
	r := runEnv(t, home, nil, "show", "nope")
	if r.Exit != 1 {
		t.Errorf("exit = %d, want 1", r.Exit)
	}
	// appspec/07: the "Unsupported application:" prefix is a literal contract
	// token, on stderr, exit 1, with no stdout.
	if got := stripANSI(r.Stderr); got != "Unsupported application: nope\n" {
		t.Errorf("stderr = %q, want the exact Unsupported application line", got)
	}
	if r.Stdout != "" {
		t.Errorf("stdout = %q, want empty", r.Stdout)
	}
}

func TestListAndShowFailIdenticallyToBackupWhenStorageMissing(t *testing.T) {
	// appspec/02 + appspec/01 §4 level 1: list/show run the same usable-env
	// gate as sync commands, so an unlocatable storage folder fails them all
	// identically — this is where file_system's deferred existence check fires.
	home := t.TempDir()
	writeHome(t, home, ".mackup.cfg", "[storage]\nengine = file_system\npath = nostorage\n")

	backup := runEnv(t, home, nil, "backup")
	if backup.Exit != 1 || !strings.Contains(stripANSI(backup.Stderr), "Unable to find the storage folder") {
		t.Fatalf("backup did not fail at the storage gate: exit=%d stderr=%q", backup.Exit, backup.Stderr)
	}
	for _, argv := range [][]string{{"list"}, {"show", "git"}} {
		r := runEnv(t, home, nil, argv...)
		if r.Exit != backup.Exit {
			t.Errorf("%v exit = %d, want %d (identical to backup)", argv, r.Exit, backup.Exit)
		}
		if r.Stdout != "" {
			t.Errorf("%v stdout = %q, want empty", argv, r.Stdout)
		}
		if stripANSI(r.Stderr) != stripANSI(backup.Stderr) {
			t.Errorf("%v stderr = %q, want identical to backup %q", argv, r.Stderr, backup.Stderr)
		}
	}
}
