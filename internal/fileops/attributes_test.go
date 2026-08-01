package fileops

import (
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

func TestRemoveAttributesInvokesPresentBinariesOnThePath(t *testing.T) {
	// Capture what removeAttributes would spawn: every present-binary command of
	// the current platform, applied to the path with its args; absent binaries
	// produce no invocation.
	var got [][]string
	orig := runAttrCommand
	runAttrCommand = func(bin string, args []string) {
		got = append(got, append([]string{bin}, args...))
	}
	defer func() { runAttrCommand = orig }()

	path := filepath.Join(t.TempDir(), "target")
	writeFile(t, path, "x")
	removeAttributes(path)

	var want [][]string
	for _, c := range attributeCleanupCommands(runtime.GOOS) {
		if binaryExists(c.bin) {
			want = append(want, append(append([]string{c.bin}, c.args...), path))
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("invocations = %v, want %v", got, want)
	}
}

func TestBinaryExists(t *testing.T) {
	// A directory is not an executable binary, and a nonexistent path does not
	// exist; a real dir stands in for "present but not a file".
	dir := t.TempDir()
	if binaryExists(dir) {
		t.Error("binaryExists(directory) = true, want false")
	}
	if binaryExists(filepath.Join(dir, "nope")) {
		t.Error("binaryExists(nonexistent) = true, want false")
	}
	present := filepath.Join(dir, "bin")
	writeFile(t, present, "#!/bin/sh\n")
	if !binaryExists(present) {
		t.Error("binaryExists(existing file) = false, want true")
	}
}
