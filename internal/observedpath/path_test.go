package observedpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalObservedIdentity(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, selected := range []string{root, path} {
		physical, err := Canonical(selected)
		if err != nil {
			t.Fatal(err)
		}
		before, err := os.Stat(selected)
		if err != nil {
			t.Fatal(err)
		}
		after, err := os.Stat(physical)
		if err != nil || !os.SameFile(before, after) {
			t.Fatalf("physical identity changed: %q %v", physical, err)
		}
	}
	if _, err := Canonical(filepath.Join(root, "missing.go")); err == nil {
		t.Fatal("missing path fell back to unobserved spelling")
	}
}
