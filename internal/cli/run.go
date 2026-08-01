package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/brandon-fryslie/macklebox/internal/config"
)

// forceConflictLine is the exact stderr line appspec/02 "Mutually exclusive
// force flags" specifies, verbatim.
const forceConflictLine = "Options --force and --force-no are mutually exclusive."

// Run executes one invocation against the given streams and returns the
// process exit code. It is the whole dispatch pipeline of appspec/01 §4:
// parse → config load → subcommand. Streams and exit code are parameters so
// the entire boundary contract is testable in-process.
// [LAW:effects-at-boundaries]
func Run(argv []string, stdout, stderr io.Writer) int {
	switch inv := Parse(argv).(type) {
	case Help:
		fmt.Fprint(stdout, helpText)
		return 0
	case Version:
		// Info-level per appspec/07: colored unconditionally, even piped.
		fmt.Fprintln(stdout, info.paint("Mackup "+versionString()))
		return 0
	case ShowUsage:
		fmt.Fprint(stdout, usageText)
		return 0
	case UsageError:
		fmt.Fprintf(stderr, "mackup: %s\n%s", inv.Warning, usageText)
		return 1
	case ForceConflict:
		// appspec/02's exit-code table colors every fatal exit-1 diagnostic,
		// this one included; the verbatim wording contract lives inside the
		// SGR wrapper.
		fmt.Fprintln(stderr, fatalError.paint(forceConflictLine))
		return 1
	case Command:
		return runCommand(inv, stdout, stderr)
	}
	// Parse returns a closed set; a new Invocation shape without a case here
	// is a programming error, and it must not fail silently.
	// [LAW:no-silent-failure]
	panic("cli: unhandled invocation shape")
}

// runCommand is dispatch steps 2–3 of appspec/01 §4: every command except
// --help/--version passes the config-load gate before its subcommand runs.
// The environment is read here — the process boundary — so config.Load stays
// a function of values. [LAW:effects-at-boundaries]
func runCommand(cmd Command, stdout, stderr io.Writer) int {
	env := config.Env{
		Home:          os.Getenv("HOME"),
		MackupConfig:  os.Getenv("MACKUP_CONFIG"),
		XDGConfigHome: os.Getenv("XDG_CONFIG_HOME"),
	}
	// Guarded config failures arrive as errors and render through the single
	// fatal enforcer; unguarded ones panic inside Load and stay uncaught — the
	// appspec/01 §6 regime split, carried by mechanism.
	if _, err := config.Load(env, cmd.ConfigFile); err != nil {
		return fatal(stderr, err.Error())
	}
	return fatal(stderr, "the "+cmd.Verb.String()+" command is not implemented yet")
}

// fatal writes one guarded fatal diagnostic — the `Error: …` line that
// appspec/07 routes to stderr in bright red — and returns the guarded exit
// code. Every clean fatal exit goes through here so the shape (prefix, color,
// stream, code) cannot drift between sites. [LAW:single-enforcer]
func fatal(stderr io.Writer, msg string) int {
	fmt.Fprintln(stderr, fatalError.paint("Error: "+msg))
	return 1
}
