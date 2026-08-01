// Package homepath holds the home-directory path primitives shared by config
// (appspec/03, appspec/04) and appdb (appspec/05). Its containment predicate is
// the single authoritative answer to "is this path within the home directory" —
// the check the sync engine's safety leans on (appspec/05 "home-relativity
// guarantee") and the one whose earlier hand-rolled prefix variants produced
// the trailing-slash and separator-grammar bugs. One predicate, one place.
// [LAW:single-enforcer]
package homepath

import (
	"os"
	"path/filepath"
	"strings"
)

// WithinHome reports whether p is home itself or a descendant of it. The
// comparison is lexical on the given paths via filepath.Rel: p is within home
// unless the relative route leaves home (exactly "..", or a route that starts
// with "../"). A sibling like /home/bobby against home /home/bob leaves home
// and is correctly rejected, where a raw string-prefix check would not.
// Callers pass already-cleaned paths.
func WithinHome(home, p string) bool {
	rel, err := filepath.Rel(home, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// XDGBase resolves the XDG config base directory: $XDG_CONFIG_HOME when set,
// otherwise ~/.config (appspec/03 discovery, appspec/05 discovery and XDG
// relativization). [LAW:one-source-of-truth]
func XDGBase(home, xdgConfigHome string) string {
	if xdgConfigHome != "" {
		return xdgConfigHome
	}
	return filepath.Join(home, ".config")
}

// IsRegularFile reports whether p exists and is a regular file. Any stat error
// (including a nonexistent path) reads as "not a regular file".
func IsRegularFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}
