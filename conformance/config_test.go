package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These cases transcribe the config/storage rows of the appspec/07 error
// table and the appspec/03 discovery facts at the process boundary. Guarded
// rows are a clean Error: line with exit 1; unguarded rows are an uncaught
// failure — nonzero exit, stderr only. Both regimes share "no stdout".

func writeHome(t *testing.T, home, rel, content string) {
	t.Helper()
	path := filepath.Join(home, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultConfigFailsAtDropboxDetection(t *testing.T) {
	// appspec/03 "Absent / empty": no config anywhere means engine dropbox,
	// which on a bare home is the guarded multi-line provider fatal of
	// appspec/04 — the very failure TestAllCommandsFailIdenticallyAtTheConfigGate
	// sees seven times over.
	r := run(t, "list")
	if r.Exit != 1 {
		t.Errorf("exit = %d, want 1", r.Exit)
	}
	stderr := stripANSI(r.Stderr)
	if !strings.Contains(stderr, "Dropbox install") || !strings.Contains(stderr, "https://") {
		t.Errorf("stderr = %q, want the provider name and a documentation URL", stderr)
	}
	if r.Stdout != "" {
		t.Errorf("stdout = %q, want empty", r.Stdout)
	}
}

func TestExplicitConfigMissingIsTheExactGuardedLine(t *testing.T) {
	// appspec/03 -c rules, including home-relative resolution of the
	// relative path: the message names the absolute path.
	home := t.TempDir()
	r := runEnv(t, home, nil, "-c", "nope.cfg", "list")
	if r.Exit != 1 {
		t.Errorf("exit = %d, want 1", r.Exit)
	}
	want := "Error: The config file '" + filepath.Join(home, "nope.cfg") + "' does not exist. Aborting.\n"
	if got := stripANSI(r.Stderr); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	if r.Stdout != "" {
		t.Errorf("stdout = %q, want empty", r.Stdout)
	}
}

func TestConfigOutsideHomeIsRejected(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "evil.cfg")
	if err := os.WriteFile(outside, []byte("[storage]\nengine = icloud\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runEnv(t, home, nil, "-c", outside, "list")
	if r.Exit != 1 {
		t.Errorf("exit = %d, want 1", r.Exit)
	}
	want := "Error: The config file '" + outside + "' is not in your home directory. Aborting.\n"
	if got := stripANSI(r.Stderr); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	if r.Stdout != "" {
		t.Errorf("stdout = %q, want empty", r.Stdout)
	}
}

func TestLegacySectionsAbortEveryCommandCleanly(t *testing.T) {
	home := t.TempDir()
	writeHome(t, home, ".mackup.cfg", "[Allowed Applications]\nvim\n")
	r := runEnv(t, home, nil, "backup")
	if r.Exit != 1 {
		t.Errorf("exit = %d, want 1", r.Exit)
	}
	stderr := stripANSI(r.Stderr)
	if !strings.Contains(stderr, "Old config") || strings.Count(stderr, "\n") < 2 {
		t.Errorf("stderr = %q, want a multi-line old-config rejection", stderr)
	}
	if r.Stdout != "" {
		t.Errorf("stdout = %q, want empty", r.Stdout)
	}
}

func TestUnknownEngineIsTheUnguardedRegime(t *testing.T) {
	// appspec/07 table: unguarded — an uncaught error naming the value,
	// nonzero exit, nothing on stdout. The exact exit value and traceback
	// shape are implementation-owned; the spec pins stream, nonzero, and the
	// no-stdout post-condition.
	home := t.TempDir()
	writeHome(t, home, ".mackup.cfg", "[storage]\nengine = floppynet\n")
	r := runEnv(t, home, nil, "list")
	if r.Exit == 0 {
		t.Error("exit = 0, want nonzero")
	}
	if !strings.Contains(r.Stderr, "floppynet") {
		t.Errorf("stderr = %q, want it to name the unknown engine value", r.Stderr)
	}
	if r.Stdout != "" {
		t.Errorf("stdout = %q, want empty", r.Stdout)
	}
}

func TestFileSystemEngineDefersExistenceCheckToTheGate(t *testing.T) {
	// appspec/04 clause 2 + appspec/01 §4 level 1: the file_system engine
	// performs no existence check at resolution, so a config naming a
	// nonexistent path resolves exactly like one naming an existing path. The
	// check is deferred to the universal environment gate — the existing path
	// clears every gate (exit 0), while the missing path fails there with
	// "Unable to find the storage folder: <path>", not with a resolution-time
	// provider error.
	existing := t.TempDir()
	if err := os.Mkdir(filepath.Join(existing, "store"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeHome(t, existing, ".mackup.cfg", "[storage]\nengine = file_system\npath = store\n")
	missing := t.TempDir()
	writeHome(t, missing, ".mackup.cfg", "[storage]\nengine = file_system\npath = does/not/exist\n")

	ok := runEnv(t, existing, nil, "list")
	if ok.Exit != 0 {
		t.Errorf("existing storage path: exit = %d, want 0 (all gates cleared); stderr=%q", ok.Exit, ok.Stderr)
	}

	bad := runEnv(t, missing, nil, "list")
	if bad.Exit != 1 {
		t.Errorf("missing storage path: exit = %d, want 1", bad.Exit)
	}
	stderr := stripANSI(bad.Stderr)
	if strings.Contains(stderr, "Unable to find your") {
		t.Errorf("missing path failed at resolution, not the gate: %q", stderr)
	}
	if !strings.Contains(stderr, "Unable to find the storage folder") {
		t.Errorf("missing-path stderr = %q, want the storage-folder gate error", stderr)
	}
}

func TestDiscoveryEnvironmentWiring(t *testing.T) {
	// appspec/03 discovery order at the process boundary: with no
	// ~/.mackup.cfg, MACKUP_CONFIG (engine icloud → iCloud fatal) beats the
	// XDG candidate (engine dropbox → Dropbox fatal); with MACKUP_CONFIG
	// unset, the XDG candidate is used.
	home := t.TempDir()
	writeHome(t, home, "mc.cfg", "[storage]\nengine = icloud\n")
	writeHome(t, home, "xdg/mackup/mackup.cfg", "[storage]\nengine = dropbox\n")
	xdg := "XDG_CONFIG_HOME=" + filepath.Join(home, "xdg")

	// Both winners are guarded provider fatals, so each observation also pins
	// the shared post-condition: exit 1, nothing on stdout.
	r := runEnv(t, home, []string{"MACKUP_CONFIG=~/mc.cfg", xdg}, "list")
	if !strings.Contains(stripANSI(r.Stderr), "iCloud Drive") {
		t.Errorf("with MACKUP_CONFIG set: stderr = %q, want the iCloud fatal (MACKUP_CONFIG wins)", r.Stderr)
	}
	if r.Exit != 1 || r.Stdout != "" {
		t.Errorf("with MACKUP_CONFIG set: exit = %d, stdout = %q; want 1 and empty", r.Exit, r.Stdout)
	}
	r = runEnv(t, home, []string{xdg}, "list")
	if !strings.Contains(stripANSI(r.Stderr), "Dropbox install") {
		t.Errorf("without MACKUP_CONFIG: stderr = %q, want the Dropbox fatal (XDG candidate wins)", r.Stderr)
	}
	if r.Exit != 1 || r.Stdout != "" {
		t.Errorf("without MACKUP_CONFIG: exit = %d, stdout = %q; want 1 and empty", r.Exit, r.Stdout)
	}
}
