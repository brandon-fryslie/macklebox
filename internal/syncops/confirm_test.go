package syncops

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestConfirmerForcePoliciesDoNotPrompt(t *testing.T) {
	var out strings.Builder
	yes := NewConfirmer(AlwaysYes, strings.NewReader(""), &out)
	if ok, err := yes.ask("replace?"); !ok || err != nil {
		t.Errorf("AlwaysYes = (%v, %v), want (true, nil)", ok, err)
	}
	no := NewConfirmer(AlwaysNo, strings.NewReader(""), &out)
	if ok, err := no.ask("replace?"); ok || err != nil {
		t.Errorf("AlwaysNo = (%v, %v), want (false, nil)", ok, err)
	}
	if out.Len() != 0 {
		t.Errorf("force policies wrote a prompt: %q", out.String())
	}
}

func TestConfirmerAcceptsYesNoCaseInsensitivelyAndReasks(t *testing.T) {
	cases := map[string]bool{"y\n": true, "Yes\n": true, "n\n": false, "NO\n": false}
	for input, want := range cases {
		var out strings.Builder
		c := NewConfirmer(Ask, strings.NewReader(input), &out)
		got, err := c.ask("replace?")
		if err != nil || got != want {
			t.Errorf("ask(%q) = (%v, %v), want (%v, nil)", input, got, err, want)
		}
		if !strings.Contains(out.String(), "replace? <Yes|No> ") {
			t.Errorf("prompt = %q, want the question with the <Yes|No> suffix", out.String())
		}
	}

	// An unrecognized answer re-asks, then a recognized one resolves.
	var out strings.Builder
	c := NewConfirmer(Ask, strings.NewReader("maybe\nyes\n"), &out)
	if got, err := c.ask("replace?"); !got || err != nil {
		t.Errorf("re-ask = (%v, %v), want (true, nil)", got, err)
	}
	if strings.Count(out.String(), "<Yes|No>") != 2 {
		t.Errorf("expected the prompt twice (re-ask), got %q", out.String())
	}
}

func TestConfirmerEndOfInputIsAnError(t *testing.T) {
	var out strings.Builder
	c := NewConfirmer(Ask, strings.NewReader(""), &out) // immediate EOF
	if _, err := c.ask("replace?"); !errors.Is(err, errEndOfInput) {
		t.Errorf("EOF ask err = %v, want errEndOfInput", err)
	}
}

var _ io.Reader = strings.NewReader("")
