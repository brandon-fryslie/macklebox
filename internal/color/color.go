// Package color is the one home of the appspec/07 "Colored output" scheme: the
// level→SGR mapping and the reset-safe painter. Every message in the program
// picks a Style and calls Paint; no raw escape code appears anywhere else, so
// the scheme cannot drift between the cli and the sync executor.
// [LAW:one-source-of-truth]
package color

import "strings"

// Style is one output level of appspec/07, carried as the SGR parameter string
// of its escape sequence.
type Style string

const (
	Info        Style = "33"   // normal progress / info → yellow
	Anomaly     Style = "1;33" // non-fatal anomaly (the "differs between …" header) → bold yellow
	Success     Style = "32"   // success / diff additions → green
	FatalError  Style = "91"   // fatal errors that exit → bright red
	CopyFailure Style = "31"   // non-fatal per-file copy failures → red
	Trace       Style = "35"   // verbose-only traces → magenta
)

const reset = "\x1b[0m"

// Paint colors text per appspec/07: reset-safe (the color is re-applied after
// any embedded reset so a nested reset cannot strip the rest of the line) and
// terminated with a reset. It is emitted unconditionally — Paint has no notion
// of a TTY, so the no-TTY-detection contract holds by construction rather than
// by an untested branch. [LAW:dataflow-not-control-flow]
func (s Style) Paint(text string) string {
	open := "\x1b[" + string(s) + "m"
	return open + strings.ReplaceAll(text, reset, reset+open) + reset
}
