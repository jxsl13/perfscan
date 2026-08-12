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

	t.Run("only empty-file entries yield no TUs, no error", func(t *testing.T) {
		// A non-empty array whose entries carry no "file" is not an "empty
		// database" error (entries exist); it simply resolves to zero TUs.
		p := filepath.Join(dir, Name)
		if err := os.WriteFile(p, []byte(`[{"directory":"/x","file":""}]`), 0o644); err != nil {
			t.Fatal(err)
		}
		tus, err := Load(p)
		if err != nil {
			t.Fatalf("Load: unexpected error %v", err)
		}
		if len(tus) != 0 {
			t.Errorf("Load = %v, want no translation units", tus)
		}
	})
}
