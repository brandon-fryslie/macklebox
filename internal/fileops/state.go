// Package fileops is the per-file executor substrate of appspec/06 "Shared
// vocabulary" — the copy/delete/link primitives, the recursive permission clamp,
// the best-effort attribute cleanup, the LinkState branch variable, and the one
// already-linked predicate that four operations share. It is the effect boundary
// for sync: every filesystem mutation the five sync commands perform bottoms out
// here, so the per-command tickets are a procedure over these primitives rather
// than their own I/O. [LAW:effects-at-boundaries]
package fileops

import (
	"errors"
	"io/fs"
	"os"
)

// LinkState is the derived per-file state every sync operation dispatches on
// (appspec/06). It is modelled once, here, rather than re-derived in each
// operation, so all five commands branch on the same closed set.
// [LAW:types-are-the-program]
type LinkState int

const (
	// StateAlreadyLinked — home is a live symlink resolving to the existing
	// mackup copy (AlreadyLinked is true).
	StateAlreadyLinked LinkState = iota
	// StateRealFilePresent — home holds present, non-mackup content: a real
	// file, a real directory, or a live symlink to some other target. The
	// operations treat all three as "something is there that isn't our link".
	StateRealFilePresent
	// StateBrokenLink — home is a symlink that does not resolve (dangling).
	StateBrokenLink
	// StateAbsent — nothing at the home path, and no mackup copy either.
	StateAbsent
	// StateMackupOnly — the mackup copy exists but the home path is absent.
	StateMackupOnly
)

func (s LinkState) String() string {
	switch s {
	case StateAlreadyLinked:
		return "already-linked"
	case StateRealFilePresent:
		return "real-file-present"
	case StateBrokenLink:
		return "broken-link"
	case StateAbsent:
		return "absent"
	case StateMackupOnly:
		return "mackup-only"
	}
	return "LinkState(?)"
}

// AlreadyLinked is the single shared predicate of appspec/01 §2: is the home path
// already a live symlink to its mackup copy? Four operations call this one
// definition — backup (skip), link install (guard), link (guard), link uninstall
// (safety check) — so their skip/guard semantics are identical by construction.
// [LAW:single-enforcer]
//
// Shape table (appspec/06 "already-linked predicate"):
//
//	MUST ACCEPT (true):
//	  home is a symlink, resolves (not dangling), mackup exists, same file
//	MUST REJECT (false, never an error):
//	  home is a real file            | home is a real directory
//	  home is absent                 | home is a dangling symlink
//	  home symlinks to another target| home symlinks to mackup but mackup absent
//	  home and mackup are different files
func AlreadyLinked(homePath, mackupPath string) bool {
	info, err := os.Lstat(homePath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false // absent, or a real file/dir — not a symlink at all
	}
	homeResolved, err := os.Stat(homePath) // follows the link
	if err != nil {
		return false // dangling symlink: the target does not resolve
	}
	mackupInfo, err := os.Stat(mackupPath)
	if err != nil {
		return false // the storage copy is missing
	}
	return os.SameFile(homeResolved, mackupInfo)
}

// State derives the LinkState of one file from its home and mackup paths. It is
// total over the input space; the foreign-live-symlink case (a symlink that
// resolves but not to mackup) is classified as real-file-present — present,
// non-mackup content — which is where appspec/01 §2 groups it (a symlink that
// does not resolve to the storage copy is a conflict, like a real file).
func State(homePath, mackupPath string) LinkState {
	if AlreadyLinked(homePath, mackupPath) {
		return StateAlreadyLinked
	}
	info, err := os.Lstat(homePath)
	if err != nil {
		// Nothing usable at home (absent, per the reference's error-as-false
		// reading). The mackup copy's presence splits absent from mackup-only.
		if pathExists(mackupPath) {
			return StateMackupOnly
		}
		return StateAbsent
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// A symlink that is not our live link. Only a target that genuinely does
		// not exist (ENOENT) is broken-link — a diagnosis of absence. Any other
		// stat failure — a permission or I/O error, or a symlink cycle (ELOOP,
		// which fs.ErrNotExist does not match) — leaves the link present but
		// unresolvable-here; it stays real-file-present on purpose. broken-link
		// invites an operation to skip the path as dangling; real-file-present
		// makes the operation act on it and fail loudly (Copy/Clamp surface the
		// ELOOP or permission error) rather than silently dropping it.
		if _, err := os.Stat(homePath); errors.Is(err, fs.ErrNotExist) {
			return StateBrokenLink
		}
		return StateRealFilePresent
	}
	return StateRealFilePresent // a real file or directory
}

// pathExists reports whether anything (file, directory, or symlink) is present at
// p. It uses Lstat so a symlink counts as present without being followed.
func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
