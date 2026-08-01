package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func runCLI(t *testing.T, argv ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, err bytes.Buffer
	code = Run(argv, &out, &err)
	return out.String(), err.String(), code
}

// These tests assert the machine-read facts of appspec/02: which stream each
// message class lands on and which exit code accompanies it — the contract,
// not the wording. [LAW:behavior-not-structure]

func TestHelpGoesToStdoutExitZero(t *testing.T) {
	for _, argv := range [][]string{{"-h"}, {"--help"}} {
		stdout, stderr, code := runCLI(t, argv...)
		if code != 0 {
			t.Errorf("%v: exit = %d, want 0", argv, code)
		}
		if !strings.Contains(stdout, "Usage:") {
			t.Errorf("%v: stdout lacks usage block:\n%s", argv, stdout)
		}
		if stderr != "" {
			t.Errorf("%v: stderr = %q, want empty", argv, stderr)
		}
	}
}

func TestVersionGoesToStdoutExitZero(t *testing.T) {
	stdout, stderr, code := runCLI(t, "--version")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	// appspec/02: the line is `Mackup <version>`; the version value is the
	// installed package version or the stable fallback token. appspec/07
	// colors it at info level (yellow), unconditionally.
	if !regexp.MustCompile(`^\x1b\[33mMackup \S+\x1b\[0m\n$`).MatchString(stdout) {
		t.Errorf("stdout = %q, want a single yellow 'Mackup <version>' line", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestForceConflictExactStderrLineExitOne(t *testing.T) {
	stdout, stderr, code := runCLI(t, "--force", "--force-no", "backup")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	// The wording is verbatim contract; appspec/02 wraps every fatal exit-1
	// diagnostic in fatal-error color.
	if want := "\x1b[91mOptions --force and --force-no are mutually exclusive.\x1b[0m\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestUsageErrorWarningAndUsageOnStderr(t *testing.T) {
	stdout, stderr, code := runCLI(t, "frobnicate")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "frobnicate") || !strings.Contains(stderr, "Usage:") {
		t.Errorf("stderr lacks warning naming the argument plus the usage block:\n%s", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestBareInvocationShowsUsageExitZero(t *testing.T) {
	stdout, stderr, code := runCLI(t)
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("stdout lacks usage block:\n%s", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

// Until the resolvers epic lands, every real command must hit the config-load
// gate and fail loudly — never exit 0 pretending work happened.
// [LAW:no-silent-failure]
func TestCommandsFailLoudlyAtConfigGate(t *testing.T) {
	for _, argv := range [][]string{{"list"}, {"show", "vim"}, {"backup"}, {"restore"}, {"link"}} {
		stdout, stderr, code := runCLI(t, argv...)
		if code == 0 {
			t.Errorf("%v: exit = 0, want nonzero (config gate is stubbed)", argv)
		}
		if !strings.Contains(stderr, "Error:") {
			t.Errorf("%v: stderr lacks Error diagnostic: %q", argv, stderr)
		}
		if stdout != "" {
			t.Errorf("%v: stdout = %q, want empty", argv, stdout)
		}
	}
}
