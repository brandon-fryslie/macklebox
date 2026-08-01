package cli

import (
	"reflect"
	"strings"
	"testing"
)

// The grammar cases follow appspec/02 "Invocation forms": every listed form
// accepted, non-matching forms rejected. [LAW:behavior-not-structure]
func TestParseGrammar(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want Invocation
	}{
		{"help short", []string{"-h"}, Help{}},
		{"help long", []string{"--help"}, Help{}},
		{"help short-circuits before grammar", []string{"--help", "frobnicate"}, Help{}},
		{"version", []string{"--version"}, Version{}},
		{"version short-circuits before conflict", []string{"-f", "--force-no", "--version"}, Version{}},

		{"bare invocation shows usage", []string{}, ShowUsage{}},
		{"options without subcommand show usage", []string{"-v"}, ShowUsage{}},

		{"list", []string{"list"}, Command{Verb: VerbList}},
		{"show", []string{"show", "vim"}, Command{Verb: VerbShow, App: "vim"}},
		{"backup all", []string{"backup"}, Command{Verb: VerbBackup}},
		{"backup one", []string{"backup", "vim"}, Command{Verb: VerbBackup, App: "vim"}},
		{"restore all", []string{"restore"}, Command{Verb: VerbRestore}},
		{"restore one", []string{"restore", "git"}, Command{Verb: VerbRestore, App: "git"}},
		{"link", []string{"link"}, Command{Verb: VerbLink}},
		{"link one", []string{"link", "vim"}, Command{Verb: VerbLink, App: "vim"}},
		{"link install all", []string{"link", "install"}, Command{Verb: VerbLinkInstall}},
		{"link install one", []string{"link", "install", "vim"}, Command{Verb: VerbLinkInstall, App: "vim"}},
		{"link uninstall all", []string{"link", "uninstall"}, Command{Verb: VerbLinkUninstall}},
		{"link uninstall one", []string{"link", "uninstall", "vim"}, Command{Verb: VerbLinkUninstall, App: "vim"}},

		{"force yes", []string{"-f", "backup"}, Command{Verb: VerbBackup, Confirm: ConfirmAlwaysYes}},
		{"force yes long", []string{"--force", "backup"}, Command{Verb: VerbBackup, Confirm: ConfirmAlwaysYes}},
		{"force no", []string{"--force-no", "restore"}, Command{Verb: VerbRestore, Confirm: ConfirmAlwaysNo}},
		{"force conflict", []string{"-f", "--force-no", "backup"}, ForceConflict{}},
		{"force conflict long", []string{"--force", "--force-no", "list"}, ForceConflict{}},

		{"all boolean options", []string{"-r", "-n", "-v", "list"},
			Command{Verb: VerbList, Root: true, DryRun: true, Verbose: true}},
		{"long boolean options", []string{"--root", "--dry-run", "--verbose", "list"},
			Command{Verb: VerbList, Root: true, DryRun: true, Verbose: true}},
		{"config file short", []string{"-c", "/tmp/cfg", "list"},
			Command{Verb: VerbList, ConfigFile: "/tmp/cfg"}},
		{"config file long", []string{"--config-file", "/tmp/cfg", "list"},
			Command{Verb: VerbList, ConfigFile: "/tmp/cfg"}},
		{"config file equals", []string{"--config-file=/tmp/cfg", "list"},
			Command{Verb: VerbList, ConfigFile: "/tmp/cfg"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.argv)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Parse(%q) = %#v, want %#v", tc.argv, got, tc.want)
			}
		})
	}
}

// Non-matching argv is a usage error carrying a warning that identifies the
// unmatched input (appspec/02 "Argument-parser behavior").
func TestParseUsageErrors(t *testing.T) {
	cases := []struct {
		name        string
		argv        []string
		wantWarning string // substring the warning must contain
	}{
		{"unknown subcommand", []string{"frobnicate"}, "frobnicate"},
		{"show without application", []string{"show"}, "show"},
		{"list with extra argument", []string{"list", "extra"}, "extra"},
		{"show with extra argument", []string{"show", "vim", "extra"}, "extra"},
		{"backup with extra argument", []string{"backup", "vim", "extra"}, "extra"},
		{"link with extra argument", []string{"link", "vim", "extra"}, "extra"},
		{"link install with extra argument", []string{"link", "install", "vim", "extra"}, "extra"},
		{"unknown option", []string{"--wat", "list"}, "--wat"},
		{"config file without path", []string{"-c"}, "-c"},
		{"option after subcommand", []string{"backup", "-v"}, "-v"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.argv)
			ue, ok := got.(UsageError)
			if !ok {
				t.Fatalf("Parse(%q) = %#v, want UsageError", tc.argv, got)
			}
			if !strings.Contains(ue.Warning, tc.wantWarning) {
				t.Errorf("Parse(%q) warning = %q, want it to mention %q", tc.argv, ue.Warning, tc.wantWarning)
			}
		})
	}
}
