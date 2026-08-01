package config

import "strings"

// sections is the parsed shape of the INI-style config file (appspec/03 "File
// format"): section name → key → value. Section names are kept exact ("Section
// presence is by exact name") while keys are lowercased — that is the config
// half of the spec's cross-component case-policy pair (config keys
// case-normalized, definition file paths case-exact). Values are verbatim
// after comment stripping and trimming, which keeps [storage] values
// case-sensitive by construction.
type sections map[string]map[string]string

// parseINI applies one uniform rule to every line — strip inline comment,
// trim, classify as section header / key=value / bare key — so comment and
// whitespace handling cannot differ between section kinds.
// [LAW:dataflow-not-control-flow]
func parseINI(text string) sections {
	parsed := sections{}
	current := ""
	for _, line := range strings.Split(text, "\n") {
		// appspec/03: text following ';' or '#' on a line is comment; a line
		// that is only comment (or blank) is ignored.
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
			// A key with no enclosing section has no defined meaning in
			// appspec/03; dropping it would hide a broken config file.
			// [LAW:no-silent-failure]
			panic("config line outside any [section]: " + line)
		}
		key, value, _ := strings.Cut(line, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			panic("config line with no key: " + line)
		}
		parsed[current][key] = strings.TrimSpace(value)
	}
	return parsed
}
