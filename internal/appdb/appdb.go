// Package appdb assembles and enumerates the application database — startup
// pipeline step 3 of appspec/01 §4, specified in appspec/05. The database is a
// map application-key → (display name, home-relative file set) with no
// duplicate keys, assembled fresh from three definition directories under a
// fixed precedence.
//
// It guarantees the two properties the sync engine (appspec/06) relies on
// without re-checking: every file path is home-relative (absolute paths are
// rejected at assembly), and every path keeps its exact case. Those rejections
// are contract, not input hygiene — appspec/05 "home-relativity guarantee":
// weaken them and the engine silently gains the ability to write outside home.
// [LAW:parse-dont-validate]
package appdb

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/brandon-fryslie/macklebox/internal/homepath"
	"github.com/brandon-fryslie/macklebox/internal/ini"
)

// Application is one resolved definition: the display name shown by `show`, and
// the union file set as sorted, case-exact, home-relative paths. The zero value
// is not a valid application — obtain one only through Database.Lookup, so a
// consumer can never fabricate an entry the discovery rules did not produce.
// [LAW:types-are-the-program]
type Application struct {
	name  string
	files []string
}

// Name is the display name from the definition's [application] name key.
func (a Application) Name() string { return a.name }

// Files is the union of the definition's [configuration_files] and
// home-relativized [xdg_configuration_files] entries, sorted ascending. A
// consumer cannot tell an XDG-sourced path from a plain one — they are one
// uniform home-relative type (appspec/05). It hands out a fresh owned slice —
// like Keys() and config.Config.Scope() — so the assembled database, the
// authoritative source, cannot be corrupted through a read accessor.
// [LAW:one-source-of-truth]
func (a Application) Files() []string {
	out := make([]string, len(a.files))
	copy(out, a.files)
	return out
}

// Database maps application key → Application with one deterministic winner per
// key. It is obtained only from Assemble and read only through its lookups, so
// every command and list/show see the same closed set.
type Database struct {
	apps map[string]Application
}

// Keys is the set of all application keys, sorted ascending — what `list`
// prints and counts (appspec/05 Enumeration).
func (d Database) Keys() []string {
	keys := make([]string, 0, len(d.apps))
	for key := range d.apps {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Lookup returns the application for a key, and whether it is known. The bool is
// the discriminator `show` turns into "Unsupported application: <key>" — a
// missing entry is a typed absence, never a fabricated empty Application.
// [LAW:parse-dont-validate]
func (d Database) Lookup(key string) (Application, bool) {
	app, ok := d.apps[key]
	return app, ok
}

// caseExact is the definition-file key normalizer: paths keep their exact case,
// the case-preserving half of the appspec/05 case-policy pair (the reference's
// configparser optionxform = str).
func caseExact(s string) string { return s }

// Assemble builds the database from the three discovery directories of
// appspec/05, highest precedence first: the legacy custom-apps directory
// (~/.mackup/), the XDG custom-apps directory ($XDG_CONFIG_HOME/mackup/
// applications/), and the built-in definitions in builtin. One definition file
// wins per key, decided by filename — an earlier tier's file of a given name
// shadows every later tier's same-named file.
//
// The XDG base is resolved once and required to be within home before any
// definition is read; an out-of-home base is an unguarded fatal, because it
// would let an [xdg_configuration_files] entry escape home (appspec/05).
// [LAW:no-silent-failure]
func Assemble(home, xdgConfigHome string, builtin fs.FS) Database {
	xdgBase := filepath.Clean(homepath.XDGBase(home, xdgConfigHome))
	if !homepath.WithinHome(home, xdgBase) {
		// appspec/07: uncaught error naming the value.
		panic("$XDG_CONFIG_HOME (" + xdgBase + ") must be somewhere within your home directory")
	}

	apps := map[string]Application{}
	// os.DirFS turns each user directory into the same fs.FS the built-in
	// catalog already is, so one loader handles all three tiers and precedence
	// is just the order they are drained in. [LAW:dataflow-not-control-flow]
	loadDir(apps, home, xdgBase, os.DirFS(filepath.Join(home, ".mackup")))
	loadDir(apps, home, xdgBase, os.DirFS(filepath.Join(xdgBase, "mackup", "applications")))
	loadDir(apps, home, xdgBase, builtin)
	return Database{apps: apps}
}

// loadDir reads every *.cfg at the root of fsys and inserts each key not already
// claimed by an earlier (higher-precedence) tier. A directory that does not
// exist is skipped; any other read failure is loud, never swallowed into a
// silently smaller database. [LAW:no-silent-failure]
func loadDir(apps map[string]Application, home, xdgBase string, fsys fs.FS) {
	entries, err := fs.ReadDir(fsys, ".")
	if errors.Is(err, fs.ErrNotExist) {
		return // appspec/05: a discovery directory that does not exist is skipped.
	}
	if err != nil {
		panic("cannot read application-definition directory: " + err.Error())
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".cfg") {
			continue // appspec/05: only *.cfg files directly in the directory count.
		}
		key := strings.TrimSuffix(name, ".cfg")
		if _, taken := apps[key]; taken {
			continue // a higher-precedence tier already won this key.
		}
		text, err := fs.ReadFile(fsys, name)
		if err != nil {
			panic("cannot read application definition '" + name + "': " + err.Error())
		}
		apps[key] = parseDefinition(key, string(text), home, xdgBase)
	}
}

// parseDefinition turns one definition file into an Application: the required
// [application] name, and the union of [configuration_files] and
// home-relativized [xdg_configuration_files] entries as sorted, case-exact,
// home-relative paths. Absolute paths in either section are fatal (appspec/05).
func parseDefinition(key, text, home, xdgBase string) Application {
	secs := ini.Parse(text, caseExact)
	name, ok := secs["application"]["name"]
	if !ok {
		// appspec/05: the [application] name is required. A definition without
		// it cannot satisfy `show`, and defaulting it would be an
		// answer-shaped void. [LAW:no-silent-failure]
		panic("application definition '" + key + "' has no [application] name")
	}

	fileSet := map[string]bool{}
	for path := range secs["configuration_files"] {
		// [configuration_files] entries are authored home-relative and stored
		// verbatim; authored path and stored path are the same value.
		fileSet[requireUnderHome(home, path, path)] = true
	}
	for path := range secs["xdg_configuration_files"] {
		// [xdg_configuration_files] entries are authored relative to the XDG
		// base; render home-relative first, then run the same guarantee check.
		fileSet[requireUnderHome(home, path, xdgToHomeRelative(path, home, xdgBase))] = true
	}

	files := make([]string, 0, len(fileSet))
	for path := range fileSet {
		files = append(files, path)
	}
	sort.Strings(files)
	return Application{name: name, files: files}
}

// requireUnderHome enforces the appspec/05 home-relativity guarantee for one
// file-set entry, and is the single choke point every path from either section
// passes through. [LAW:single-enforcer] It rejects an absolute (leading-'/')
// authored path with the pinned appspec/07 message, then rejects any stored
// path that escapes home once joined to HOME. The second check is not
// redundant with the first: a '..' sequence carries no leading slash yet still
// leaves home (e.g. "../../rachel/data" joined to /home/bob resolves to
// /home/rachel/data), and the sync engine, which never re-checks, would then
// write outside home. filepath.Join resolves the '..' before the containment
// test. Returns the stored home-relative path, case preserved.
func requireUnderHome(home, authored, stored string) string {
	if strings.HasPrefix(authored, "/") {
		// appspec/05 + appspec/07: uncaught, names the offending path.
		panic("Unsupported absolute path: " + authored)
	}
	if !homepath.WithinHome(home, filepath.Join(home, stored)) {
		panic("Unsupported path escapes the home directory: " + authored)
	}
	return stored
}

// xdgToHomeRelative renders an [xdg_configuration_files] entry (authored
// relative to the XDG base) as a home-relative path: join it under the XDG base
// and strip the home prefix, so it is stored exactly like a
// [configuration_files] entry (appspec/05). Absolute-path and escape rejection
// belong to requireUnderHome, applied uniformly to both sections.
func xdgToHomeRelative(path, home, xdgBase string) string {
	rel, err := filepath.Rel(home, filepath.Join(xdgBase, path))
	if err != nil {
		panic("cannot render XDG config path relative to home: " + path)
	}
	return rel
}
