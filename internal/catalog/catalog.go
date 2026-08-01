// Package catalog provides the built-in application definitions that ship with
// the program — the third, lowest-precedence discovery tier of appspec/05. It
// is the injection point the cli pipeline hands to appdb.Assemble.
//
// The catalog is empty until aol.4 ships the definition files; that ticket
// replaces FS's body with an embedded filesystem (go:embed) and changes
// nothing else, because the consumer only needs an fs.FS whose root holds
// <key>.cfg files.
package catalog

import "io/fs"

// FS returns the built-in definitions as an fs.FS whose root holds the
// definition files. Empty for now — an empty root reads as "no built-in
// applications", which appdb.Assemble handles like any empty tier.
func FS() fs.FS { return emptyFS{} }

// emptyFS is an fs.FS with an existing but empty root directory, so
// appdb.Assemble reads zero built-in definitions rather than treating the tier
// as a missing directory. It satisfies fs.ReadDirFS; Open exists only to
// complete the fs.FS interface and is never reached while the root is empty.
type emptyFS struct{}

// ReadDir reports the root as an empty directory and every other path as
// nonexistent — reporting "empty" for a path that does not exist would be a
// silent failure of the fs contract. [LAW:no-silent-failure]
func (emptyFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name != "." {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return nil, nil
}

func (emptyFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}
