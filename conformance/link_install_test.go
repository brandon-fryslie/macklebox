package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These transcribe the appspec/06 "link install" done-claims at the process
// boundary: the home file becomes a symlink into the Mackup folder holding the
// real content, an immediate re-run skips (already-linked), and dry-run mutates
// nothing.

func TestLinkInstallMovesHomeIntoMackupAndSymlinksBack(t *testing.T) {
	home, mackup, homeFile := seedApp(t)
	r := runEnv(t, home, nil, "--force", "link", "install", "myapp")
	if r.Exit != 0 {
		t.Fatalf("link install exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	// The home path is now a symlink into the Mackup folder.
	target, err := os.Readlink(homeFile)
	if err != nil {
		t.Fatalf("home path is not a symlink: %v", err)
	}
	mackupCopy := filepath.Join(mackup, ".myapprc")
	if target != mackupCopy {
		t.Errorf("symlink target = %q, want %q", target, mackupCopy)
	}
	// The real content lives in the Mackup folder at 0600.
	info, err := os.Stat(mackupCopy)
	if err != nil {
		t.Fatalf("no real content in the Mackup folder: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mackup copy mode = %o, want 600", info.Mode().Perm())
	}
	if got, _ := os.ReadFile(mackupCopy); string(got) != "config v1\n" {
		t.Errorf("mackup content = %q, want the moved home content", got)
	}
	if !strings.Contains(stripANSI(r.Stdout), "Linking .myapprc") {
		t.Errorf("stdout = %q, want the link progress line", r.Stdout)
	}
}

func TestLinkInstallReRunSkipsAlreadyLinked(t *testing.T) {
	home, _, _ := seedApp(t)
	runEnv(t, home, nil, "--force", "link", "install", "myapp") // first install
	// Second run with no force: an already-linked file must be skipped without a
	// prompt (EOF on stdin would fail otherwise) and print nothing.
	r := runEnv(t, home, nil, "link", "install", "myapp")
	if r.Exit != 0 {
		t.Errorf("re-run exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if stripANSI(r.Stdout) != "" {
		t.Errorf("re-run stdout = %q, want empty (already-linked skip)", r.Stdout)
	}
}

func TestLinkInstallDryRunMutatesNothing(t *testing.T) {
	home, mackup, homeFile := seedApp(t)
	r := runEnv(t, home, nil, "--force", "--dry-run", "link", "install", "myapp")
	if r.Exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	if info, err := os.Lstat(homeFile); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Error("dry-run turned the home file into a symlink")
	}
	if _, err := os.Stat(filepath.Join(mackup, ".myapprc")); !os.IsNotExist(err) {
		t.Error("dry-run copied content into the Mackup folder")
	}
	if !strings.Contains(stripANSI(r.Stdout), "Linking .myapprc") {
		t.Errorf("dry-run stdout = %q, want the would-do progress line", r.Stdout)
	}
}

func TestLinkInstallUnknownApplicationFails(t *testing.T) {
	home := workingHome(t)
	r := runEnv(t, home, nil, "--force", "link", "install", "nope")
	if r.Exit != 1 || !strings.Contains(stripANSI(r.Stderr), "Unsupported application: nope") {
		t.Errorf("exit=%d stderr=%q, want Unsupported application and exit 1", r.Exit, stripANSI(r.Stderr))
	}
}
