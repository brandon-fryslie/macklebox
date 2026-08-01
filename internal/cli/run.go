package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/brandon-fryslie/macklebox/internal/appdb"
	"github.com/brandon-fryslie/macklebox/internal/catalog"
	"github.com/brandon-fryslie/macklebox/internal/color"
	"github.com/brandon-fryslie/macklebox/internal/config"
	"github.com/brandon-fryslie/macklebox/internal/homepath"
	"github.com/brandon-fryslie/macklebox/internal/syncops"
)

// forceConflictLine is the exact stderr line appspec/02 "Mutually exclusive
// force flags" specifies, verbatim.
const forceConflictLine = "Options --force and --force-no are mutually exclusive."

// Run executes one invocation against the given streams and returns the
// process exit code. It is the whole dispatch pipeline of appspec/01 §4:
// parse → config load → subcommand. Streams and exit code are parameters so
// the entire boundary contract is testable in-process.
// [LAW:effects-at-boundaries]
func Run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	switch inv := Parse(argv).(type) {
	case Help:
		fmt.Fprint(stdout, helpText)
		return 0
	case Version:
		// Info-level per appspec/07: colored unconditionally, even piped.
		fmt.Fprintln(stdout, color.Info.Paint("Mackup "+versionString()))
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
		return runCommand(inv, stdin, stdout, stderr)
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
func runCommand(cmd Command, stdin io.Reader, stdout, stderr io.Writer) int {
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
	// Step 5. list/show have no folder gate; backup/restore run the sync engine,
	// whose Mackup-folder gate is inside syncops. A named application is
	// validated here, before that gate (appspec/01 §3, appspec/06).
	switch cmd.Verb {
	case VerbList:
		return runList(db, stdout)
	case VerbShow:
		return runShow(db, cmd.App, stdout, stderr)
	case VerbBackup, VerbRestore, VerbLinkInstall, VerbLink, VerbLinkUninstall:
		scope, ok := resolveScope(cmd.App, cfg, db, stderr)
		if !ok {
			return 1 // an unknown named application (message already written)
		}
		conf := syncops.NewConfirmer(cmd.Confirm, stdin, stdout)
		opts := syncops.Options{DryRun: cmd.DryRun, Verbose: cmd.Verbose}
		home := filepath.Clean(env.Home)
		folder := cfg.MackupFolder()
		switch cmd.Verb {
		case VerbBackup:
			return syncops.Backup(home, folder, db, scope, opts, conf, stdout, stderr)
		case VerbRestore:
			return syncops.Restore(home, folder, db, scope, opts, conf, stdout, stderr)
		case VerbLinkInstall:
			return syncops.LinkInstall(home, folder, db, scope, opts, conf, stdout, stderr)
		case VerbLink:
			return syncops.Link(home, folder, db, scope, opts, conf, stdout, stderr)
		case VerbLinkUninstall:
			return syncops.LinkUninstall(home, folder, db, scope, opts, conf, stdout, stderr)
		default:
			// The outer case admits exactly the five verbs above; a sixth added
			// there without a dispatch here must fail loudly, not run the wrong
			// operation. [LAW:no-silent-failure]
			panic("cli: sync verb without a dispatch: " + cmd.Verb.String())
		}
	default:
		return fatal(stderr, "the "+cmd.Verb.String()+" command is not implemented yet")
	}
}

// resolveScope applies the appspec/01 §3 selector: a named application replaces
// the configured scope with exactly that key, overriding both the allow and
// ignore lists; otherwise the scope is the configured allow-minus-ignore set
// over all keys. A named application is validated here — an unknown name writes
// "Unsupported application" and returns ok=false before any folder gate or
// prompt (appspec/06). The configured set is already sorted (db.Keys is sorted).
func resolveScope(app string, cfg config.Config, db appdb.Database, stderr io.Writer) ([]string, bool) {
	if app != "" {
		if _, known := db.Lookup(app); !known {
			unsupportedApp(stderr, app)
			return nil, false
		}
		return []string{app}, true
	}
	return cfg.Scope(db.Keys()), true
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
	if !homepath.IsDir(cfg.Root()) {
		return fmt.Errorf("Unable to find the storage folder: %s", cfg.Root())
	}
	return nil
}

// fatalLine renders one fatal diagnostic — bright red, on stderr, exit 1 — the
// single rendering path every fatal exit shares (appspec/07). The message text
// is the caller's: the guarded shape goes through fatal, which adds the "Error:"
// prefix; the spec's bare contract-token fatals (the "Unsupported application"
// line, the force-conflict line) pass their exact verbatim wording, which the
// "Error:" prefix would corrupt. Color, stream, and exit code cannot drift
// because they live only here. [LAW:single-enforcer]
func fatalLine(stderr io.Writer, msg string) int {
	fmt.Fprintln(stderr, color.FatalError.Paint(msg))
	return 1
}

// unsupportedApp writes the bare "Unsupported application: <key>" contract line
// (appspec/07 — no "Error:" prefix) and returns exit 1. It is the one renderer
// for the unknown-application failure that show and the sync commands share.
// [LAW:single-enforcer]
func unsupportedApp(stderr io.Writer, key string) int {
	return fatalLine(stderr, "Unsupported application: "+key)
}

// fatal writes one guarded fatal diagnostic — the `Error: …` line that
// appspec/07 routes to stderr in bright red. It is the "Error:"-prefixed
// content variant over fatalLine; the guarded shape lives here, the rendering
// invariant lives in fatalLine. [LAW:single-enforcer]
func fatal(stderr io.Writer, msg string) int {
	return fatalLine(stderr, "Error: "+msg)
}
