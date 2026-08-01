// Package config implements startup-pipeline step 2 of appspec/01 §4: it
// turns the ambient environment plus the user config file (appspec/03) into
// the eagerly-resolved storage location and application scope (appspec/04).
// Construction either yields the fully-resolved value or fails the process —
// there is no lazy or partial config, which is why a bad storage engine
// breaks even commands that never touch storage.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Env is the ambient environment the configuration subsystem reads, captured
// once at the process boundary so that resolution itself is a function of
// values. [LAW:effects-at-boundaries]
type Env struct {
	Home          string // $HOME; empty is an unguarded fatal (appspec/03 env table)
	MackupConfig  string // $MACKUP_CONFIG: discovery candidate 2; empty = skipped
	XDGConfigHome string // $XDG_CONFIG_HOME: base of candidate 3; empty = ~/.config
}

// Config is the immutable resolved value of appspec/03: the storage root, the
// sub-directory name, the derived Mackup-folder path, and the two application
// lists. Fields are unexported so no consumer can mutate or partially
// construct it — the only way to obtain one is a fully-resolved Load.
// [LAW:types-are-the-program]
type Config struct {
	root      string
	directory string
	allow     map[string]bool
	ignore    map[string]bool
}

// Root is the absolute storage root the engine resolved.
func (c Config) Root() string { return c.root }

// Directory is the sub-folder name inside the storage root.
func (c Config) Directory() string { return c.directory }

// MackupFolder is the full path all sync operations use,
// <storage-root>/<directory>. It is derived on read so it can never disagree
// with its parts. [LAW:one-source-of-truth]
func (c Config) MackupFolder() string { return filepath.Join(c.root, c.directory) }

// Scope applies the combined allow/deny precedence of appspec/03 to the full
// key set of the application database: start from the allowlist when it is
// present and non-empty (otherwise all keys), then remove every denylisted
// key — so a key in both lists is ignored. Naming an app on the command line
// bypasses this entirely (appspec/02); that override lives upstream. Order of
// allKeys is preserved.
func (c Config) Scope(allKeys []string) []string {
	scope := make([]string, 0, len(allKeys))
	for _, key := range allKeys {
		inAllow := len(c.allow) == 0 || c.allow[key]
		if inAllow && !c.ignore[key] {
			scope = append(scope, key)
		}
	}
	return scope
}

// Load resolves the user configuration eagerly: locate the config file
// (discovery or the -c override), parse it, and resolve the storage root
// through the selected engine. explicitPath "" means default discovery, as
// carried by cli.Command.ConfigFile.
//
// Failure regimes (appspec/01 §6, table in appspec/07): guarded failures
// return an error the caller renders as a clean `Error:` line with exit 1;
// unguarded failures panic — the analog of the reference's uncaught
// traceback (stderr noise, nonzero exit, no stdout). The split is carried by
// the mechanism, so an unguarded case can never leak into the guarded shape.
// [LAW:no-silent-failure]
func Load(env Env, explicitPath string) (Config, error) {
	if env.Home == "" {
		panic("HOME is not set; Mackup cannot locate the home directory")
	}
	home := filepath.Clean(env.Home)

	path, err := configPath(env, home, explicitPath)
	if err != nil {
		return Config{}, err
	}
	ini := parseINI(readConfigFile(path))
	if err := rejectLegacy(ini); err != nil {
		return Config{}, err
	}
	storage := ini["storage"]
	directory := directoryName(storage)
	root, err := parseEngine(storage).root(home)
	if err != nil {
		return Config{}, err
	}
	return Config{
		root:      root,
		directory: directory,
		allow:     keySet(ini["applications_to_sync"]),
		ignore:    keySet(ini["applications_to_ignore"]),
	}, nil
}

// configPath yields the finally-resolved config file path: the -c override
// with its must-exist rule, or the first existing regular file among the
// three discovery candidates of appspec/03, or the (possibly nonexistent)
// default. Home containment applies to whichever path wins, discovered or
// explicit alike. [LAW:single-enforcer]
func configPath(env Env, home, explicit string) (string, error) {
	if explicit != "" {
		p := resolveUserPath(explicit, home)
		if !isRegularFile(p) {
			return "", fmt.Errorf("The config file '%s' does not exist. Aborting.", p)
		}
		return requireInHome(p, home)
	}

	xdgBase := env.XDGConfigHome
	if xdgBase == "" {
		xdgBase = filepath.Join(home, ".config")
	}
	candidates := []string{filepath.Join(home, ".mackup.cfg")}
	if env.MackupConfig != "" {
		candidates = append(candidates, resolveUserPath(env.MackupConfig, home))
	}
	candidates = append(candidates, filepath.Join(xdgBase, "mackup", "mackup.cfg"))

	for _, candidate := range candidates {
		if isRegularFile(candidate) {
			return requireInHome(candidate, home)
		}
	}
	// appspec/03: with no candidate existing, the default path is used anyway
	// and parses as empty. It is inside home by construction.
	return filepath.Join(home, ".mackup.cfg"), nil
}

// resolveUserPath applies the path rules appspec/03 states for -c — leading ~
// expands to home, and a relative path is home-relative, never CWD-relative —
// to both user-named config paths (-c and MACKUP_CONFIG). The spec pins only
// the ~ rule for MACKUP_CONFIG; home-relative resolution of a bare relative
// value is the consistent completion, one rule for both.
// [LAW:one-type-per-behavior]
func resolveUserPath(p, home string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		p = filepath.Join(home, strings.TrimPrefix(p[1:], "/"))
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(home, p)
	}
	return filepath.Clean(p)
}

// requireInHome is the home-containment check of appspec/03, applied to the
// finally-resolved path regardless of how it was named. The comparison is
// lexical on the cleaned paths.
func requireInHome(p, home string) (string, error) {
	rel, err := filepath.Rel(home, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("The config file '%s' is not in your home directory. Aborting.", p)
	}
	return p, nil
}

// readConfigFile treats a nonexistent file as empty — structurally valid per
// appspec/03 "Absent / empty" — while any other read failure stays loud
// (unguarded). [LAW:no-silent-failure]
func readConfigFile(path string) string {
	text, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	if err != nil {
		panic("cannot read the config file '" + path + "': " + err.Error())
	}
	return string(text)
}

// legacySectionNames are the pre-migration section names whose presence
// aborts every command (appspec/03 "Legacy config rejection" — guarded).
var legacySectionNames = [...]string{"Allowed Applications", "Ignored Applications"}

func rejectLegacy(ini sections) error {
	for _, name := range legacySectionNames {
		if _, ok := ini[name]; ok {
			return errors.New("Old config file detected. Aborting.\n" +
				"An old section ([Allowed Applications] or [Ignored Applications]) is present in your config file,\n" +
				"and Mackup would rather do nothing than sync the wrong applications.\n" +
				"Please rename those sections to [applications_to_sync] / [applications_to_ignore] and run Mackup again.")
		}
	}
	return nil
}

// directoryName reads [storage] directory, defaulting to "Mackup". The value
// is constrained, not free (appspec/03): the two custom-apps collisions are
// unguarded fatals; anything else is accepted verbatim. The comparison runs
// on the cleaned path — filepath.Join normalizes the value onto the disk
// anyway, so "mackup/applications/" and "./.mackup" are the same collision
// the constraint forbids, not different values. ToSlash pins the comparison
// to the spec's forward-slash grammar on every platform — filepath.Clean
// alone would emit backslashes on Windows and un-match the literals below.
func directoryName(storage map[string]string) string {
	dir, ok := storage["directory"]
	if !ok {
		return "Mackup"
	}
	cleaned := filepath.ToSlash(filepath.Clean(dir))
	if cleaned == ".mackup" ||
		cleaned == "mackup/applications" ||
		cleaned == ".config/mackup/applications" ||
		strings.HasSuffix(cleaned, "/.config/mackup/applications") {
		panic("Forbidden storage directory value: " + dir)
	}
	return dir
}

// keySet lifts a parsed section's keys into a set; a missing section yields
// the empty set, so absent and empty config read identically downstream.
func keySet(section map[string]string) map[string]bool {
	set := make(map[string]bool, len(section))
	for key := range section {
		set[key] = true
	}
	return set
}
