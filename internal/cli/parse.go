// Package cli implements the command-line boundary specified by
// appspec/02-invocation.md: the invocation grammar, global options, dispatch
// order, and exit codes.
package cli

import (
	"fmt"
	"strings"
)

// Invocation is the parser's entire output: every argv resolves to exactly one
// of these shapes, covering appspec/02 "Invocation forms" and
// "Argument-parser behavior". Downstream code routes on the shape and never
// re-inspects argv. [LAW:types-are-the-program]
type Invocation interface{ invocation() }

// Help is -h / --help: usage to stdout, exit 0, no config read (appspec/02
// global-options table).
type Help struct{}

// Version is --version: "Mackup <version>" to stdout, exit 0, no config read.
type Version struct{}

// ShowUsage is an invocation with no positional subcommand: appspec/02
// "Argument-parser behavior" treats it as a usage display (stdout, exit 0).
type ShowUsage struct{}

// UsageError is argv that matches none of the listed invocation forms: a
// warning identifying the unmatched input plus the usage block, on stderr
// (appspec/07 stream table), nonzero exit.
type UsageError struct{ Warning string }

// ForceConflict is --force combined with --force-no: the exact single stderr
// line and exit 1, before any config read (appspec/02 "Mutually exclusive
// force flags").
type ForceConflict struct{}

// Command is a recognized subcommand with its options. The force flags have
// already been folded into Confirm, so the illegal both-flags state cannot
// reach any consumer. [LAW:types-are-the-program]
type Command struct {
	Verb Verb
	// App is the application key the command is scoped to; "" means the
	// configured set (appspec/02 "Selecting which applications"). Keys are
	// never empty, and the parser guarantees App != "" for VerbShow.
	App        string
	Confirm    ConfirmPolicy
	Root       bool
	DryRun     bool
	Verbose    bool
	ConfigFile string // "" = default discovery per appspec/03
}

func (Help) invocation()          {}
func (Version) invocation()       {}
func (ShowUsage) invocation()     {}
func (UsageError) invocation()    {}
func (ForceConflict) invocation() {}
func (Command) invocation()       {}

// Verb enumerates the seven subcommand shapes of appspec/02.
type Verb int

const (
	VerbList Verb = iota
	VerbShow
	VerbBackup
	VerbRestore
	VerbLinkInstall
	VerbLinkUninstall
	VerbLink
)

func (v Verb) String() string {
	switch v {
	case VerbList:
		return "list"
	case VerbShow:
		return "show"
	case VerbBackup:
		return "backup"
	case VerbRestore:
		return "restore"
	case VerbLinkInstall:
		return "link install"
	case VerbLinkUninstall:
		return "link uninstall"
	case VerbLink:
		return "link"
	}
	return fmt.Sprintf("Verb(%d)", int(v))
}

// ConfirmPolicy is the resolved answer source for yes/no confirmation prompts
// (appspec/07 confirmation policy): ask interactively, or pre-answer every
// prompt one way.
type ConfirmPolicy int

const (
	ConfirmAsk       ConfirmPolicy = iota
	ConfirmAlwaysYes               // --force
	ConfirmAlwaysNo                // --force-no
)

// Parse maps argv (without the program name) to an Invocation. It is a pure
// function — no I/O, no environment — so the whole grammar is testable with
// plain equality. [LAW:effects-at-boundaries]
func Parse(argv []string) Invocation {
	var (
		forceYes, forceNo, root, dryRun, verbose bool

		configFile string
	)

	// Options precede the subcommand (appspec/02: the listed forms are
	// `mackup [options] <subcommand> [args]`). -h/--help and --version
	// short-circuit the moment they are seen, before any other judgment
	// (appspec/01 §4 step 1).
	i := 0
	for i < len(argv) && strings.HasPrefix(argv[i], "-") {
		tok := argv[i]
		switch {
		case tok == "-h" || tok == "--help":
			return Help{}
		case tok == "--version":
			return Version{}
		case tok == "-f" || tok == "--force":
			forceYes = true
		case tok == "--force-no":
			forceNo = true
		case tok == "-r" || tok == "--root":
			root = true
		case tok == "-n" || tok == "--dry-run":
			dryRun = true
		case tok == "-v" || tok == "--verbose":
			verbose = true
		case tok == "-c" || tok == "--config-file":
			if i+1 >= len(argv) {
				return UsageError{Warning: fmt.Sprintf("option %s requires a <path> argument", tok)}
			}
			i++
			configFile = argv[i]
		case strings.HasPrefix(tok, "--config-file="):
			configFile = strings.TrimPrefix(tok, "--config-file=")
		default:
			return UsageError{Warning: "unrecognized option: " + tok}
		}
		i++
	}
	pos := argv[i:]

	if len(pos) == 0 {
		return ShowUsage{}
	}
	// An option-shaped token after the first positional matches no listed
	// invocation form (appspec/02: "reject forms that match none").
	for _, tok := range pos {
		if strings.HasPrefix(tok, "-") {
			return UsageError{Warning: "options must precede the subcommand: " + tok}
		}
	}

	var (
		verb Verb
		app  string
	)
	switch pos[0] {
	case "list":
		verb = VerbList
		if len(pos) > 1 {
			return unmatched(pos[1:])
		}
	case "show":
		verb = VerbShow
		if len(pos) < 2 {
			return UsageError{Warning: "show requires an <application> argument"}
		}
		app = pos[1]
		if len(pos) > 2 {
			return unmatched(pos[2:])
		}
	case "backup", "restore":
		verb = VerbBackup
		if pos[0] == "restore" {
			verb = VerbRestore
		}
		if len(pos) > 1 {
			app = pos[1]
		}
		if len(pos) > 2 {
			return unmatched(pos[2:])
		}
	case "link":
		rest := pos[1:]
		verb = VerbLink
		switch {
		case len(rest) > 0 && rest[0] == "install":
			verb = VerbLinkInstall
			rest = rest[1:]
		case len(rest) > 0 && rest[0] == "uninstall":
			verb = VerbLinkUninstall
			rest = rest[1:]
		}
		if len(rest) > 0 {
			app = rest[0]
			rest = rest[1:]
		}
		if len(rest) > 0 {
			return unmatched(rest)
		}
	default:
		return unmatched(pos)
	}

	// Grammar first, force-flag semantics second: a form that matches nothing
	// is a usage error even when it also carries the conflicting flags.
	if forceYes && forceNo {
		return ForceConflict{}
	}
	confirm := ConfirmAsk
	switch {
	case forceYes:
		confirm = ConfirmAlwaysYes
	case forceNo:
		confirm = ConfirmAlwaysNo
	}

	return Command{
		Verb:       verb,
		App:        app,
		Confirm:    confirm,
		Root:       root,
		DryRun:     dryRun,
		Verbose:    verbose,
		ConfigFile: configFile,
	}
}

func unmatched(args []string) UsageError {
	return UsageError{Warning: "unrecognized arguments: " + strings.Join(args, " ")}
}
