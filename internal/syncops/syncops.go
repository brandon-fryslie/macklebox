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

// engine holds the resolved inputs for one run of the fan-out.
type engine struct {
	dir      direction
	home     string
	mackup   string // the Mackup folder: <storage-root>/<directory>
	db       appdb.Database
	scope    []string // application keys, already resolved and sorted
	opts     Options
	conf     *Confirmer
	stdout   io.Writer
	stderr   io.Writer
	failures []string // "<src> to <dst>" for each per-file copy failure
}

// Backup runs the backup direction; Restore the restore direction. Both take the
// resolved home and Mackup folder, the application database, the already-resolved
// and sorted scope (a single named key or the configured set), the run options,
// the confirmer, and the streams. The exit code is 0 on a complete run, 1 when
// any file could not be copied (appspec/06 partial-failure) or the folder gate
// declined.
func Backup(home, mackupFolder string, db appdb.Database, scope []string, opts Options, conf *Confirmer, stdout, stderr io.Writer) int {
	return run(backupDir, home, mackupFolder, db, scope, opts, conf, stdout, stderr, ensureFolder)
}

func Restore(home, mackupFolder string, db appdb.Database, scope []string, opts Options, conf *Confirmer, stdout, stderr io.Writer) int {
	return run(restoreDir, home, mackupFolder, db, scope, opts, conf, stdout, stderr, requireFolder)
}

// gate is the Mackup-folder environment gate for the direction (appspec/06):
// backup ensures the folder (create-on-confirm), restore requires it. It returns
// ok=false with a rendered exit code when the gate fails.
type gate func(e *engine) (ok bool, code int)

func run(dir direction, home, mackupFolder string, db appdb.Database, scope []string, opts Options, conf *Confirmer, stdout, stderr io.Writer, g gate) int {
	e := &engine{
		dir: dir, home: home, mackup: mackupFolder, db: db, scope: scope,
		opts: opts, conf: conf, stdout: stdout, stderr: stderr,
	}
	if ok, code := g(e); !ok {
		return code
	}
	// Two-level sorted fan-out (appspec/01 §1): applications in sorted key
	// order, files within each in sorted order. scope is pre-sorted; Files() is
	// sorted. Each file is handled independently.
	for _, key := range e.scope {
		app, ok := e.db.Lookup(key)
		if !ok {
			// Scope keys are drawn from the database, so a miss is a broken
			// invariant, not a routine case — fail loudly rather than silently
			// dropping the application's files. [LAW:no-silent-failure]
			panic("syncops: scope key not in the application database: " + key)
		}
		for _, rel := range app.Files() {
			e.perFile(rel)
		}
	}
	return e.finish()
}

// perFile is the shared per-file procedure of appspec/06 §"The shared per-file
// procedure", identical for backup and restore save for the direction record.
func (e *engine) perFile(rel string) {
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
			fmt.Fprint(e.stdout, color.Info.Paint(cmp.Detail))
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
		if err := replace(src, dst); err != nil {
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
func replace(src, dst string) error {
	staging, err := os.MkdirTemp(filepath.Dir(dst), ".mackup-staging-*")
	if err != nil {
		return err
	}

	staged := filepath.Join(staging, "new")
	if err := fileops.Copy(src, staged); err != nil {
		os.RemoveAll(staging)
		return err // dst untouched
	}

	aside := filepath.Join(staging, "old")
	if err := os.Rename(dst, aside); err != nil {
		os.RemoveAll(staging)
		return err // could not move the old copy aside; dst untouched
	}
	if err := os.Rename(staged, dst); err != nil {
		if rbErr := os.Rename(aside, dst); rbErr != nil {
			// Do not remove staging: aside holds the only copy of the old dst.
			return fmt.Errorf("replace failed and rollback failed; old copy preserved at %s: %w (rollback: %v)", aside, err, rbErr)
		}
		os.RemoveAll(staging)
		return err
	}
	os.RemoveAll(staging) // success: discard the old copy
	return nil
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
		fmt.Sprintf("%s incomplete: %d file(s) could not be copied:", e.dir.name, len(e.failures))))
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
