package diff

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestOSFSReadWriteRoundTrip exercises the production OSFS (the real disk FS the
// -diff/-fix snapshot/restore flow uses; unit tests otherwise inject memFS).
// ReadFile round-trips bytes, WriteFile creates a new file at 0644, and — the
// behavior that matters for restore — WriteFile PRESERVES an existing file's
// permission bits rather than clobbering them to 0644 (so restoring a 0600
// source file doesn't silently widen its mode).
func TestOSFSReadWriteRoundTrip(t *testing.T) {
	var fs OSFS
	dir := t.TempDir()

	// New file: created, bytes round-trip, mode defaults to 0644.
	p := filepath.Join(dir, "new.cpp")
	want := []byte("int main() { return 0; }\n")
	if err := fs.WriteFile(p, want); err != nil {
		t.Fatalf("WriteFile(new): %v", err)
	}
	got, err := fs.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("round-trip = %q, want %q", got, want)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o644 {
			t.Errorf("new file mode = %o, want 0644", fi.Mode().Perm())
		}
	}

	// Existing file with a restrictive mode: WriteFile must keep that mode.
	if runtime.GOOS != "windows" {
		secret := filepath.Join(dir, "secret.cpp")
		if err := os.WriteFile(secret, []byte("orig"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := fs.WriteFile(secret, []byte("rewritten")); err != nil {
			t.Fatalf("WriteFile(existing): %v", err)
		}
		fi, err := os.Stat(secret)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("existing file mode = %o, want it preserved at 0600", fi.Mode().Perm())
		}
	}

	// ReadFile of a missing path surfaces an error.
	if _, err := fs.ReadFile(filepath.Join(dir, "nope.cpp")); err == nil {
		t.Error("ReadFile(missing) = nil error, want an error")
	}
}
