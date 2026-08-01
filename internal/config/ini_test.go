package config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// mustPanic asserts fn panics with a message containing want — the unguarded
// failure regime of appspec/01 §6, observed at the package boundary.
func mustPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("no panic; want one mentioning %q", want)
		}
		if msg := fmt.Sprint(r); !strings.Contains(msg, want) {
			t.Fatalf("panic = %q, want it to mention %q", msg, want)
		}
	}()
	fn()
}

func TestParseINIKeysLowercasedValuesAndSectionsExact(t *testing.T) {
	// appspec/03 case policy: keys are normalized to lowercase, values and
	// section names are case-exact.
	got := parseINI("[storage]\nEngine = File_System\nPath = /Some Folder/x\n" +
		"[applications_to_sync]\nLibreWolf\n")
	want := sections{
		"storage":              {"engine": "File_System", "path": "/Some Folder/x"},
		"applications_to_sync": {"librewolf": ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseINI = %#v, want %#v", got, want)
	}
}

func TestParseINICommentsAndBlankLines(t *testing.T) {
	got := parseINI(strings.Join([]string{
		"; whole-line comment",
		"# another",
		"",
		"[storage]",
		"engine = dropbox ; trailing comment",
		"directory = Custom # trailing comment",
		"   ",
	}, "\n"))
	want := sections{"storage": {"engine": "dropbox", "directory": "Custom"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseINI = %#v, want %#v", got, want)
	}
}

func TestParseINIBareKeysCarryNoValue(t *testing.T) {
	got := parseINI("[applications_to_ignore]\nvim\ngit\n")
	want := sections{"applications_to_ignore": {"vim": "", "git": ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseINI = %#v, want %#v", got, want)
	}
}

func TestParseINIUnknownSectionsAreKept(t *testing.T) {
	// Unknown sections are ignored by interpretation (appspec/03), but the
	// parser must retain them: legacy-section detection reads this map.
	got := parseINI("[whatever]\nkey = value\n")
	if _, ok := got["whatever"]; !ok {
		t.Errorf("parseINI dropped an unknown section: %#v", got)
	}
}

func TestParseINIKeyOutsideSectionIsLoud(t *testing.T) {
	mustPanic(t, "outside any [section]", func() { parseINI("orphan = 1\n[storage]\n") })
}
