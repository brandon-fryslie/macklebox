package config

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

// engine is the closed set of appspec/04: exactly four implementations of one
// three-clause contract. Type and method are unexported, so a fifth engine —
// or an engine value that skipped parseEngine — is unrepresentable outside
// this package. [LAW:types-are-the-program]
type engine interface {
	// root resolves the storage root from ambient HOME: one absolute path.
	// A returned error is the guarded provider-not-found fatal (clause 3);
	// the file_system engine never errors here (clause 2's deliberate
	// non-uniformity — see fileSystemEngine).
	root(home string) (string, error)
}

// parseEngine is the one type boundary where the [storage] values become a
// member of the closed engine set — matched exactly and case-sensitively
// (appspec/03). Unknown engine names and a pathless file_system are unguarded
// fatals (appspec/01 §6): they panic rather than return, so they can never be
// rendered as the guarded clean-exit shape. [LAW:parse-dont-validate]
func parseEngine(storage map[string]string) engine {
	name, ok := storage["engine"]
	if !ok {
		name = "dropbox" // appspec/03: the default engine
	}
	switch name {
	case "dropbox":
		return dropboxEngine{}
	case "google_drive":
		return googleDriveEngine{}
	case "icloud":
		return icloudEngine{}
	case "file_system":
		path, ok := storage["path"]
		if !ok {
			panic("The 'file_system' storage engine requires a 'path' key in the [storage] section")
		}
		return fileSystemEngine{path: path}
	}
	panic("Unknown storage engine: " + name)
}

// providerNotFound is the one guarded failure shape all three auto-detecting
// engines share (appspec/04 clause 3): provider name, guidance, documentation
// pointer. The engines differ only in the data they pass through this seam.
// [LAW:one-type-per-behavior]
func providerNotFound(provider string) error {
	return fmt.Errorf("Unable to find your %s =(\n"+
		"Maybe you can use another storage provider for Mackup.\n"+
		"See https://github.com/brandon-fryslie/macklebox/blob/master/appspec/04-storage-engines.md",
		provider)
}

// dropboxEngine reads the Dropbox client's host database (appspec/04): the
// file's whitespace-separated second token, strict-Base64-decoded to text, is
// the Dropbox folder path. Any miss on that shape is the same guarded fatal.
type dropboxEngine struct{}

func (dropboxEngine) root(home string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(home, ".dropbox", "host.db"))
	if err != nil {
		return "", providerNotFound("Dropbox install")
	}
	tokens := strings.Fields(string(raw))
	if len(tokens) < 2 {
		return "", providerNotFound("Dropbox install")
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(tokens[1])
	if err != nil || !utf8.Valid(decoded) {
		return "", providerNotFound("Dropbox install")
	}
	return string(decoded), nil
}

// googleDriveEngine reads the Google Drive client's sync database
// (appspec/04): the preferred user_default DB if it exists, otherwise the
// flat one; from it, the local_sync_root_path row is the storage root.
type googleDriveEngine struct{}

func (googleDriveEngine) root(home string) (string, error) {
	drive := filepath.Join(home, "Library", "Application Support", "Google", "Drive")
	db := filepath.Join(drive, "user_default", "sync_config.db")
	if !isRegularFile(db) {
		db = filepath.Join(drive, "sync_config.db")
	}
	if !isRegularFile(db) {
		return "", providerNotFound("Google Drive install")
	}
	conn, err := sql.Open("sqlite", db)
	if err != nil {
		return "", providerNotFound("Google Drive install")
	}
	defer conn.Close()
	var root string
	err = conn.QueryRow(
		"SELECT data_value FROM data WHERE entry_key = 'local_sync_root_path'").Scan(&root)
	if err != nil || root == "" {
		// appspec/04: no row, empty value, and unopenable/unqueryable DB are
		// all the same observable fatal.
		return "", providerNotFound("Google Drive install")
	}
	return root, nil
}

// icloudEngine resolves the fixed macOS iCloud Drive location (appspec/04);
// the existence of that directory is the resolution.
type icloudEngine struct{}

func (icloudEngine) root(home string) (string, error) {
	p := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		return "", providerNotFound("iCloud Drive")
	}
	return p, nil
}

// fileSystemEngine resolves the user-supplied [storage] path: relative under
// home, absolute (leading /) verbatim — and performs NO existence check.
// That is appspec/04 clause 2's deliberate non-uniformity: the uniform
// existence guarantee is supplied later by the environment gate (appspec/01
// §4), where a bad path fails as "Unable to find the storage folder", not
// here. Adding "the missing check" would move that observable failure point.
type fileSystemEngine struct{ path string }

func (e fileSystemEngine) root(home string) (string, error) {
	if filepath.IsAbs(e.path) {
		return filepath.Clean(e.path), nil
	}
	return filepath.Join(home, e.path), nil
}

func isRegularFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}
