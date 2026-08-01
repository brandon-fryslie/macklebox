package conformance

import (
	"os"
	"testing"
)

// This file closes the appspec/07 error-table rows whose conformance case was
// otherwise only proven at unit level. The unguarded regime's contract is stream
// + exit + no-partial-effect, not wording (appspec/07): each case asserts a
// nonzero exit, an empty stdout, and a non-empty stderr diagnostic.

func assertUnguarded(t *testing.T, r result) {
	t.Helper()
	if r.Exit == 0 {
		t.Errorf("exit = 0, want nonzero (unguarded failure)")
	}
	if r.Stdout != "" {
		t.Errorf("stdout = %q, want empty", r.Stdout)
	}
	if r.Stderr == "" {
		t.Errorf("stderr empty, want a diagnostic")
	}
}

func TestFileSystemEngineWithoutPathIsUnguarded(t *testing.T) {
	home := t.TempDir()
	writeHome(t, home, ".mackup.cfg", "[storage]\nengine = file_system\n") // no path key
	assertUnguarded(t, runEnv(t, home, nil, "list"))
}

func TestForbiddenStorageDirectoryIsUnguarded(t *testing.T) {
	home := t.TempDir()
	writeHome(t, home, ".mackup.cfg",
		"[storage]\nengine = file_system\npath = store\ndirectory = .mackup\n") // .mackup is forbidden
	assertUnguarded(t, runEnv(t, home, nil, "list"))
}

func TestDefinitionAbsolutePathIsUnguarded(t *testing.T) {
	home := workingHome(t) // config resolves, so assembly (step 3) is reached
	dropDef(t, home, "bad", "[application]\nname = Bad\n[configuration_files]\n/etc/passwd\n")
	assertUnguarded(t, runEnv(t, home, nil, "list"))
}

func TestXDGConfigHomeOutsideHomeIsUnguarded(t *testing.T) {
	home := workingHome(t)
	// The config resolves via ~/.mackup.cfg; assembly then rejects the XDG base.
	assertUnguarded(t, runEnv(t, home, []string{"XDG_CONFIG_HOME=" + t.TempDir()}, "list"))
}

func TestFailureInsideLinkOperationStopsTheRunUncaught(t *testing.T) {
	// A link operation surfaces a per-file failure as an uncaught error that
	// stops the run (appspec/06 partial-failure), leaving no partial effect at
	// the failing file. A read-only Mackup folder makes the copy fail.
	home, mackup, homeFile := seedApp(t)
	if err := os.MkdirAll(mackup, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(mackup, 0o700); err != nil {
			t.Logf("cleanup chmod failed: %v", err)
		}
	}()

	r := runEnv(t, home, nil, "--force", "link", "install", "myapp")
	if r.Exit == 0 {
		t.Errorf("exit = 0, want nonzero (link failure stops the run)")
	}
	// The home file is untouched — the copy failed before the delete+symlink.
	if info, err := os.Lstat(homeFile); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("home file changed despite the failed link: %v", err)
	}
}
