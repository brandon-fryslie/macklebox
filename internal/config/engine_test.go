package config

import (
	"database/sql"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
)

// storageEnv writes ~/.mackup.cfg with the given [storage] body and returns
// the env, so each engine test states only its storage facts.
func storageEnv(t *testing.T, body string) Env {
	t.Helper()
	home := t.TempDir()
	write(t, filepath.Join(home, ".mackup.cfg"), "[storage]\n"+body)
	return Env{Home: home}
}

func loadErr(t *testing.T, env Env) error {
	t.Helper()
	_, err := Load(env, "")
	if err == nil {
		t.Fatal("Load succeeded, want a guarded error")
	}
	return err
}

func TestUnknownEngineIsUnguardedAndNamesTheValue(t *testing.T) {
	env := storageEnv(t, "engine = floppynet\n")
	mustPanic(t, "Unknown storage engine: floppynet", func() { _, _ = Load(env, "") })
}

func TestEngineValueIsCaseSensitive(t *testing.T) {
	// appspec/03: [storage] values are matched exactly; "Dropbox" is not a
	// member of the closed set.
	env := storageEnv(t, "engine = Dropbox\n")
	mustPanic(t, "Unknown storage engine: Dropbox", func() { _, _ = Load(env, "") })
}

// --- file_system ---

func TestFileSystemPathIsRequired(t *testing.T) {
	// appspec/04: omitting path under file_system is an unguarded fatal.
	env := storageEnv(t, "engine = file_system\n")
	mustPanic(t, "path", func() { _, _ = Load(env, "") })
}

func TestFileSystemRelativeAbsoluteAndSpaces(t *testing.T) {
	// appspec/04: relative path under home, absolute (leading /) verbatim,
	// spaces need no quoting.
	cases := []struct{ path, wantRoot string }{
		{"some/folder", "<home>/some/folder"},
		{"/abs/folder", "/abs/folder"},
		{"My Sync Folder/backups", "<home>/My Sync Folder/backups"},
	}
	for _, c := range cases {
		env := storageEnv(t, "engine = file_system\npath = "+c.path+"\n")
		cfg := load(t, env, "")
		want := strings.ReplaceAll(c.wantRoot, "<home>", env.Home)
		if cfg.Root() != want {
			t.Errorf("path %q: Root = %q, want %q", c.path, cfg.Root(), want)
		}
	}
}

func TestFileSystemPerformsNoExistenceCheckAtResolution(t *testing.T) {
	// appspec/04 clause 2, the deliberately NON-uniform postcondition: the
	// user-path engine returns the path without checking it exists — the
	// uniform existence guarantee belongs to the environment gate (appspec/01
	// §4), not the resolver. This test pins the deferral; do not "fix" it.
	env := storageEnv(t, "engine = file_system\npath = does/not/exist\n")
	cfg := load(t, env, "")
	if want := filepath.Join(env.Home, "does/not/exist"); cfg.Root() != want {
		t.Errorf("Root = %q, want %q resolved with no existence check", cfg.Root(), want)
	}
}

// --- [storage] directory ---

func TestDirectoryCustomValueIsVerbatim(t *testing.T) {
	env := storageEnv(t, "engine = file_system\npath = store\ndirectory = My Backups\n")
	cfg := load(t, env, "")
	if want := filepath.Join(env.Home, "store", "My Backups"); cfg.MackupFolder() != want {
		t.Errorf("MackupFolder = %q, want %q", cfg.MackupFolder(), want)
	}
}

func TestDirectoryForbiddenValuesAreUnguarded(t *testing.T) {
	// appspec/03: the storage sub-directory may never collide with a
	// custom-apps directory.
	for _, dir := range []string{
		".mackup",
		"mackup/applications",
		".config/mackup/applications",
		"nested/.config/mackup/applications",
	} {
		env := storageEnv(t, "engine = file_system\npath = store\ndirectory = "+dir+"\n")
		mustPanic(t, dir, func() { _, _ = Load(env, "") })
	}
}

// --- dropbox ---

func TestDropboxDecodesHostDB(t *testing.T) {
	// appspec/04: second whitespace-separated token, Base64-decoded, is the
	// Dropbox folder.
	home := t.TempDir()
	write(t, filepath.Join(home, ".dropbox", "host.db"),
		"ignored "+base64.StdEncoding.EncodeToString([]byte("/home/some_user/Dropbox"))+"\n")
	cfg := load(t, Env{Home: home}, "")

	if cfg.Root() != "/home/some_user/Dropbox" {
		t.Errorf("Root = %q, want the decoded Dropbox path", cfg.Root())
	}
	if want := "/home/some_user/Dropbox/Mackup"; cfg.MackupFolder() != want {
		t.Errorf("MackupFolder = %q, want %q", cfg.MackupFolder(), want)
	}
}

func TestDropboxFailuresAreTheGuardedProviderFatal(t *testing.T) {
	// appspec/04: missing file, fewer than two tokens, invalid Base64, and
	// undecodable text are all the same guarded fatal.
	cases := map[string]func(home string){
		"missing host.db": func(string) {},
		"one token": func(home string) {
			write(t, filepath.Join(home, ".dropbox", "host.db"), "only-one-token")
		},
		"invalid base64": func(home string) {
			write(t, filepath.Join(home, ".dropbox", "host.db"), "x not!valid!b64")
		},
		"not text": func(home string) {
			write(t, filepath.Join(home, ".dropbox", "host.db"),
				"x "+base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe}))
		},
	}
	for name, arrange := range cases {
		home := t.TempDir()
		arrange(home)
		err := loadErr(t, Env{Home: home})
		if !strings.Contains(err.Error(), "Dropbox install") {
			t.Errorf("%s: err = %v, want the Dropbox provider fatal", name, err)
		}
	}
}

// --- icloud ---

func TestICloudResolvesTheFixedPath(t *testing.T) {
	home := t.TempDir()
	cloud := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")
	write(t, filepath.Join(cloud, ".keep"), "")
	env := Env{Home: home}
	write(t, filepath.Join(home, ".mackup.cfg"), "[storage]\nengine = icloud\n")

	if got := load(t, env, "").Root(); got != cloud {
		t.Errorf("Root = %q, want %q", got, cloud)
	}
}

func TestICloudMissingDirectoryIsTheGuardedProviderFatal(t *testing.T) {
	env := storageEnv(t, "engine = icloud\n")
	if err := loadErr(t, env); !strings.Contains(err.Error(), "iCloud Drive") {
		t.Errorf("err = %v, want the iCloud Drive provider fatal", err)
	}
}

// --- google_drive ---

func gdriveBase(home string) string {
	return filepath.Join(home, "Library", "Application Support", "Google", "Drive")
}

func writeSyncConfigDB(t *testing.T, path string, rows map[string]string) {
	t.Helper()
	write(t, path, "") // ensure parent dirs exist; sqlite will overwrite
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec("CREATE TABLE data (entry_key TEXT, data_value TEXT)"); err != nil {
		t.Fatal(err)
	}
	for key, value := range rows {
		if _, err := conn.Exec("INSERT INTO data VALUES (?, ?)", key, value); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGoogleDrivePreferredDBWinsOverFallback(t *testing.T) {
	// appspec/04: the user_default DB is preferred when it exists.
	env := storageEnv(t, "engine = google_drive\n")
	writeSyncConfigDB(t, filepath.Join(gdriveBase(env.Home), "user_default", "sync_config.db"),
		map[string]string{"local_sync_root_path": "/gd/preferred"})
	writeSyncConfigDB(t, filepath.Join(gdriveBase(env.Home), "sync_config.db"),
		map[string]string{"local_sync_root_path": "/gd/fallback"})

	if got := load(t, env, "").Root(); got != "/gd/preferred" {
		t.Errorf("Root = %q, want /gd/preferred", got)
	}
}

func TestGoogleDriveFallbackDBIsUsedWhenPreferredAbsent(t *testing.T) {
	env := storageEnv(t, "engine = google_drive\n")
	writeSyncConfigDB(t, filepath.Join(gdriveBase(env.Home), "sync_config.db"),
		map[string]string{"local_sync_root_path": "/gd/fallback"})

	if got := load(t, env, "").Root(); got != "/gd/fallback" {
		t.Errorf("Root = %q, want /gd/fallback", got)
	}
}

func TestGoogleDriveFailuresAreTheGuardedProviderFatal(t *testing.T) {
	// appspec/04: no DB file, no usable row, empty value, and an unqueryable
	// file are all the same guarded fatal.
	cases := map[string]func(home string){
		"no db":       func(string) {},
		"row missing": func(home string) { writeSyncConfigDB(t, filepath.Join(gdriveBase(home), "sync_config.db"), nil) },
		"empty value": func(home string) {
			writeSyncConfigDB(t, filepath.Join(gdriveBase(home), "sync_config.db"),
				map[string]string{"local_sync_root_path": ""})
		},
		"not sqlite": func(home string) {
			write(t, filepath.Join(gdriveBase(home), "sync_config.db"), "this is not a database")
		},
	}
	for name, arrange := range cases {
		env := storageEnv(t, "engine = google_drive\n")
		arrange(env.Home)
		if err := loadErr(t, env); !strings.Contains(err.Error(), "Google Drive install") {
			t.Errorf("%s: err = %v, want the Google Drive provider fatal", name, err)
		}
	}
}
