// Command mackup is the console entry point required by appspec/00
// "Provenance": a single command named mackup.
package main

import (
	"os"

	"github.com/brandon-fryslie/macklebox/internal/cli"
)

// main is the only place that touches the real process boundary — argv,
// stdout/stderr, exit code. Every decision lives in cli, where it is testable
// without a subprocess. [LAW:effects-at-boundaries]
func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
