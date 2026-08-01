package catalog

import (
	"io/fs"
	"strings"
	"testing"
)

func TestFSExposesCfgDefinitionsAtRoot(t *testing.T) {
	entries, err := fs.ReadDir(FS(), ".")
	if err != nil {
		t.Fatalf("ReadDir(.) error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("catalog FS root is empty; want the built-in definitions")
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".cfg") {
			t.Errorf("unexpected catalog entry %q; want only <key>.cfg files", e.Name())
		}
	}
}

func TestMackupSelfDefinitionCoversItsConfig(t *testing.T) {
	// appspec/06 whole-Mackup mode relies on the mackup self-definition naming
	// its own config file.
	body, err := fs.ReadFile(FS(), "mackup.cfg")
	if err != nil {
		t.Fatalf("mackup.cfg missing from the catalog: %v", err)
	}
	if !strings.Contains(string(body), ".mackup.cfg") {
		t.Errorf("mackup.cfg does not cover .mackup.cfg:\n%s", body)
	}
}
