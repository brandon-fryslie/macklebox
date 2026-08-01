package cli

// The usage lines transcribe the invocation forms of appspec/02 exactly; the
// surrounding wording is human-facing and explicitly not machine contract
// (appspec/02 "Argument-parser behavior"). One block serves help, the bare
// invocation, and usage errors, so the grammar is stated once.
// [LAW:one-source-of-truth]
const usageText = `Usage:
  mackup [options] list
  mackup [options] show <application>
  mackup [options] backup [<application>]
  mackup [options] restore [<application>]
  mackup [options] link install [<application>]
  mackup [options] link uninstall [<application>]
  mackup [options] link [<application>]
  mackup -h | --help
  mackup --version
`

const helpText = `Mackup — keep your application settings in sync across machines.

` + usageText + `
Options:
  -h, --help                Print this help text and exit.
      --version             Print the Mackup version and exit.
  -f, --force               Answer every confirmation prompt with yes.
      --force-no            Answer every confirmation prompt with no.
  -r, --root                Permit running as the superuser.
  -n, --dry-run             Print the steps that would be taken, but change no files.
  -v, --verbose             Print full paths and per-file traces.
  -c, --config-file <path>  Use <path> as the user config file.
`
