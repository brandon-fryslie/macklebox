package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brandon-fryslie/macklebox/internal/config"
)

// loadWorkingConfig resolves a file_system config whose storage root exists, so
// checkEnvironment's only remaining variable is the root guard.
func loadWorkingConfig(t *testing.T) config.Config {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".mackup.cfg"),
		[]byte("[storage]\nengine = file_system\npath = storage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "storage"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Env{Home: home}, "")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// The root-refusal path is unreachable from the black-box rig, which only ever
// runs as a non-root user (appspec/07). Driving checkEnvironment directly is
// the one way to verify the guard and its --root bypass. [LAW:behavior-not-structure]
func TestCheckEnvironmentRootGuard(t *testing.T) {
	cfg := loadWorkingConfig(t)

	if err := checkEnvironment(cfg, false, 0); err == nil {
		t.Error("effective UID 0 without --root: want the root-guard error, got nil")
	}
	if err := checkEnvironment(cfg, true, 0); err != nil {
		t.Errorf("effective UID 0 with --root: want pass, got %v", err)
	}
	if err := checkEnvironment(cfg, false, 1000); err != nil {
		t.Errorf("non-root user: want pass, got %v", err)
	}
}

func TestCheckEnvironmentStorageRootMissing(t *testing.T) {
	// A file_system config whose storage root does not exist: the gate is where
	// the engine's deferred existence check finally fires.
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".mackup.cfg"),
		[]byte("[storage]\nengine = file_system\npath = nostorage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Env{Home: home}, "")
	if err != nil {
		t.Fatal(err)
	}
	err = checkEnvironment(cfg, false, 1000)
	if err == nil {
		t.Fatal("missing storage root: want an error, got nil")
	}
	// The error must be the storage-folder gate error and must name the
	// resolved root — appspec/07 "naming the value". [LAW:verifiable-goals]
	if !strings.Contains(err.Error(), "Unable to find the storage folder") ||
		!strings.Contains(err.Error(), cfg.Root()) {
		t.Errorf("error = %q, want the storage-folder error naming the root %q", err, cfg.Root())
	}
}
