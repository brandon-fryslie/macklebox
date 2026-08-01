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

// runAttrCommand executes one attribute-cleanup command, ignoring its result —
// best-effort. It is a package variable solely so tests can observe which
// commands removeAttributes spawns without the host needing chmod/setfacl/etc.;
// production always runs the real subprocess. [LAW:no-shared-mutable-globals]
// exception: an exec seam with a stable default, overridden only under test.
var runAttrCommand = func(bin string, args []string) {
	_ = exec.Command(bin, args...).Run()
}

// removeAttributes strips filesystem attributes that would block a chmod or a
// delete, by running the platform's cleanup commands recursively on path. It is
// the precondition shared by Clamp and Delete (appspec/06), so it lives in one
// place. Each step is best-effort: a step whose binary is absent is skipped, and
// a command that runs but fails (e.g. no ACL to remove) is ignored — the real
// failure, if any, surfaces at the subsequent chmod or remove.
// [LAW:no-silent-failure] exception: best-effort attribute cleanup per appspec/06.
func removeAttributes(path string) {
	for _, c := range attributeCleanupCommands(runtime.GOOS) {
		if !binaryExists(c.bin) {
			continue // the binary is absent on this system; skip this step
		}
		runAttrCommand(c.bin, append(append([]string{}, c.args...), path))
	}
}

// binaryExists reports whether an executable is present at the absolute path bin.
// os.Stat follows symlinks, so a symlinked system binary counts as present.
func binaryExists(bin string) bool {
	info, err := os.Stat(bin)
	return err == nil && !info.IsDir()
}
