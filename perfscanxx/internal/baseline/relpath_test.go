package baseline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/report"
)

// relPath normalizes a finding's file to a cwd-relative, slash-separated key so
// baseline entries match across runs regardless of the absolute path; a path
// outside the cwd falls back to the slash-normalized path itself.
func TestRelPath(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd() // canonical (symlink-resolved) cwd
	if err != nil {
		t.Fatal(err)
	}

	if got := relPath(filepath.Join(cwd, "src", "a.cpp")); got != "src/a.cpp" {
		t.Errorf("relPath(under cwd) = %q, want src/a.cpp", got)
	}
	outside := filepath.Join(filepath.Dir(cwd), "elsewhere", "b.cpp")
	if got := relPath(outside); got != filepath.ToSlash(outside) {
		t.Errorf("relPath(outside cwd) = %q, want %q (slash-normalized fallback)", got, filepath.ToSlash(outside))
	}
}

// Write must surface an os error rather than silently reporting success.
func TestWriteError(t *testing.T) {
	dir := t.TempDir()
	notADir := filepath.Join(dir, "afile")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Parent path is a regular file, so os.WriteFile under it fails.
	badPath := filepath.Join(notADir, "baseline.yaml")
	if _, err := Write(badPath, []report.Finding{f("x.cpp", "PX1", "m")}); err == nil {
		t.Error("Write under a non-directory path: want error, got nil")
	}
}
