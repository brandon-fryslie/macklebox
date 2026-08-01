package fileops

import (
	"os"
	"os/exec"
	"runtime"
)

// attrCommand is one external attribute-stripping invocation: an absolute binary
// path and its fixed arguments, applied recursively to the target path.
type attrCommand struct {
	bin  string
	args []string
}

// attributeCleanupCommands returns the ACL- and immutable-flag-removal commands
// for a platform (appspec/06 "Attribute cleanup"). It is pure — the platform is
// a parameter — so command selection is testable without spawning anything. An
// unrecognized platform has no cleanup commands. [LAW:dataflow-not-control-flow]
func attributeCleanupCommands(goos string) []attrCommand {
	switch goos {
	case "darwin":
		return []attrCommand{
			{"/bin/chmod", []string{"-R", "-N"}},           // remove ACLs
			{"/usr/bin/chflags", []string{"-R", "nouchg"}}, // remove immutable flag
		}
	case "linux":
		return []attrCommand{
			{"/bin/setfacl", []string{"-R", "-b"}},          // remove ACLs
			{"/usr/bin/chattr", []string{"-R", "-f", "-i"}}, // remove immutable flag
		}
	}
	return nil
}

// attributeInvocations is the pure decision behind removeAttributes: given the
// platform and which binaries are present, the exact argv of each cleanup
// command that will run for path — the platform's commands whose binary is
// present, each with path appended. present is injected (binaryExists in
// production) so this is a pure function of its inputs, testable with a fake
// presence oracle and no filesystem. [LAW:effects-at-boundaries]
func attributeInvocations(goos, path string, present func(string) bool) [][]string {
	var invocations [][]string
	for _, c := range attributeCleanupCommands(goos) {
		if !present(c.bin) {
			continue // the binary is absent on this system; skip this step
		}
		invocations = append(invocations, append(append([]string{c.bin}, c.args...), path))
	}
	return invocations
}

// removeAttributes strips filesystem attributes that would block a chmod or a
// delete, by running the platform's cleanup commands recursively on path. It is
// the precondition shared by Clamp and Delete (appspec/06), so it lives in one
// place. path is expected to be absolute — the primitives operate on the
// resolved home ($HOME/f) and mackup (<folder>/f) paths — so it always begins
// with a separator and can never be misparsed by a command as an option. Each
// step is best-effort: an absent binary is skipped (by attributeInvocations),
// and a command that runs but fails (e.g. no ACL to remove) is ignored — the
// real failure, if any, surfaces at the subsequent chmod or remove.
// [LAW:no-silent-failure] exception: best-effort per appspec/06.
func removeAttributes(path string) {
	for _, argv := range attributeInvocations(runtime.GOOS, path, binaryExists) {
		_ = exec.Command(argv[0], argv[1:]...).Run()
	}
}

// binaryExists reports whether an executable file is present at the absolute
// path bin: it must exist, not be a directory, and carry an execute bit — a
// non-executable stub left at a binary path (e.g. a half-installed 0644 file)
// would fail exec, so it is gated out here rather than attempted. os.Stat
// follows symlinks, so a symlinked system binary counts.
func binaryExists(bin string) bool {
	info, err := os.Stat(bin)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}
