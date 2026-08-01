package conformance

import (
	"regexp"
	"strings"
	"testing"
)

// These cases transcribe the machine-read facts of appspec/02: --help and
// --version go to stdout and exit 0, fatal errors go to stderr with exit 1,
// and the force-conflict line is verbatim contract. Each case is argv in,
// observed streams and exit code out — new behaviors join the rig as new
// cases, never new machinery. [LAW:composability]

func TestHelpPrintsToStdoutAndExitsZero(t *testing.T) {
	for _, argv := range [][]string{{"-h"}, {"--help"}} {
		r := run(t, argv...)
		if r.Exit != 0 {
			t.Errorf("mackup %v: exit = %d, want 0", argv, r.Exit)
		}
		if r.Stdout == "" {
			t.Errorf("mackup %v: stdout empty, want the help text", argv)
		}
		if r.Stderr != "" {
			t.Errorf("mackup %v: stderr = %q, want empty", argv, r.Stderr)
		}
	}
}

func TestVersionPrintsMackupLineAndExitsZero(t *testing.T) {
	r := run(t, "--version")
	if r.Exit != 0 {
		t.Errorf("exit = %d, want 0", r.Exit)
	}
	// appspec/02: the line is `Mackup <version>`; the value is the package
	// version when installed, a stable fallback token otherwise (appspec/00
	// "Provenance"), so the rig pins the shape, not the token. Coloring is
	// appspec/07's separate fact (output_test.go), so the shape is checked on
	// the stripped text. [LAW:single-enforcer]
	if !regexp.MustCompile(`^Mackup \S+\n$`).MatchString(stripANSI(r.Stdout)) {
		t.Errorf("stdout = %q, want a single 'Mackup <version>' line", r.Stdout)
	}
	if r.Stderr != "" {
		t.Errorf("stderr = %q, want empty", r.Stderr)
	}
}

func TestBareInvocationShowsUsageAndExitsZero(t *testing.T) {
	r := run(t)
	if r.Exit != 0 {
		t.Errorf("exit = %d, want 0", r.Exit)
	}
	if r.Stdout == "" {
		t.Error("stdout empty, want the usage block")
	}
	if r.Stderr != "" {
		t.Errorf("stderr = %q, want empty", r.Stderr)
	}
}

func TestForceFlagConflictIsTheExactStderrLine(t *testing.T) {
	r := run(t, "--force", "--force-no", "backup")
	if r.Exit != 1 {
		t.Errorf("exit = %d, want 1", r.Exit)
	}
	// The one line appspec/02 "Mutually exclusive force flags" specifies
	// verbatim — here the wording IS the contract. appspec/02 colors every
	// fatal exit-1 diagnostic, so the verbatim check reads through the SGR
	// wrapper; the color itself is output_test.go's fact.
	if want := "Options --force and --force-no are mutually exclusive.\n"; stripANSI(r.Stderr) != want {
		t.Errorf("stderr = %q, want %q (ignoring color)", r.Stderr, want)
	}
	if r.Stdout != "" {
		t.Errorf("stdout = %q, want empty", r.Stdout)
	}
}

func TestUsageErrorsNameTheArgumentOnStderr(t *testing.T) {
	// appspec/02 "Argument-parser behavior": an unmatched positional gets a
	// warning identifying it; `show` without <application> is a usage error.
	// The spec pins the stream and a nonzero exit, not the exit value.
	cases := []struct {
		argv  []string
		names string // substring the warning must identify
	}{
		{[]string{"frobnicate"}, "frobnicate"},
		{[]string{"list", "extra"}, "extra"},
		{[]string{"show"}, "show"},
	}
	for _, c := range cases {
		r := run(t, c.argv...)
		if r.Exit == 0 {
			t.Errorf("mackup %v: exit = 0, want nonzero", c.argv)
		}
		if !strings.Contains(r.Stderr, c.names) {
			t.Errorf("mackup %v: stderr does not identify %q:\n%s", c.argv, c.names, r.Stderr)
		}
		if r.Stdout != "" {
			t.Errorf("mackup %v: stdout = %q, want empty", c.argv, r.Stdout)
		}
	}
}

func TestAllCommandsFailIdenticallyAtTheConfigGate(t *testing.T) {
	// appspec/02 "Command dispatch order": every command except --help and
	// --version loads config first, so under a home with no usable storage
	// they all fail identically — one stderr diagnostic, exit 1, nothing on
	// stdout. True today at the stubbed gate and still true after the
	// resolvers land, because the scratch HOME has no storage engine.
	commands := [][]string{{"list"}, {"show", "vim"}, {"backup"}, {"restore"}, {"link"}, {"link", "install"}, {"link", "uninstall"}}
	var firstStderr string
	for i, argv := range commands {
		r := run(t, argv...)
		if r.Exit != 1 {
			t.Errorf("mackup %v: exit = %d, want 1", argv, r.Exit)
		}
		if r.Stderr == "" {
			t.Errorf("mackup %v: stderr empty, want a fatal diagnostic", argv)
		}
		if r.Stdout != "" {
			t.Errorf("mackup %v: stdout = %q, want empty", argv, r.Stdout)
		}
		if i == 0 {
			firstStderr = r.Stderr
		} else if r.Stderr != firstStderr {
			t.Errorf("mackup %v: stderr differs from %v's:\n got: %q\nwant: %q",
				argv, commands[0], r.Stderr, firstStderr)
		}
	}
}
