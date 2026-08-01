package conformance

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// appendixKeys parses the fenced key block of appspec/appendix-application-names.md
// — the reference build's 614 keys — as the independent oracle for what list
// must print. [LAW:one-source-of-truth]
func appendixKeys(t *testing.T) []string {
	t.Helper()
	f, err := os.Open("../appspec/appendix-application-names.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// Read only the first fenced block — the key list. A later fenced block
	// (a prose example, a migration note) must not leak into the oracle, and an
	// appendix with no fenced block is a structural change the test must report,
	// not silently treat as zero keys. [LAW:no-silent-failure]
	var keys []string
	inFence, sawFence := false, false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "```") {
			if inFence {
				break // first fenced block closed; the key list ends here
			}
			inFence, sawFence = true, true
			continue
		}
		if inFence {
			if k := strings.TrimSpace(line); k != "" {
				keys = append(keys, k)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if !sawFence {
		t.Fatal("no fenced key block found in the appendix")
	}
	return keys
}

// listedKeys pulls the " - <key>" lines out of list's stdout, and the count from
// its trailer.
func listedKeys(t *testing.T, stdout string) ([]string, int) {
	t.Helper()
	var keys []string
	count := -1
	trailer := regexp.MustCompile(`^(\d+) applications supported in Mackup v\S+$`)
	for _, line := range strings.Split(stripANSI(stdout), "\n") {
		if strings.HasPrefix(line, " - ") {
			keys = append(keys, strings.TrimPrefix(line, " - "))
		} else if m := trailer.FindStringSubmatch(line); m != nil {
			count, _ = strconv.Atoi(m[1])
		}
	}
	return keys, count
}

func TestBuiltinCatalogListsExactlyTheAppendixKeys(t *testing.T) {
	// The done-claim: with no user definitions, list prints exactly the
	// appendix's 614 keys and a matching count trailer.
	want := appendixKeys(t)
	if len(want) != 614 {
		t.Fatalf("appendix has %d keys, expected 614", len(want))
	}

	home := workingHome(t) // working storage config, no ~/.mackup user defs
	r := runEnv(t, home, nil, "list")
	if r.Exit != 0 {
		t.Fatalf("list exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	got, count := listedKeys(t, r.Stdout)

	if count != len(want) {
		t.Errorf("count trailer = %d, want %d", count, len(want))
	}
	if len(got) != len(want) {
		t.Fatalf("list printed %d keys, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("key %d = %q, want %q (list must match the appendix exactly, in order)", i, got[i], want[i])
		}
	}
}

func TestShowMackupCoversItsConfig(t *testing.T) {
	// The done-claim: show mackup lists its file set including .mackup.cfg.
	home := workingHome(t)
	r := runEnv(t, home, nil, "show", "mackup")
	if r.Exit != 0 {
		t.Fatalf("show mackup exit = %d, want 0; stderr=%q", r.Exit, r.Stderr)
	}
	out := stripANSI(r.Stdout)
	if !strings.Contains(out, "Name: Mackup") || !strings.Contains(out, " - .mackup.cfg") {
		t.Errorf("show mackup = %q, want the Mackup name and its .mackup.cfg file", out)
	}
}
