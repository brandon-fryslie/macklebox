// Package drift implements the source-vs-destination comparison of appspec/06
// "Drift detection": given a path that exists on both sides, it decides
// identical (skip — the backup/restore idempotency fixed point) vs. differs, and
// for a difference produces the detail text the caller shows before replacing.
// The detail is plain; the diff coloring of appspec/07 is applied at the print
// site (the backup/restore command), which owns the color layer.
package drift

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"unicode/utf8"

	"github.com/pmezard/go-difflib/difflib"
	"howett.net/plist"
)

// Comparison is the (identical, detail) result of appspec/06. Detail is empty
// when the paths are not content-comparable (either is a symlink, a file is
// unreadable, or the types are incomparable), in which case the caller shows a
// plain prompt with no diff. [LAW:types-are-the-program]
type Comparison struct {
	Identical bool
	Detail    string
}

var (
	identical  = Comparison{Identical: true}
	differsNo  = Comparison{Identical: false} // differs, no detail (plain prompt)
	binaryDiff = Comparison{Identical: false, Detail: "binary contents differ"}
)

// Compare classifies source against dest (appspec/06 "Drift detection"). Both
// paths are expected to exist — drift is consulted only when a destination copy
// is already present. The dispatch is total over the lstat type pair: either
// being a symlink short-circuits to differs/no-detail before any type logic.
func Compare(source, dest string) Comparison {
	srcInfo, srcErr := os.Lstat(source)
	dstInfo, dstErr := os.Lstat(dest)
	if srcErr != nil || dstErr != nil {
		return differsNo // one side missing or unreadable: cannot compare
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 || dstInfo.Mode()&os.ModeSymlink != 0 {
		return differsNo // appspec/06: either is a symlink → differs, no diff
	}

	srcDir, dstDir := srcInfo.IsDir(), dstInfo.IsDir()
	srcReg, dstReg := srcInfo.Mode().IsRegular(), dstInfo.Mode().IsRegular()
	switch {
	case srcDir && dstReg:
		return Comparison{Identical: false, Detail: "type mismatch: folder vs file"}
	case srcReg && dstDir:
		return Comparison{Identical: false, Detail: "type mismatch: file vs folder"}
	case srcReg && dstReg:
		return compareFiles(source, dest)
	case srcDir && dstDir:
		return compareDirs(source, dest)
	}
	return differsNo // special files (device/socket/etc.): not content-comparable
}

// compareFiles runs the two-regular-files cascade of appspec/06: plist, then
// UTF-8 text, then byte-for-byte. An unreadable file is differs/no-detail.
func compareFiles(source, dest string) Comparison {
	srcData, srcErr := os.ReadFile(source)
	dstData, dstErr := os.ReadFile(dest)
	if srcErr != nil || dstErr != nil {
		return differsNo // appspec/06: either unreadable → differs, no detail
	}

	// 1. Both parse as property lists: compare parsed content.
	if srcPlist, ok := parsePlist(srcData); ok {
		if dstPlist, ok := parsePlist(dstData); ok {
			if reflect.DeepEqual(srcPlist, dstPlist) {
				return identical
			}
			// The structures already differ; if either cannot be rendered, the
			// detail says so rather than an empty diff that would read identical.
			srcPretty, ok1 := prettyValue(srcPlist)
			dstPretty, ok2 := prettyValue(dstPlist)
			if !ok1 || !ok2 {
				return Comparison{Identical: false, Detail: "plist structures differ"}
			}
			return Comparison{Identical: false, Detail: unifiedDiff(srcPretty, dstPretty)}
		}
	}

	// 2. Both readable as UTF-8 text: compare as text.
	if utf8.Valid(srcData) && utf8.Valid(dstData) {
		if bytes.Equal(srcData, dstData) {
			return identical
		}
		return Comparison{Identical: false,
			Detail: unifiedDiff(string(srcData), string(dstData))}
	}

	// 3. Byte-for-byte.
	if bytes.Equal(srcData, dstData) {
		return identical
	}
	return binaryDiff
}

// parsePlist reports whether data is a property list and, if so, its parsed
// structure. It accepts only the XML and binary formats — the two the
// reference's plistlib recognizes — and deliberately rejects the OpenStep and
// GNUstep text formats that howett.net/plist also parses: those are far more
// permissive (a plain `key = value` config could parse), and treating such a
// file as a plist instead of text would diverge from the reference. A non-plist
// input fails to unmarshal outright.
func parsePlist(data []byte) (any, bool) {
	var v any
	format, err := plist.Unmarshal(data, &v)
	if err != nil {
		return nil, false
	}
	return v, format == plist.XMLFormat || format == plist.BinaryFormat
}

// prettyValue renders a parsed structure as deterministic, indented JSON — map
// keys sorted — so the unified diff of two plists is stable, not reordered. The
// bool reports whether the render succeeded; a plist parses into only
// JSON-serializable Go values, so failure is not expected, but the caller must
// not turn a marshal failure into an empty (identical-looking) diff.
func prettyValue(v any) (string, bool) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", false
	}
	return string(out), true
}

// unifiedDiff is a Python-difflib-style unified diff of two texts, labelled
// source vs target, matching the reference's diff shape. The generator writes to
// an in-memory buffer, so its error is not expected; should it occur, a clear
// marker is returned rather than an empty string that would read as identical.
func unifiedDiff(a, b string) string {
	text, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(a),
		B:        difflib.SplitLines(b),
		FromFile: "source",
		ToFile:   "target",
		Context:  3,
	})
	if err != nil {
		return "diff unavailable"
	}
	return text
}
