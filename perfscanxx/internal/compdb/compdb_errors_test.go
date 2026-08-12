package compdb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Load's three error paths are user-facing (perfscanxx surfaces the message
// verbatim), so pin each: a missing file, a malformed JSON body, and a
// syntactically valid but empty database. Only the success path was covered
// before.
func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing file", func(t *testing.T) {
		if _, err := Load(filepath.Join(dir, "does-not-exist.json")); err == nil {
			t.Fatal("Load of a missing path: want error, got nil")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		p := filepath.Join(dir, Name)
		if err := os.WriteFile(p, []byte("NOT JSON{{{"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(p)
		if err == nil {
			t.Fatal("Load of malformed JSON: want error, got nil")
		}
		// The message is prefixed with the file's base name so the user knows
		// which database failed to parse.
		if !strings.Contains(err.Error(), Name) {
			t.Errorf("malformed-JSON error %q must name the file %q", err, Name)
		}
	})

	t.Run("empty database", func(t *testing.T) {
		p := filepath.Join(dir, Name)
		if err := os.WriteFile(p, []byte("[]"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(p)
		if err == nil {
			t.Fatal("Load of an empty array: want error, got nil")
		}
		if !strings.Contains(err.Error(), "empty compilation database") {
			t.Errorf("empty-database error = %q, want it to mention 'empty compilation database'", err)
		}
	})

	t.Run("entries but no usable file is a clear error", func(t *testing.T) {
		// A non-empty array whose entries carry no "file" resolves to zero TUs.
		// Rather than return an empty list (which later reads as "no TUs matched"
		// and misdirects the user), Load reports the malformed database directly.
		p := filepath.Join(dir, Name)
		if err := os.WriteFile(p, []byte(`[{"directory":"/x","file":""},{"directory":"/y"}]`), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(p)
		if err == nil {
			t.Fatal("Load of entries-without-files: want error, got nil")
		}
		if !strings.Contains(err.Error(), "none names a source file") {
			t.Errorf("no-usable-file error = %q, want it to mention 'none names a source file'", err)
		}
	})
}

// TestLoadRelativeDirectoryResolvesToDatabase pins that a spec-violating
// relative (or empty) entry directory resolves the translation unit against the
// database's OWN location, not the process working directory — so results do not
// depend on where perfscanxx happens to be invoked from.
func TestLoadRelativeDirectoryResolvesToDatabase(t *testing.T) {
	dbDir := t.TempDir()
	p := filepath.Join(dbDir, Name)
	// directory "." (relative) + file "sub/a.cpp": the TU is dbDir/sub/a.cpp.
	if err := os.WriteFile(p, []byte(`[{"directory":".","file":"sub/a.cpp"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Run from an unrelated working directory to prove CWD is not consulted.
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	tus, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(dbDir, "sub", "a.cpp")
	if len(tus) != 1 || tus[0] != want {
		t.Errorf("Load = %v, want [%s] (resolved against the database dir, not CWD)", tus, want)
	}
}
