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
// sorted: "changed: <rel>", "only in source: <rel>", "only in target: <rel>".
// Byte-for-byte is the directory contract, not the plist/text/binary cascade
// that single-file comparison uses.
func compareDirs(source, dest string) Comparison {
	srcFiles := relFileSet(source)
	dstFiles := relFileSet(dest)

	var lines []string
	for rel, srcPath := range srcFiles {
		dstPath, inDst := dstFiles[rel]
		if !inDst {
			lines = append(lines, "only in source: "+rel)
			continue
		}
		if !filesEqual(srcPath, dstPath) {
			lines = append(lines, "changed: "+rel)
		}
	}
	for rel := range dstFiles {
		if _, inSrc := srcFiles[rel]; !inSrc {
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

// relFileSet maps every regular file under root to its absolute path, keyed by
// forward-slash relative path so the two sides compare on a common key
// regardless of platform separator.
func relFileSet(root string) map[string]string {
	files := map[string]string{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if rel, err := filepath.Rel(root, p); err == nil {
				files[filepath.ToSlash(rel)] = p
			}
		}
		return nil
	})
	return files
}

// filesEqual reports whether two files have identical bytes; an unreadable file
// reads as differing.
func filesEqual(a, b string) bool {
	da, ea := os.ReadFile(a)
	db, eb := os.ReadFile(b)
	if ea != nil || eb != nil {
		return false
	}
	return bytes.Equal(da, db)
}
