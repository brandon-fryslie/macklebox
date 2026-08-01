package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The cross-cutting uniformity rules of appspec/01 §3, checked against the whole
// command surface: verbose observational purity, the two do-not-generalize
// stdout messages, and dry-run's uniform no-mutation (the per-command dry-run
// cases live with each command; here restore closes the set).
//
// The root guard (appspec/07) applies to every command including list/show, but
// its refusal path needs an effective UID of 0, which the black-box rig never
// has; it is exercised in-process in internal/cli (TestCheckEnvironmentRootGuard).

func TestDriftHeaderAndDiffGoToStdoutNotStderr(t *testing.T) {
	// appspec/06 stream note (do-not-generalize): the "differs between …" header
	// and the diff are printed to stdout, not stderr.
	home, _, homeFile := seedApp(t)
	if s := runEnv(t, home, nil, "--force", "backup", "myapp"); s.Exit != 0 { // mackup copy = v1
		t.Fatalf("setup backup failed: exit=%d stderr=%q", s.Exit, s.Stderr)
	}
	if err := os.WriteFile(homeFile, []byte("config v2 changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runEnv(t, home, nil, "--force", "backup", "myapp") // diverged → shows the diff
	if r.Exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if !strings.Contains(stripANSI(r.Stdout), ".myapprc differs between home and Mackup:") {
		t.Errorf("stdout = %q, want the differs-between header", stripANSI(r.Stdout))
	}
	if strings.Contains(stripANSI(r.Stderr), "differs between") {
		t.Errorf("stderr = %q, the differs header must be on stdout", stripANSI(r.Stderr))
	}
}

func TestVerboseIsObservationallyPure(t *testing.T) {
	// Verbose changes only output, never the filesystem effect: a --verbose
	// backup and a plain backup of the same input produce the identical Mackup
	// copy (content and mode).
	plainHome, plainMackup, _ := seedApp(t)
	verboseHome, verboseMackup, _ := seedApp(t)

	if s := runEnv(t, plainHome, nil, "--force", "backup", "myapp"); s.Exit != 0 {
		t.Fatalf("plain setup backup failed: exit=%d stderr=%q", s.Exit, s.Stderr)
	}
	rv := runEnv(t, verboseHome, nil, "--force", "--verbose", "backup", "myapp")
	if rv.Exit != 0 {
		t.Fatalf("verbose backup exit = %d; stderr=%q", rv.Exit, rv.Stderr)
	}

	plainFile := filepath.Join(plainMackup, ".myapprc")
	verboseFile := filepath.Join(verboseMackup, ".myapprc")
	pc, pe := os.ReadFile(plainFile)
	vc, ve := os.ReadFile(verboseFile)
	if pe != nil || ve != nil {
		t.Fatalf("could not read the copies: plain=%v verbose=%v", pe, ve)
	}
	if string(pc) != string(vc) {
		t.Errorf("verbose vs plain content differ: %q vs %q", pc, vc)
	}
	pm, err1 := os.Stat(plainFile)
	vm, err2 := os.Stat(verboseFile)
	if err1 != nil || err2 != nil {
		t.Fatalf("could not stat the copies: plain=%v verbose=%v", err1, err2)
	}
	if pm.Mode() != vm.Mode() {
		t.Errorf("verbose vs plain mode differ: %v vs %v", pm.Mode(), vm.Mode())
	}
	// And the verbose run actually produced the long-form output.
	if !strings.Contains(stripANSI(rv.Stdout), "Backing up\n") {
		t.Errorf("verbose stdout = %q, want the long-form progress", stripANSI(rv.Stdout))
	}
}

func TestRestoreDryRunMutatesNothing(t *testing.T) {
	home, _, homeFile := seedApp(t)
	if s := runEnv(t, home, nil, "--force", "backup", "myapp"); s.Exit != 0 { // mackup copy exists
		t.Fatalf("setup backup failed: exit=%d stderr=%q", s.Exit, s.Stderr)
	}
	if err := os.WriteFile(homeFile, []byte("local edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runEnv(t, home, nil, "--force", "--dry-run", "restore", "myapp")
	if r.Exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if got, err := os.ReadFile(homeFile); err != nil || string(got) != "local edit\n" {
		t.Errorf("home file = %q, %v; dry-run restore must not overwrite it", got, err)
	}
}
