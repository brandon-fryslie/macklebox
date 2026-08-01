package conformance

import (
	"os"
	"path/filepath"
	"regexp"
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

func TestListPrintsSortedKeysAndCountTrailer(t *testing.T) {
	home := workingHome(t)
	dropDef(t, home, "zebra", "[application]\nname = Zebra\n")
	dropDef(t, home, "alpha", "[application]\nname = Alpha\n")
	dropDef(t, home, "mike", "[application]\nname = Mike\n")

	r := runEnv(t, home, nil, "list")
	if r.Exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if r.Stderr != "" {
		t.Errorf("stderr = %q, want empty", r.Stderr)
	}
	got := stripANSI(r.Stdout)
	// appspec/05 format: header, sorted keys, blank line, count trailer naming
	// the version. The version value is data, so match its shape not its text.
	want := regexp.MustCompile(
		`^Supported applications:\n - alpha\n - mike\n - zebra\n\n3 applications supported in Mackup v\S+\n$`)
	if !want.MatchString(got) {
		t.Errorf("list stdout = %q, want the sorted appspec/05 format", got)
	}
}

func TestListOutputIsColored(t *testing.T) {
	// appspec/07: all human-facing output is ANSI-colored by level; list is
	// info (yellow). The stream and text are contract; color is emitted too.
	home := workingHome(t)
	dropDef(t, home, "solo", "[application]\nname = Solo\n")
	r := runEnv(t, home, nil, "list")
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
