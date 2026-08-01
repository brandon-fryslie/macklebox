// Package syncops is the per-file executor of appspec/06: backup and restore as
// ONE operation parameterized by a direction record, plus the shared machinery
// every sync command rides on — the two-level sorted fan-out, the Mackup-folder
// gate, the one confirmation mechanism, dry-run/verbose, and the partial-failure
// contract. The link commands (a later epic) plug into this same fan-out and
// gating. [LAW:one-type-per-behavior]
//
// It is the effect boundary for sync: filesystem mutations go through
// internal/fileops, comparison through internal/drift, and all human output
// through the injected streams colored by internal/color. [LAW:effects-at-boundaries]
package syncops

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/brandon-fryslie/macklebox/internal/appdb"
	"github.com/brandon-fryslie/macklebox/internal/color"
	"github.com/brandon-fryslie/macklebox/internal/drift"
	"github.com/brandon-fryslie/macklebox/internal/fileops"
	"github.com/brandon-fryslie/macklebox/internal/homepath"
)

// Options carries the two run-mode booleans (appspec/01 §3).
type Options struct {
	DryRun  bool
	Verbose bool
}

// direction is the record that turns one algorithm into backup and restore
// (appspec/06). Everything that differs between the two lives here — beyond
// {direction, wording, the one link-skip}, any divergence is a defect.
type direction struct {
	name         string // "Backup" / "Restore" — the partial-failure summary verb
	verb         string // progress verb: "Backing up" / "Recovering"
	driftPhr     string // drift header phrasing: "home and Mackup" / "Mackup and home"
	destNoun     string // destination-location noun: "the Mackup folder" / "your home folder"
	forceHint    bool   // backup appends "(use --force to skip this prompt)"
	linkSkip     bool   // backup skips a source already linked into Mackup
	sourceIsHome bool   // source = home path (backup) or mackup path (restore)
}

var backupDir = direction{
	name: "Backup", verb: "Backing up", driftPhr: "home and Mackup",
	destNoun: "the Mackup folder", forceHint: true, linkSkip: true, sourceIsHome: true,
}

var restoreDir = direction{
	name: "Restore", verb: "Recovering", driftPhr: "Mackup and home",
	destNoun: "your home folder", forceHint: false, linkSkip: false, sourceIsHome: false,
}

// engine holds the resolved inputs for one run of the fan-out. opName is the
// operation's partial-failure summary verb; dir carries the copy operations'
// direction record and is unused by link install, whose per-file procedure does
// not read it.
type engine struct {
	opName   string
	dir      direction
	home     string
	mackup   string // the Mackup folder: <storage-root>/<directory>
	db       appdb.Database
	scope    []string // application keys, already resolved and sorted
	opts     Options
	conf     *Confirmer
	stdout   io.Writer
	stderr   io.Writer
	failures []string // "<src> to <dst>" for each per-file failure
}

// Backup and Restore are the two copy directions; LinkInstall moves home files
// into the Mackup folder and symlinks them back. All three take the resolved
// home and Mackup folder, the application database, the already-resolved and
// sorted scope (a single named key or the configured set), the run options, the
// confirmer, and the streams. The exit code is 0 on a complete run, 1 when any
// file failed (appspec/06 partial-failure) or the folder gate declined.
func Backup(home, mackupFolder string, db appdb.Database, scope []string, opts Options, conf *Confirmer, stdout, stderr io.Writer) int {
	e := newEngine("Backup", backupDir, home, mackupFolder, db, scope, opts, conf, stdout, stderr)
	return e.execute(ensureFolder, e.copyFile)
}

func Restore(home, mackupFolder string, db appdb.Database, scope []string, opts Options, conf *Confirmer, stdout, stderr io.Writer) int {
	e := newEngine("Restore", restoreDir, home, mackupFolder, db, scope, opts, conf, stdout, stderr)
	return e.execute(requireFolder, e.copyFile)
}

func LinkInstall(home, mackupFolder string, db appdb.Database, scope []string, opts Options, conf *Confirmer, stdout, stderr io.Writer) int {
	e := newEngine("Link install", direction{}, home, mackupFolder, db, scope, opts, conf, stdout, stderr)
	return e.execute(ensureFolder, e.linkInstallFile)
}

// Link is the join-an-existing-sync path (appspec/06 "link"): symlink files that
// already live in the Mackup folder into home, moving nothing out of home. It
// requires the Mackup folder to already exist.
func Link(home, mackupFolder string, db appdb.Database, scope []string, opts Options, conf *Confirmer, stdout, stderr io.Writer) int {
	e := newEngine("Link", direction{}, home, mackupFolder, db, scope, opts, conf, stdout, stderr)
	return e.execute(requireFolder, e.linkFile)
}

// LinkUninstall reverts links back to real files (appspec/06 "link uninstall"):
// each genuine symlink into Mackup is replaced by a real copy of the Mackup
// content, while a foreign/user-substituted file at the home path is left
// untouched with a warning. It requires the Mackup folder to already exist.
func LinkUninstall(home, mackupFolder string, db appdb.Database, scope []string, opts Options, conf *Confirmer, stdout, stderr io.Writer) int {
	e := newEngine("Link uninstall", direction{}, home, mackupFolder, db, scope, opts, conf, stdout, stderr)
	return e.execute(requireFolder, e.linkUninstallFile)
}

func newEngine(opName string, dir direction, home, mackupFolder string, db appdb.Database, scope []string, opts Options, conf *Confirmer, stdout, stderr io.Writer) *engine {
	return &engine{
		opName: opName, dir: dir, home: home, mackup: mackupFolder, db: db,
		scope: scope, opts: opts, conf: conf, stdout: stdout, stderr: stderr,
	}
}

// gate is the Mackup-folder environment gate (appspec/06): ensure the folder
// (create-on-confirm) or require it. It returns ok=false with a rendered exit
// code when the gate fails.
type gate func(e *engine) (ok bool, code int)

// execute is the shared shape of every sync command: pass the folder gate, run
// perFile over the two-level sorted fan-out, then emit the partial-failure
// summary and exit code. The per-file procedure is the one part that varies.
// [LAW:one-type-per-behavior]
func (e *engine) execute(g gate, perFile func(rel string)) int {
	if ok, code := g(e); !ok {
		return code
	}
	// appspec/01 §1: applications in sorted key order, files within each in
	// sorted order (scope is pre-sorted; Files() is sorted); each handled
	// independently.
	for _, key := range e.scope {
		app, ok := e.db.Lookup(key)
		if !ok {
			// Scope keys are drawn from the database, so a miss is a broken
			// invariant, not a routine case — fail loudly rather than silently
			// dropping the application's files. [LAW:no-silent-failure]
			panic("syncops: scope key not in the application database: " + key)
		}
		for _, rel := range app.Files() {
			perFile(rel)
		}
	}
	return e.finish()
}

// copyFile is the shared per-file procedure of appspec/06 §"The shared per-file
// procedure", identical for backup and restore save for the direction record.
func (e *engine) copyFile(rel string) {
	src := e.sourcePath(rel)
	dst := e.destPath(rel)

	// 1. Source absent as a real file/dir → skip silently.
	if !existsFileOrDir(src) {
		return
	}
	// 2. Backup-only link-skip: source already a symlink into Mackup → skip.
	if e.dir.linkSkip && fileops.AlreadyLinked(src, dst) {
		e.trace("Skipping " + rel + ", already linked to " + dst)
		return
	}
	// 3. A copy exists at the destination → drift-compare.
	if fileops.PathExists(dst) {
		cmp := drift.Compare(src, dst)
		if cmp.Identical {
			e.trace(rel + " already in sync, skipping")
			return
		}
		e.progress(rel, src, dst)
		if e.opts.DryRun {
			return // dry-run: the progress line, no diff/prompt/mutation
		}
		if cmp.Detail != "" {
			fmt.Fprintln(e.stdout, color.Anomaly.Paint(rel+" differs between "+e.dir.driftPhr+":"))
			// Fprintln (not Fprint): some drift details are a single line with no
			// trailing newline, and the prompt writes to the same stream — without
			// this the prompt would collide onto the detail's line.
			fmt.Fprintln(e.stdout, color.Info.Paint(cmp.Detail))
		}
		prompt := fmt.Sprintf("A %s named %s already exists in %s. Are you sure that you want to replace it?",
			pathKind(dst), dst, e.dir.destNoun)
		if e.dir.forceHint {
			prompt += " (use --force to skip this prompt)"
		}
		yes, err := e.conf.ask(prompt)
		if err != nil {
			panic(err) // appspec/07: end-of-input at a prompt is unguarded
		}
		if !yes {
			return
		}
		if err := e.replace(src, dst); err != nil {
			e.recordFailure(src, dst, err)
		}
		return
	}
	// 4. No copy at the destination → progress line, then copy directly.
	e.progress(rel, src, dst)
	if e.opts.DryRun {
		return
	}
	if err := fileops.Copy(src, dst); err != nil {
		e.recordFailure(src, dst, err)
	}
}

// linkInstallFile is the per-file procedure of appspec/06 "link install": move
// a real home file into the Mackup folder and replace it with a symlink. It acts
// only on a home path that exists as a real file/dir and is not already linked
// into Mackup (the shared predicate), so an already-linked file is skipped and
// the command is idempotent.
func (e *engine) linkInstallFile(rel string) {
	home := filepath.Join(e.home, rel)
	mackup := filepath.Join(e.mackup, rel)

	// 1. Act only on real, not-already-linked home content.
	if !existsFileOrDir(home) || fileops.AlreadyLinked(home, mackup) {
		e.linkInstallTrace(home, mackup)
		return
	}
	// 2. Progress.
	if e.opts.Verbose {
		fmt.Fprintln(e.stdout, color.Info.Paint("Backing up\n  "+home+"\n  to\n  "+mackup+" ..."))
	} else {
		fmt.Fprintln(e.stdout, color.Info.Paint("Linking "+rel+" ..."))
	}
	if e.opts.DryRun {
		return // dry-run stops after the progress line
	}

	// 3. If a backup copy already exists, confirm replacing it with the home
	//    content; otherwise copy the home content in fresh. Unlike backup and
	//    restore, the link family does not aggregate failures — a failure inside
	//    a link operation is an uncaught error that stops the run (appspec/06
	//    partial-failure contract, the deliberate honesty-of-failure asymmetry).
	if fileops.PathExists(mackup) {
		yes, err := e.conf.ask(fmt.Sprintf(
			"A %s named %s already exists in the backup. Are you sure that you want to replace it?",
			pathKind(mackup), mackup))
		if err != nil {
			panic(err)
		}
		if !yes {
			return
		}
		if err := e.replace(home, mackup); err != nil {
			panic("link install: cannot replace the backup copy " + mackup + ": " + err.Error())
		}
	} else if err := fileops.Copy(home, mackup); err != nil {
		panic("link install: cannot copy " + home + " into the Mackup folder: " + err.Error())
	}

	// The content now lives in Mackup. Turn the home path into a symlink to it:
	// delete the home file, then link. This copy → delete-home → symlink order is
	// appspec/06's one documented non-atomic window (appspec/01 §2, appspec/07
	// crash residue): an interruption between the delete and the link leaves the
	// home path missing while the content survives in Mackup (StateMackupOnly).
	// Re-running `link` recovers it (re-links from the surviving Mackup copy);
	// link install itself acts only on real home content (step 1 above), so it
	// does not re-link a mackup-only file.
	if err := fileops.Delete(home); err != nil {
		panic("link install: cannot remove the home file " + home + ": " + err.Error())
	}
	if err := fileops.Link(mackup, home); err != nil {
		panic("link install: cannot create the symlink " + home + ": " + err.Error())
	}
}

// linkInstallTrace prints the verbose "Doing nothing" trace keyed on the file's
// LinkState (appspec/06 link-install step 1). It is silent unless verbose.
func (e *engine) linkInstallTrace(home, mackup string) {
	if !e.opts.Verbose {
		return
	}
	switch fileops.State(home, mackup) {
	case fileops.StateAlreadyLinked:
		e.trace("Doing nothing, " + home + " is already linked to " + mackup)
	case fileops.StateBrokenLink:
		e.trace("Doing nothing, " + home + " is a broken link")
	case fileops.StateMackupOnly:
		// The crash-window residue: home is gone but the content survives in
		// Mackup. Say so, so verbose output does not imply the data is lost.
		e.trace("Doing nothing, " + home + " does not exist (its content is in the Mackup folder; run 'link' to recover)")
	default:
		e.trace("Doing nothing, " + home + " does not exist")
	}
}

// linkFile is the per-file procedure of appspec/06 "link": symlink a file that
// already exists in the Mackup folder into home, moving nothing out of home
// (the second-machine door, distinct by contract from link install). Unlike the
// copy family, a link failure is uncaught and stops the run (appspec/06
// partial-failure).
func (e *engine) linkFile(rel string) {
	home := filepath.Join(e.home, rel)
	mackup := filepath.Join(e.mackup, rel)

	// 1. Act only if the Mackup copy exists as real content, home is not already
	//    our link, and the platform permits syncing this path.
	if !existsFileOrDir(mackup) || fileops.AlreadyLinked(home, mackup) || !linkAllowedOnPlatform(runtime.GOOS, e.home, home) {
		e.trace("Doing nothing for " + home)
		return
	}
	// 2. Progress.
	if e.opts.Verbose {
		fmt.Fprintln(e.stdout, color.Info.Paint("Restoring\n  linking "+home+"\n  to      "+mackup+" ..."))
	} else {
		fmt.Fprintln(e.stdout, color.Info.Paint("Restoring "+rel+" ..."))
	}
	if e.opts.DryRun {
		return
	}
	// 3. If anything is at the home path, confirm replacing it. On yes, remove it
	//    first so the symlink can be created in its place.
	if fileops.PathExists(home) {
		yes, err := e.conf.ask(fmt.Sprintf(
			"You already have a %s at %s. Do you want to replace it with your backup?",
			pathKind(home), home))
		if err != nil {
			panic(err)
		}
		if !yes {
			return
		}
		if err := fileops.Delete(home); err != nil {
			panic("link: cannot remove the existing home path " + home + ": " + err.Error())
		}
	}
	// 3/4. Create the symlink into the (unmodified) Mackup copy.
	if err := fileops.Link(mackup, home); err != nil {
		panic("link: cannot create the symlink " + home + ": " + err.Error())
	}
}

// linkUninstallFile is the per-file procedure of appspec/06 "link uninstall",
// the inverse of link install: revert a genuine symlink into Mackup back to a
// real home file, while protecting a foreign file the user substituted at the
// home path. A home path that only Mackup holds (no home entry) is left
// storage-only — this pass reverts existing links, it does not create new home
// copies. Like all link operations, a failure is uncaught and stops the run.
func (e *engine) linkUninstallFile(rel string) {
	home := filepath.Join(e.home, rel)
	mackup := filepath.Join(e.mackup, rel)

	// 1. Act only if the Mackup copy exists.
	if !existsFileOrDir(mackup) {
		e.trace("Doing nothing, " + mackup + " does not exist")
		return
	}
	// 3. Nothing at the home path → leave the file storage-only.
	if !fileops.PathExists(home) {
		return
	}
	// 2. A home entry that is not our live link is a foreign/user-substituted
	//    file: warn (to STDOUT — appspec/06 stream note) and skip, so the user's
	//    own file is never clobbered (appspec/00 promise 10, reversibility).
	if !fileops.AlreadyLinked(home, mackup) {
		fmt.Fprintln(e.stdout, color.Anomaly.Paint(fmt.Sprintf(
			"Warning: the file in your home %q does not point to the original file in Mackup %s, skipping...",
			home, mackup)))
		return
	}
	// A genuine link → revert it.
	if e.opts.Verbose {
		fmt.Fprintln(e.stdout, color.Info.Paint("Reverting "+mackup+"\n at "+home+" ..."))
	} else {
		fmt.Fprintln(e.stdout, color.Info.Paint("Reverting "+rel+" ..."))
	}
	if e.opts.DryRun {
		return
	}
	// Delete the home symlink, then copy the Mackup content back as a real file.
	if err := fileops.Delete(home); err != nil {
		panic("link uninstall: cannot remove the home symlink " + home + ": " + err.Error())
	}
	if err := fileops.Copy(mackup, home); err != nil {
		panic("link uninstall: cannot copy " + mackup + " back to " + home + ": " + err.Error())
	}
}

// linkAllowedOnPlatform applies appspec/06's platform rule, stated in the link
// section: on Linux a home path under ~/Library/ is not linked (skipped); macOS
// has no such restriction. The rule is specific to link because link is the one
// command driven by the Mackup copy — which exists — so on Linux it could
// otherwise create a symlink under ~/Library, a directory that is meaningless
// there. Backup, restore, and link install are driven by the home/source file,
// which does not exist under ~/Library on Linux, so their existence guard skips
// those paths without a platform rule. The platform is a parameter so the rule
// is testable without the host's GOOS, and it reuses the shared containment
// predicate (Library is just another base here).
func linkAllowedOnPlatform(goos, homeDir, homePath string) bool {
	if goos != "linux" {
		return true
	}
	return !homepath.WithinHome(filepath.Join(homeDir, "Library"), homePath)
}

// replace overwrites dst with a copy of src using replace (not merge) semantics,
// so that dst is never observable as missing — appspec/07's "no filesystem
// change for the failing operation," made total rather than merely likely.
//
// The failure-prone copy is staged in a unique temp directory under dst's parent
// (same filesystem, so every rename below is atomic) while dst stays intact.
// Then dst is moved aside and the staged copy moved into place. If the second
// rename fails, the old copy is rolled back so dst reappears; if even the
// rollback fails — two same-directory renames failing in succession — the old
// copy is left on disk with a clear error rather than destroyed. At every point
// dst is present as the old copy, the new copy, or the restored old copy.
func (e *engine) replace(src, dst string) error {
	staging, err := os.MkdirTemp(filepath.Dir(dst), ".mackup-staging-*")
	if err != nil {
		return err
	}

	staged := filepath.Join(staging, "new")
	if err := fileops.Copy(src, staged); err != nil {
		e.cleanupStaging(staging)
		return err // dst untouched
	}

	aside := filepath.Join(staging, "old")
	if err := os.Rename(dst, aside); err != nil {
		e.cleanupStaging(staging)
		return err // could not move the old copy aside; dst untouched
	}
	if err := os.Rename(staged, dst); err != nil {
		if rbErr := os.Rename(aside, dst); rbErr != nil {
			// Do not remove staging: aside holds the only copy of the old dst.
			return fmt.Errorf("replace failed and rollback failed; old copy preserved at %s: %w (rollback: %v)", aside, err, rbErr)
		}
		e.cleanupStaging(staging)
		return err
	}
	e.cleanupStaging(staging) // success: discard the old copy
	return nil
}

// cleanupStaging removes the temporary staging directory. A failure here does
// not affect the operation's outcome (the replace already succeeded or was
// rolled back), but it leaves an orphaned .mackup-staging-* directory in the
// user's folder, so it is surfaced as a non-fatal warning rather than
// discarded. [LAW:no-silent-failure]
func (e *engine) cleanupStaging(staging string) {
	if err := os.RemoveAll(staging); err != nil {
		fmt.Fprintln(e.stderr, color.Anomaly.Paint(
			"Warning: could not remove staging directory "+staging+": "+err.Error()))
	}
}

func (e *engine) sourcePath(rel string) string {
	if e.dir.sourceIsHome {
		return filepath.Join(e.home, rel)
	}
	return filepath.Join(e.mackup, rel)
}

func (e *engine) destPath(rel string) string {
	if e.dir.sourceIsHome {
		return filepath.Join(e.mackup, rel)
	}
	return filepath.Join(e.home, rel)
}

// progress prints the progress line — short by default, or the full multi-line
// source/destination form under verbose (appspec/06, appspec/01 §3).
func (e *engine) progress(rel, src, dst string) {
	if e.opts.Verbose {
		fmt.Fprintln(e.stdout, color.Info.Paint(e.dir.verb+"\n  "+src+"\n  to\n  "+dst+" ..."))
		return
	}
	fmt.Fprintln(e.stdout, color.Info.Paint(e.dir.verb+" "+rel+" ..."))
}

// trace prints a verbose-only skip trace; it is silent otherwise. Verbose is
// observationally pure — it changes only output (appspec/01 §3).
func (e *engine) trace(msg string) {
	if e.opts.Verbose {
		fmt.Fprintln(e.stdout, color.Trace.Paint(msg))
	}
}

// recordFailure logs a per-file copy failure to stderr and records the path, so
// the loop continues and the run ends non-zero with a summary (appspec/06
// partial-failure contract). Failures flow as data, never as control flow.
// [LAW:no-silent-failure]
func (e *engine) recordFailure(src, dst string, err error) {
	fmt.Fprintln(e.stderr, color.CopyFailure.Paint(
		fmt.Sprintf("Error: Unable to copy %s to %s: %s", src, dst, err)))
	e.failures = append(e.failures, src+" to "+dst)
}

// finish emits the end-of-run partial-failure summary and returns the exit code:
// 0 for a complete run, 1 when any file failed.
func (e *engine) finish() int {
	if len(e.failures) == 0 {
		return 0
	}
	fmt.Fprintln(e.stderr, color.CopyFailure.Paint(
		fmt.Sprintf("%s incomplete: %d file(s) could not be copied:", e.opName, len(e.failures))))
	for _, f := range e.failures {
		fmt.Fprintln(e.stderr, color.CopyFailure.Paint("  "+f))
	}
	return 1
}

// ensureFolder is backup's gate: create the Mackup folder on confirmation if it
// is absent. The folder-creation decision is NOT suppressed by dry-run — it is
// an environment gate, not a per-file mutation (appspec/01 §3).
func ensureFolder(e *engine) (bool, int) {
	if homepath.IsDir(e.mackup) {
		return true, 0
	}
	yes, err := e.conf.ask("Mackup needs a directory to store your configuration files / Do you want to create it now? " + e.mackup)
	if err != nil {
		panic(err)
	}
	if !yes {
		return false, e.fatal("Mackup can't do anything without a home =(")
	}
	if err := os.MkdirAll(e.mackup, 0o700); err != nil {
		// A permission or disk error creating the folder is a comprehensible
		// failure, not a programming error — render it as a clean diagnostic.
		return false, e.fatal("cannot create the Mackup folder '" + e.mackup + "': " + err.Error())
	}
	return true, 0
}

// requireFolder is restore's gate: the Mackup folder must already exist.
func requireFolder(e *engine) (bool, int) {
	if homepath.IsDir(e.mackup) {
		return true, 0
	}
	return false, e.fatal("Unable to find the Mackup folder: " + e.mackup +
		"\nYou might want to back up some files or get your Mackup folder synced first.")
}

// fatal renders one guarded "Error: …" diagnostic (appspec/07) to stderr in the
// fatal color and returns exit 1, mirroring the cli renderer over the shared
// color scheme.
func (e *engine) fatal(msg string) int {
	fmt.Fprintln(e.stderr, color.FatalError.Paint("Error: "+msg))
	return 1
}

// existsFileOrDir reports whether src is present as a regular file or directory
// (appspec/06 step 1). It follows symlinks — a source symlink to real content
// counts, matching the copy primitive.
func existsFileOrDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && (info.Mode().IsRegular() || info.IsDir())
}

// pathKind is the noun appspec/07 uses for an existing destination: "link" for a
// symlink, "folder" for a directory, "file" otherwise.
func pathKind(p string) string {
	info, err := os.Lstat(p)
	switch {
	case err != nil:
		return "file"
	case info.Mode()&os.ModeSymlink != 0:
		return "link"
	case info.IsDir():
		return "folder"
	default:
		return "file"
	}
}
