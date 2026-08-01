package cli

import (
	"errors"
	"fmt"
	"io"
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
func runCommand(cmd Command, stdout, stderr io.Writer) int {
	if _, err := loadConfig(cmd.ConfigFile); err != nil {
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

// Config will carry the three facts the startup pipeline resolves
// (appspec/01 §4: storage location, storage directory, application scope).
type Config struct{}

// loadConfig is the seam the resolvers epic (macklebox-resolvers-aol) fills.
// Until then it fails loudly: pretending an empty config loaded would let
// every subcommand run against a state the spec says cannot exist.
// [LAW:no-silent-failure] [LAW:locality-or-seam]
func loadConfig(path string) (Config, error) {
	return Config{}, errors.New("config loading is not implemented yet (macklebox-resolvers-aol.1)")
}
