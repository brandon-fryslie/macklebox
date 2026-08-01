// Package ini is the one structural reader for the two INI-style file formats
// the program consumes: the user config file (appspec/03) and the application
// definition files (appspec/05). It owns exactly the structural rules both
// formats share — inline and whole-line comment stripping, whitespace
// trimming, [section] header detection, and `key = value` / bare-key lines —
// and nothing about what any section means. [LAW:one-source-of-truth]
//
// The single axis on which the two formats differ is key case, and it is a
// parameter, not a fork: the reference implementation's configparser lowercases
// keys for the config file (its default optionxform) and preserves them for
// definition files (optionxform = str), and keyName carries exactly that
// choice. [LAW:dataflow-not-control-flow]
package ini

import "strings"

// Sections is the parsed shape: section name → key → value. Section names are
// verbatim — the bracket contents, untrimmed, matching configparser, which
// does not strip whitespace inside a header. Keys pass through the caller's
// keyName; values are verbatim after comment stripping and trimming.
type Sections map[string]map[string]string

// Parse applies one uniform rule to every line so comment and whitespace
// handling cannot differ between section kinds. keyName normalizes every key
// (strings.ToLower for the config file's case-normalized keys, an identity
// function for definition files' case-exact paths). A key outside any section,
// or a key that normalizes to empty, is an unguarded fatal — dropping either
// would hide a malformed file rather than surface it. [LAW:no-silent-failure]
func Parse(text string, keyName func(string) string) Sections {
	parsed := Sections{}
	current := ""
	for _, line := range strings.Split(text, "\n") {
		// appspec/03 and appspec/05: text following ';' or '#' is a comment; a
		// line that is only comment (or blank) is ignored.
		if i := strings.IndexAny(line, ";#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = line[1 : len(line)-1]
			if _, ok := parsed[current]; !ok {
				parsed[current] = map[string]string{}
			}
			continue
		}
		if current == "" {
			// A key with no enclosing section has no defined meaning in either
			// format; dropping it would hide a broken file.
			// [LAW:no-silent-failure]
			panic("config line outside any [section]: " + line)
		}
		key, value, _ := strings.Cut(line, "=")
		key = keyName(strings.TrimSpace(key))
		if key == "" {
			panic("config line with no key: " + line)
		}
		parsed[current][key] = strings.TrimSpace(value)
	}
	return parsed
}
