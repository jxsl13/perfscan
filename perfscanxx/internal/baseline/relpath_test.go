package baseline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/report"
)

// relPath normalizes a finding's file to a slash-separated key relative to the
// anchor directory (the baseline file's directory), so baseline entries match
// across runs regardless of the invocation CWD. A path under the anchor becomes
// a clean relative key; a path outside the anchor keeps a stable "../"-prefixed
// relative form (still anchor-relative, hence still invocation-independent).
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
	anchor := cwd

	// Absolute path under the anchor.
	if got := relPath(filepath.Join(cwd, "src", "a.cpp"), anchor); got != "src/a.cpp" {
		t.Errorf("relPath(under anchor) = %q, want src/a.cpp", got)
	}
	// A CWD-relative finding path (what report.displayPath emits) resolves to
	// the same key via filepath.Abs against the current CWD.
	if got := relPath(filepath.ToSlash(filepath.Join("src", "a.cpp")), anchor); got != "src/a.cpp" {
		t.Errorf("relPath(cwd-relative) = %q, want src/a.cpp", got)
	}
	// A path outside the anchor keeps a stable ../-relative key (not the raw
	// absolute path): anchor-relative, so still invocation-independent.
	outside := filepath.Join(filepath.Dir(cwd), "elsewhere", "b.cpp")
	if got := relPath(outside, anchor); got != "../elsewhere/b.cpp" {
		t.Errorf("relPath(outside anchor) = %q, want ../elsewhere/b.cpp", got)
	}
}

// TestKeyIsInvocationCWDIndependent is the regression pin for the baseline
// CWD-instability fix: a baseline written from one directory must still suppress
// the same findings when the check is later run from a DIFFERENT directory (e.g.
// written at the repo root, run from build/ in CI), as long as the baseline file
// is addressed by the same absolute path. Previously the key was CWD-relative, so
// every baselined finding resurfaced as a false regression under a different CWD.
func TestKeyIsInvocationCWDIndependent(t *testing.T) {
	root := t.TempDir()
	// Resolve symlinks so the paths we build match os.Getwd()'s canonical form
	// (macOS /var -> /private/var), otherwise filepath.Abs vs the finding path
	// diverge for reasons unrelated to what we're testing.
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	baselinePath := filepath.Join(root, ".perfscanxx-baseline.yaml") // addressed absolutely

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	// Write the baseline from the repo ROOT. The finding's File is what
	// report.displayPath would produce from the root cwd: "src/a.cpp".
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(baselinePath, []report.Finding{f("src/a.cpp", "PX1001", "copy")}); err != nil {
		t.Fatal(err)
	}

	// Now run the check from the build/ SUBDIR. From here report.displayPath
	// cannot make /root/src/a.cpp relative (it would need "../"), so the finding
	// carries the ABSOLUTE path — exactly the shape that used to miss.
	if err := os.Chdir(filepath.Join(root, "build")); err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(root, "src", "a.cpp")
	kept, suppressed, err := Filter(baselinePath, []report.Finding{f(abs, "PX1001", "copy")})
	if err != nil {
		t.Fatal(err)
	}
	if suppressed != 1 || len(kept) != 0 {
		t.Errorf("baseline written from root did not suppress the same finding from build/: suppressed=%d kept=%d (want 1, 0) — key is CWD-dependent", suppressed, len(kept))
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
