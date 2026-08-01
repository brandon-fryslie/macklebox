package syncops

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Policy is the three-valued confirmation decision of appspec/01 §3, fixed for
// the whole run and threaded to every prompt as data. [LAW:dataflow-not-control-flow]
type Policy int

const (
	Ask       Policy = iota // interactive: prompt on stdin
	AlwaysYes               // --force: auto-yes, no prompt shown
	AlwaysNo                // --force-no: auto-no, no prompt shown
)

// errEndOfInput is returned by Confirmer.ask when an interactive prompt is
// reached but stdin has no answer left. appspec/07 makes this the unguarded
// regime — the caller turns it into an uncaught failure, never an implicit
// yes or no.
var errEndOfInput = errors.New("end-of-input at a confirmation prompt")

// Confirmer is the one confirmation mechanism of appspec/07: every yes/no in the
// program routes through it. Prompts are written to stdout (the question text
// followed by " <Yes|No> ") and answers read from stdin. [LAW:single-enforcer]
type Confirmer struct {
	policy Policy
	in     *bufio.Reader
	out    io.Writer
}

// NewConfirmer builds the confirmer for a run from its fixed policy and the
// stdin/stdout streams.
func NewConfirmer(policy Policy, in io.Reader, out io.Writer) *Confirmer {
	return &Confirmer{policy: policy, in: bufio.NewReader(in), out: out}
}

// ask resolves one yes/no question. A force policy pre-answers it without
// prompting; otherwise the question is shown and stdin is read, accepting
// yes/y/no/n case-insensitively and re-asking on anything else. End-of-input
// with no valid answer returns errEndOfInput.
func (c *Confirmer) ask(question string) (bool, error) {
	switch c.policy {
	case AlwaysYes:
		return true, nil
	case AlwaysNo:
		return false, nil
	}
	for {
		fmt.Fprint(c.out, question+" <Yes|No> ")
		line, err := c.in.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		if err != nil {
			// Unrecognized (or empty) answer and the input has ended: no valid
			// answer can be obtained.
			return false, errEndOfInput
		}
		// Recognized nothing, but more input remains — re-ask.
	}
}
