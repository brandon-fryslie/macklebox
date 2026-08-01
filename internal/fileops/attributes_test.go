package fileops

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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
	path := "/some/target"
	invs := attributeInvocations(runtime.GOOS, path)

	// Every invocation names a present binary and ends with the path.
	for _, argv := range invs {
		if !binaryExists(argv[0]) {
			t.Errorf("invocation %v names an absent binary", argv)
		}
		if argv[len(argv)-1] != path {
			t.Errorf("invocation %v does not end with the path", argv)
		}
	}
	// The count is exactly the present-binary subset of the platform's commands.
	present := 0
	for _, c := range attributeCleanupCommands(runtime.GOOS) {
		if binaryExists(c.bin) {
			present++
		}
	}
	if len(invs) != present {
		t.Errorf("got %d invocations, want %d (present binaries)", len(invs), present)
	}
	// An unknown platform has no commands, so no invocations.
	if inv := attributeInvocations("plan9", path); inv != nil {
		t.Errorf("plan9 invocations = %v, want none", inv)
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
