// Package catalog provides the built-in application definitions that ship with
// the program — the third, lowest-precedence discovery tier of appspec/05. It
// is the injection point the cli pipeline hands to appdb.Assemble.
//
// The definition files under defs/ are project-authored data (appspec/appendix
// records only the keys; this is an MIT clean-room build, so no reference data
// is copied). Every appendix key has a name-only definition — a valid, listable,
// showable entry with an empty file set (appspec/05) — except the hand-authored
// mackup self-definition, which carries the file set whole-Mackup mode needs
// (appspec/06). The defs/ files are the source of truth: later tickets grow file
// sets by editing individual definitions.
package catalog

import (
	"embed"
	"io/fs"
)

//go:embed defs/*.cfg
var defsFS embed.FS

// FS returns the built-in definitions as an fs.FS whose root holds the
// <key>.cfg files, matching the shape appdb.Assemble reads. The embed subtree
// is compiled in, so a missing defs/ root is a build/programming error, not a
// runtime condition — hence the panic rather than a returned error.
func FS() fs.FS {
	sub, err := fs.Sub(defsFS, "defs")
	if err != nil {
		panic("catalog: embedded defs subtree missing: " + err.Error())
	}
	return sub
}
