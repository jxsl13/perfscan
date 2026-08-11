package compdb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeDB marshals entries to compile_commands.json under dir (json.Marshal
// escapes Windows backslashes correctly, unlike string concatenation).
func writeDB(t *testing.T, dir string, entries []map[string]string) string {
	t.Helper()
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, Name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFindAndLoad(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	db := writeDB(t, dir, []map[string]string{
		{"directory": dir, "file": "x.cpp"},                     // relative -> resolved
		{"directory": dir, "file": filepath.Join(dir, "y.cpp")}, // absolute
		{"directory": dir, "file": "x.cpp"},                     // duplicate
	})

	if got, err := Find("", sub); err != nil || got != db {
		t.Fatalf("Find=%q,%v want %q", got, err, db)
	}
	if got, err := Find(dir, filepath.Join(dir, "nowhere")); err != nil || got != db {
		t.Fatalf("Find(-p)=%q,%v want %q", got, err, db)
	}
	tus, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "x.cpp"), filepath.Join(dir, "y.cpp")}
	if len(tus) != 2 || tus[0] != want[0] || tus[1] != want[1] {
		t.Errorf("Load=%v want %v", tus, want)
	}
}

func TestFindInBuildSubdir(t *testing.T) {
	root := t.TempDir()
	build := filepath.Join(root, "build")
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	db := writeDB(t, build, []map[string]string{{"directory": build, "file": "x.cpp"}})
	// From the project root (no -p), Find should locate build/compile_commands.json.
	if got, err := Find("", root); err != nil || got != db {
		t.Fatalf("Find(root)=%q,%v want %q", got, err, db)
	}
}

func TestFindMissing(t *testing.T) {
	if _, err := Find(t.TempDir(), ""); err == nil {
		t.Error("expected error for -p without compile_commands.json")
	}
}
