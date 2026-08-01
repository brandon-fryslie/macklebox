// Package conformance is the black-box rig: it runs the real built binary
// under a throwaway home directory and asserts on stdout, stderr, and exit
// code — the same observation style appspec/00 "Provenance" describes for how
// the spec itself was written. It checks only the machine-read facts of the
// spec (stream routing, exit codes, verbatim contract lines); the wording of
// usage/help text is human-facing, explicitly not contract (appspec/02
// "Argument-parser behavior"), and its fidelity is covered once by the
// in-process tests in internal/cli. [LAW:single-enforcer]
package conformance

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// binPath is the binary under test, built fresh by TestMain so the rig can
// never observe a stale artifact. [LAW:one-source-of-truth]
var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "macklebox-conformance-")
	if err != nil {
		panic("conformance: create build dir: " + err.Error())
	}
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "mackup")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	build := exec.Command("go", "build", "-o", binPath,
		"github.com/brandon-fryslie/macklebox/cmd/mackup")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		os.RemoveAll(dir)
		panic("conformance: building the binary under test failed: " + err.Error())
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// result is one observation at the process boundary — everything the spec
// lets a caller see.
type result struct {
	Stdout string
	Stderr string
	Exit   int
}

// run spawns the binary with the given argv under a scratch HOME and an
// explicitly constructed environment, so nothing ambient (the developer's
// real HOME, XDG variables, an existing ~/.mackup.cfg) can leak into an
// observation. [LAW:no-ambient-temporal-coupling]
func run(t *testing.T, argv ...string) result {
	t.Helper()
	return runEnv(t, t.TempDir(), nil, argv...)
}

// runEnv is run with a caller-prepared home and extra KEY=VALUE environment
// entries, for observations that need config files or discovery variables in
// place before the process starts.
func runEnv(t *testing.T, home string, extraEnv []string, argv ...string) result {
	t.Helper()
	return runStdin(t, home, "", extraEnv, argv...)
}

// runStdin is runEnv with a prepared stdin, for commands that reach a
// confirmation prompt without a force flag.
func runStdin(t *testing.T, home, stdin string, extraEnv []string, argv ...string) result {
	t.Helper()
	cmd := exec.Command(binPath, argv...)
	cmd.Env = append([]string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}, extraEnv...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		// Only a failure to run at all is a rig error; a nonzero exit is an
		// observation, not a failure. [LAW:no-silent-failure]
		t.Fatalf("mackup %v: could not run binary: %v", argv, err)
	}
	return result{Stdout: stdout.String(), Stderr: stderr.String(), Exit: cmd.ProcessState.ExitCode()}
}
