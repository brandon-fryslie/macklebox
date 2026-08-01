package ini

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// lower and exact are the two key-normalizers the real formats use: the config
// file's default configparser lowercasing, and definition files'
// optionxform = str case preservation.
func lower(s string) string { return strings.ToLower(s) }
func exact(s string) string { return s }

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

func TestParseLowercasesKeysUnderLowerNormalizer(t *testing.T) {
	// appspec/03 case policy: config keys are normalized to lowercase, values
	// and section names are case-exact.
	got := Parse("[storage]\nEngine = File_System\nPath = /Some Folder/x\n"+
		"[applications_to_sync]\nLibreWolf\n", lower)
	want := Sections{
		"storage":              {"engine": "File_System", "path": "/Some Folder/x"},
		"applications_to_sync": {"librewolf": ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse = %#v, want %#v", got, want)
	}
}

func TestParsePreservesKeyCaseUnderExactNormalizer(t *testing.T) {
	// appspec/05 case policy: definition file paths keep their exact case — the
	// other half of the cross-component case-policy pair.
	got := Parse("[configuration_files]\n.config/Karabiner\n.VIM/vimrc\n", exact)
	want := Sections{
		"configuration_files": {".config/Karabiner": "", ".VIM/vimrc": ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse = %#v, want %#v", got, want)
	}
}

func TestParseSectionNamesAreUntrimmed(t *testing.T) {
	// configparser does not strip whitespace inside a header, so [ storage ]
	// is the section " storage ", NOT "storage" — the exact-name behavior
	// pinned by appspec/03 and relied on by the [storage] lookup.
	got := Parse("[ storage ]\nengine = icloud\n", lower)
	if _, ok := got[" storage "]; !ok {
		t.Errorf("Parse dropped the padded section name: %#v", got)
	}
	if _, ok := got["storage"]; ok {
		t.Errorf("Parse trimmed a section name it must keep verbatim: %#v", got)
	}
}

func TestParseCommentsAndBlankLines(t *testing.T) {
	got := Parse(strings.Join([]string{
		"; whole-line comment",
		"# another",
		"",
		"[storage]",
		"engine = dropbox ; trailing comment",
		"directory = Custom # trailing comment",
		"   ",
	}, "\n"), lower)
	want := Sections{"storage": {"engine": "dropbox", "directory": "Custom"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse = %#v, want %#v", got, want)
	}
}

func TestParseBareKeysCarryNoValue(t *testing.T) {
	got := Parse("[applications_to_ignore]\nvim\ngit\n", lower)
	want := Sections{"applications_to_ignore": {"vim": "", "git": ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse = %#v, want %#v", got, want)
	}
}

func TestParseUnknownSectionsAreKept(t *testing.T) {
	// Unknown sections are ignored by interpretation, but the parser must
	// retain them: legacy-section detection reads this map.
	got := Parse("[whatever]\nkey = value\n", lower)
	if _, ok := got["whatever"]; !ok {
		t.Errorf("Parse dropped an unknown section: %#v", got)
	}
}

func TestParseKeyOutsideSectionIsLoud(t *testing.T) {
	mustPanic(t, "outside any [section]", func() { Parse("orphan = 1\n[storage]\n", lower) })
}

func TestParseEmptyKeyIsLoud(t *testing.T) {
	// A line whose key normalizes to empty (only whitespace before '=') is the
	// second loud-failure branch — a malformed line, not a droppable one.
	mustPanic(t, "no key", func() { Parse("[storage]\n  = value\n", lower) })
}
