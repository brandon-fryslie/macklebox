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
		fmt.Fprintf(stdout, "Mackup %s\n", versionString())
		return 0
	case ShowUsage:
		fmt.Fprint(stdout, usageText)
		return 0
	case UsageError:
		fmt.Fprintf(stderr, "mackup: %s\n%s", inv.Warning, usageText)
		return 1
	case ForceConflict:
		fmt.Fprintln(stderr, forceConflictLine)
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
		// Guarded fatal-error shape per appspec/02 exit-code table: a
		// diagnostic line on stderr, exit 1, nothing on stdout.
		fmt.Fprintf(stderr, "Error: %s\n", err)
		return 1
	}
	fmt.Fprintf(stderr, "Error: the %s command is not implemented yet\n", cmd.Verb)
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
