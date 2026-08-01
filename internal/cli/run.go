package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/brandon-fryslie/macklebox/internal/appdb"
	"github.com/brandon-fryslie/macklebox/internal/catalog"
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
		// this one included; its verbatim wording carries no "Error:" prefix, so
		// it renders through fatalLine rather than fatal.
		return fatalLine(stderr, forceConflictLine)
	case Command:
		return runCommand(inv, stdout, stderr)
	}
	// Parse returns a closed set; a new Invocation shape without a case here
	// is a programming error, and it must not fail silently.
	// [LAW:no-silent-failure]
	panic("cli: unhandled invocation shape")
}

// runCommand runs the fixed startup pipeline of appspec/01 §4 for every command
// except --help/--version: resolve config (2), assemble the application
// database (3), pass the universal environment gate (4), then dispatch. All
// ambient state — environment variables and the effective UID — is read here at
// the process boundary, so the steps below stay functions of values.
// [LAW:effects-at-boundaries]
func runCommand(cmd Command, stdout, stderr io.Writer) int {
	env := config.Env{
		Home:          os.Getenv("HOME"),
		MackupConfig:  os.Getenv("MACKUP_CONFIG"),
		XDGConfigHome: os.Getenv("XDG_CONFIG_HOME"),
	}
	// Step 2. Guarded config failures arrive as errors and render through the
	// single fatal enforcer; unguarded ones panic inside Load and stay uncaught
	// — the appspec/01 §6 regime split, carried by mechanism.
	cfg, err := config.Load(env, cmd.ConfigFile)
	if err != nil {
		return fatal(stderr, err.Error())
	}
	// Step 3. Assemble the application database. A bad definition (absolute or
	// escaping path, out-of-home XDG) panics inside Assemble and stays uncaught,
	// aborting every command uniformly (appspec/05, appspec/07).
	db := appdb.Assemble(filepath.Clean(env.Home), env.XDGConfigHome, catalog.FS())
	// Step 4. The universal environment gate — root guard and storage-root
	// existence — which list and show pass identically to every other command.
	if err := checkEnvironment(cfg, cmd.Root, os.Geteuid()); err != nil {
		return fatal(stderr, err.Error())
	}
	// Step 5 has no list/show branch (appspec/01 §4); dispatch straight to them.
	// The remaining verbs' per-command storage gates and operations land in
	// later tickets.
	switch cmd.Verb {
	case VerbList:
		return runList(db, stdout)
	case VerbShow:
		return runShow(db, cmd.App, stdout, stderr)
	default:
		return fatal(stderr, "the "+cmd.Verb.String()+" command is not implemented yet")
	}
}

// checkEnvironment is the universal environment gate of appspec/01 §4 level 1:
// the root guard, then storage-root-directory existence — the point where the
// file_system engine's deferred existence check finally fires (appspec/04).
// Both failures are guarded fatals; a nil return means the environment is
// usable. euid and the storage root are passed in as values. [LAW:single-enforcer]
func checkEnvironment(cfg config.Config, allowRoot bool, euid int) error {
	if euid == 0 && !allowRoot {
		// appspec/07 root guard (guarded). Not exercised in the non-root test
		// harness, but part of the gate every command runs.
		return errors.New("Running as superuser can be dangerous. " +
			"Run 'mackup --help' for guidance, or pass --root to override.")
	}
	if !isDir(cfg.Root()) {
		return fmt.Errorf("Unable to find the storage folder: %s", cfg.Root())
	}
	return nil
}

// isDir reports whether p exists and is a directory.
func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// fatalLine renders one fatal diagnostic — bright red, on stderr, exit 1 — the
// single rendering path every fatal exit shares (appspec/07). The message text
// is the caller's: the guarded shape goes through fatal, which adds the "Error:"
// prefix; the spec's bare contract-token fatals (the "Unsupported application"
// line, the force-conflict line) pass their exact verbatim wording, which the
// "Error:" prefix would corrupt. Color, stream, and exit code cannot drift
// because they live only here. [LAW:single-enforcer]
func fatalLine(stderr io.Writer, msg string) int {
	fmt.Fprintln(stderr, fatalError.paint(msg))
	return 1
}

// fatal writes one guarded fatal diagnostic — the `Error: …` line that
// appspec/07 routes to stderr in bright red. It is the "Error:"-prefixed
// content variant over fatalLine; the guarded shape lives here, the rendering
// invariant lives in fatalLine. [LAW:single-enforcer]
func fatal(stderr io.Writer, msg string) int {
	return fatalLine(stderr, "Error: "+msg)
}
