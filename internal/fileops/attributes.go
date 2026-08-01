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

// attributeInvocations is the pure decision behind removeAttributes: the exact
// argv of each cleanup command that will actually run for path on this platform
// — the platform's commands whose binary is present, each with path appended.
// Separating the decision from the subprocess keeps command selection and
// existence gating testable without a mutable exec seam. [LAW:effects-at-boundaries]
func attributeInvocations(goos, path string) [][]string {
	var invocations [][]string
	for _, c := range attributeCleanupCommands(goos) {
		if !binaryExists(c.bin) {
			continue // the binary is absent on this system; skip this step
		}
		invocations = append(invocations, append(append([]string{c.bin}, c.args...), path))
	}
	return invocations
}

// removeAttributes strips filesystem attributes that would block a chmod or a
// delete, by running the platform's cleanup commands recursively on path. It is
// the precondition shared by Clamp and Delete (appspec/06), so it lives in one
// place. Each step is best-effort: an absent binary is skipped (by
// attributeInvocations), and a command that runs but fails (e.g. no ACL to
// remove) is ignored — the real failure, if any, surfaces at the subsequent
// chmod or remove. [LAW:no-silent-failure] exception: best-effort per appspec/06.
func removeAttributes(path string) {
	for _, argv := range attributeInvocations(runtime.GOOS, path) {
		_ = exec.Command(argv[0], argv[1:]...).Run()
	}
}

// binaryExists reports whether an executable is present at the absolute path bin.
// os.Stat follows symlinks, so a symlinked system binary counts as present.
func binaryExists(bin string) bool {
	info, err := os.Stat(bin)
	return err == nil && !info.IsDir()
}
