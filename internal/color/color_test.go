package color

import "testing"

// These pin the three coloring properties appspec/07 "Colored output" states:
// wrapped in the level's SGR code, terminated with a reset, and reset-safe —
// a reset embedded in the message must not strip color from the rest of the
// line. Paint is pure, so the contract is checkable with no process, no
// writer, no mock. [LAW:effects-at-boundaries]

func TestPaintWrapsAndTerminatesWithReset(t *testing.T) {
	got := Info.Paint("Backing up vim")
	want := "\x1b[33mBacking up vim\x1b[0m"
	if got != want {
		t.Errorf("Paint = %q, want %q", got, want)
	}
}

func TestPaintReappliesColorAfterEmbeddedReset(t *testing.T) {
	got := FatalError.Paint("before\x1b[0mafter")
	want := "\x1b[91mbefore\x1b[0m\x1b[91mafter\x1b[0m"
	if got != want {
		t.Errorf("Paint = %q, want %q", got, want)
	}
}
