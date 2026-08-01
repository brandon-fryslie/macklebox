package cli

import "strings"

// style is one level of appspec/07 "Colored output", carried as the SGR
// parameter string of its escape sequence. The level scheme below transcribes
// the spec's table exactly and is the only place SGR codes appear; every
// message picks a style, never a raw escape code. [LAW:one-source-of-truth]
// Levels not yet emitted (success, anomaly, copyFailure, trace) belong to the
// operations of later epics; they are data, and they live here so those
// tickets add call sites, not codes. Diff decoration (bold/cyan/blue) is
// per-message ornament, not a level, and arrives with the drift ticket.
type style string

const (
	info        style = "33"   // normal progress / info → yellow
	anomaly     style = "1;33" // non-fatal anomaly → bold yellow
	success     style = "32"   // success / diff additions → green
	fatalError  style = "91"   // fatal errors that exit → bright red
	copyFailure style = "31"   // non-fatal per-file copy failures → red
	trace       style = "35"   // verbose-only traces → magenta
)

const reset = "\x1b[0m"

// paint colors text per appspec/07: reset-safe (the color is re-applied after
// any embedded reset so a nested reset cannot strip the rest of the line) and
// terminated with a reset. It is emitted unconditionally — paint has no notion
// of a TTY, so the no-TTY-detection contract holds by construction rather than
// by an untested branch. [LAW:dataflow-not-control-flow]
func (s style) paint(text string) string {
	open := "\x1b[" + string(s) + "m"
	return open + strings.ReplaceAll(text, reset, reset+open) + reset
}
