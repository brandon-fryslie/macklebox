package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/brandon-fryslie/macklebox/internal/appdb"
)

// runList prints the appspec/05 Enumeration format: the sorted application keys
// under a header, then a blank line and a count trailer naming the version.
// All of it is info-level output on stdout (appspec/07). [LAW:effects-at-boundaries]
func runList(db appdb.Database, stdout io.Writer) int {
	keys := db.Keys()
	var b strings.Builder
	b.WriteString("Supported applications:\n")
	for _, key := range keys {
		b.WriteString(" - " + key + "\n")
	}
	fmt.Fprintf(&b, "\n%d applications supported in Mackup v%s\n", len(keys), versionString())
	fmt.Fprint(stdout, info.paint(b.String()))
	return 0
}

// runShow prints one application's display name and sorted file set (appspec/05
// Enumeration) to stdout at info level. An unknown key is the guarded
// "Unsupported application" fatal — stderr, exit 1, the literal prefix that
// appspec/07 pins as contract — routed off Lookup's typed-absence bool rather
// than a fabricated empty entry. [LAW:parse-dont-validate]
func runShow(db appdb.Database, key string, stdout, stderr io.Writer) int {
	app, ok := db.Lookup(key)
	if !ok {
		// The bare "Unsupported application:" contract token (appspec/07) —
		// through the shared fatal renderer, but without fatal's "Error:" prefix.
		return fatalLine(stderr, "Unsupported application: "+key)
	}
	var b strings.Builder
	b.WriteString("Name: " + app.Name() + "\n")
	b.WriteString("Configuration files:\n")
	for _, path := range app.Files() {
		b.WriteString(" - " + path + "\n")
	}
	fmt.Fprint(stdout, info.paint(b.String()))
	return 0
}
