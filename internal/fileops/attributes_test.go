package fileops

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAttributeCleanupCommandsPerPlatform(t *testing.T) {
	cases := map[string][]attrCommand{
		"darwin": {
			{"/bin/chmod", []string{"-R", "-N"}},
			{"/usr/bin/chflags", []string{"-R", "nouchg"}},
		},
		"linux": {
			{"/bin/setfacl", []string{"-R", "-b"}},
			{"/usr/bin/chattr", []string{"-R", "-f", "-i"}},
		},
		"windows": nil,
		"plan9":   nil,
	}
	for goos, want := range cases {
		if got := attributeCleanupCommands(goos); !reflect.DeepEqual(got, want) {
			t.Errorf("attributeCleanupCommands(%q) = %v, want %v", goos, got, want)
		}
	}
}

func TestAttributeInvocationsGateOnPresenceAndAppendPath(t *testing.T) {
	all := func(string) bool { return true }
	none := func(string) bool { return false }
	path := "/some/-target" // a leading-dash basename is fine: the full path is absolute

	// All present: every platform command, with path appended.
	if got := attributeInvocations("darwin", path, all); !reflect.DeepEqual(got, [][]string{
		{"/bin/chmod", "-R", "-N", path},
		{"/usr/bin/chflags", "-R", "nouchg", path},
	}) {
		t.Errorf("darwin/all = %v", got)
	}
	// None present: no invocations.
	if got := attributeInvocations("linux", path, none); got != nil {
		t.Errorf("linux/none = %v, want none", got)
	}
	// Gating: only the present binary is invoked.
	onlySetfacl := func(bin string) bool { return bin == "/bin/setfacl" }
	if got := attributeInvocations("linux", path, onlySetfacl); !reflect.DeepEqual(got, [][]string{
		{"/bin/setfacl", "-R", "-b", path},
	}) {
		t.Errorf("linux/only-setfacl = %v", got)
	}
	// Unknown platform: no commands, so no invocations regardless of the oracle.
	if got := attributeInvocations("plan9", path, all); got != nil {
		t.Errorf("plan9 = %v, want none", got)
	}
}

func TestBinaryExists(t *testing.T) {
	dir := t.TempDir()
	if binaryExists(dir) {
		t.Error("binaryExists(directory) = true, want false")
	}
	if binaryExists(filepath.Join(dir, "nope")) {
		t.Error("binaryExists(nonexistent) = true, want false")
	}
	// A non-executable file at a binary path would fail exec, so it is not a
	// usable binary.
	nonExec := filepath.Join(dir, "stub")
	if err := os.WriteFile(nonExec, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if binaryExists(nonExec) {
		t.Error("binaryExists(non-executable file) = true, want false")
	}
	exe := filepath.Join(dir, "bin")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !binaryExists(exe) {
		t.Error("binaryExists(executable file) = false, want true")
	}
}
