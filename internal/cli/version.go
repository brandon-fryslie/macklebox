package cli

import (
	"runtime/debug"
	"strings"
)

// fallbackVersion is the stable token appspec/00 "Provenance" requires when no
// installed package metadata is available (reference behavior: "unknown").
const fallbackVersion = "unknown"

// versionString resolves the version per appspec/00 "Provenance": the
// package's own version when installed, the stable fallback token otherwise.
// The toolchain's build metadata is the single authority — there is no
// hand-maintained version constant to drift from it. [LAW:one-source-of-truth]
func versionString() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return fallbackVersion
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" {
		return fallbackVersion
	}
	return strings.TrimPrefix(v, "v")
}
