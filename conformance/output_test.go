package conformance

import (
	"regexp"
	"testing"
)

// These cases observe the ticket's done-claim for appspec/07 "Output streams"
// and "Colored output" at the process boundary. The rig's child always writes
// to pipes — never a TTY — so SGR sequences appearing here demonstrate the
// spec's no-TTY-detection regime directly: color is emitted unconditionally.

// sgr matches any ANSI SGR escape sequence.
var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI reduces an observation to its text so verbatim-wording assertions
// check the wording contract independently of the coloring around it.
func stripANSI(s string) string { return sgr.ReplaceAllString(s, "") }

func TestVersionIsColoredOnStdoutWhenPiped(t *testing.T) {
	r := run(t, "--version")
	if r.Exit != 0 {
		t.Errorf("exit = %d, want 0", r.Exit)
	}
	// Info level per appspec/07: yellow (33), terminated with a reset, on
	// stdout even though stdout is a pipe.
	if !regexp.MustCompile(`^\x1b\[33mMackup \S+\x1b\[0m\n$`).MatchString(r.Stdout) {
		t.Errorf("stdout = %q, want a yellow SGR-wrapped 'Mackup <version>' line", r.Stdout)
	}
	if r.Stderr != "" {
		t.Errorf("stderr = %q, want empty", r.Stderr)
	}
}

func TestFatalErrorIsColoredOnStderrWhenPiped(t *testing.T) {
	// Any real command fails at the config gate under a scratch HOME; the
	// fatal `Error:` diagnostic must be bright red (91) on stderr with stdout
	// untouched, per appspec/07's stream table and color scheme. The gate
	// failure is the Dropbox provider fatal, which the spec's error table
	// makes multi-line — hence (?s), one 91-wrap around the whole diagnostic.
	r := run(t, "backup")
	if r.Exit != 1 {
		t.Errorf("exit = %d, want 1", r.Exit)
	}
	if !regexp.MustCompile(`(?s)^\x1b\[91mError: .+\x1b\[0m\n$`).MatchString(r.Stderr) {
		t.Errorf("stderr = %q, want a bright-red SGR-wrapped 'Error: …' diagnostic", r.Stderr)
	}
	if r.Stdout != "" {
		t.Errorf("stdout = %q, want empty", r.Stdout)
	}
}

func TestHelpAndUsageCarryNoLevelColor(t *testing.T) {
	// appspec/07 colors messages by level; the help/usage reference blocks
	// carry no level, so they stay plain — pinned so a future blanket
	// "paint everything" change fails loudly here.
	for _, argv := range [][]string{{"--help"}, {}} {
		r := run(t, argv...)
		if sgr.MatchString(r.Stdout) {
			t.Errorf("mackup %v: stdout contains SGR sequences, want plain text:\n%q", argv, r.Stdout)
		}
	}
}
