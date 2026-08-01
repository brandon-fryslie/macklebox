package drift

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// compareDirs compares two directories recursively by content (appspec/06). They
// are identical only if every regular file matches byte-for-byte and neither
// side has an entry the other lacks. Otherwise the detail lists the differences,
// sorted: "cannot read: <rel>" (present on both but unreadable), "changed:
// <rel>", "only in source: <rel>", "only in target: <rel>". Byte-for-byte is the
// directory contract, not the plist/text/binary cascade single-file comparison
// uses.
//
// "Entry" means any tree member — a regular file, a subdirectory, or a symlink —
// so a directory-structure difference (an empty subdir on one side, an entry
// that is a file on one side and a dir on the other) is a difference, not
// silently ignored. Content is byte-compared only for entries that are regular
// files on both sides; a symlink's target is not further inspected. If either
// tree cannot be fully read, the result is differs/no-detail — an
// incompletely-read tree can never be reported identical. [LAW:no-silent-failure]
func compareDirs(source, dest string) Comparison {
	srcEntries, srcErr := walkEntries(source)
	dstEntries, dstErr := walkEntries(dest)
	if srcErr != nil || dstErr != nil {
		return differsNo // a tree could not be fully read: not provably identical
	}

	var lines []string
	for rel, srcRegular := range srcEntries {
		dstRegular, inDst := dstEntries[rel]
		switch {
		case !inDst:
			lines = append(lines, "only in source: "+rel)
		case srcRegular != dstRegular:
			lines = append(lines, "changed: "+rel) // same path, different kind
		case srcRegular && dstRegular:
			equal, err := fileState(filepath.Join(source, rel), filepath.Join(dest, rel))
			switch {
			case err != nil:
				lines = append(lines, "cannot read: "+rel) // present but uncomparable
			case !equal:
				lines = append(lines, "changed: "+rel)
			}
		}
	}
	for rel := range dstEntries {
		if _, inSrc := srcEntries[rel]; !inSrc {
			lines = append(lines, "only in target: "+rel)
		}
	}
	if len(lines) == 0 {
		return identical
	}
	// Sorting whole lines yields the appspec ordering by construction: the
	// prefixes sort "changed" < "only in source" < "only in target", and equal
	// prefixes sort by name.
	sort.Strings(lines)
	return Comparison{Identical: false, Detail: strings.Join(lines, "\n") + "\n"}
}

// walkEntries maps every entry under root (excluding root itself) to whether it
// is a regular file, keyed by forward-slash relative path so the two sides
// compare on a common key regardless of platform separator. An unreadable entry
// or a failed walk surfaces as an error rather than a silently smaller set.
// [LAW:no-silent-failure]
func walkEntries(root string) (map[string]bool, error) {
	entries := map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // surface an unreadable entry instead of skipping it
		}
		if p == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		entries[filepath.ToSlash(rel)] = d.Type().IsRegular()
		return nil
	})
	return entries, err
}

// fileState reports whether two files are byte-equal, distinguishing a genuine
// difference from an inability to read one of them. Collapsing "unreadable" into
// a false "not equal" would let the caller report a confirmed content change on
// a file that was never actually compared. [LAW:no-silent-failure]
func fileState(a, b string) (equal bool, readErr error) {
	da, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(da, db), nil
}
