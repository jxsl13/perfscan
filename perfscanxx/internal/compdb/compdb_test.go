package compdb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindAndLoad(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, Name)
	os.WriteFile(db, []byte(`[
	  {"directory":"`+dir+`","file":"x.cpp"},
	  {"directory":"`+dir+`","file":"`+filepath.Join(dir, "y.cpp")+`"},
	  {"directory":"`+dir+`","file":"x.cpp"}
	]`), 0o644)

	// Find walks up from a nested start.
	got, err := Find("", sub)
	if err != nil || got != db {
		t.Fatalf("Find=%q,%v want %q", got, err, db)
	}
	// -p takes precedence.
	if got, err := Find(dir, "/nowhere"); err != nil || got != db {
		t.Fatalf("Find(-p)=%q,%v want %q", got, err, db)
	}
	// Load resolves + dedups + absolutizes.
	tus, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "x.cpp"), filepath.Join(dir, "y.cpp")}
	if len(tus) != 2 || tus[0] != want[0] || tus[1] != want[1] {
		t.Errorf("Load=%v want %v", tus, want)
	}
}

func TestFindMissing(t *testing.T) {
	if _, err := Find(t.TempDir(), ""); err == nil {
		t.Error("expected error for -p without compile_commands.json")
	}
}
