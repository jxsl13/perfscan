package report

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The exported ResolvePath wrapper plus the branch resolve_test.go misses: a
// relative path with neither a build dir nor a main source is returned as-is.
func TestResolvePathExportedFallback(t *testing.T) {
	if got := ResolvePath("a.cpp", "", ""); got != "a.cpp" {
		t.Errorf(`ResolvePath("a.cpp","","") = %q, want "a.cpp"`, got)
	}
}

// DisplayPath renders an absolute path under the cwd as relative for readable
// output, and leaves everything else (empty, already-relative, the cwd itself,
// and paths outside the cwd) unchanged.
func TestDisplayPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-style path fixtures")
	}
	if got := DisplayPath(""); got != "" {
		t.Errorf("DisplayPath(empty) = %q, want empty", got)
	}
	if got := DisplayPath(filepath.FromSlash("rel/x.cpp")); got != filepath.FromSlash("rel/x.cpp") {
		t.Errorf("DisplayPath(relative) = %q, want it unchanged", got)
	}

	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	// Canonical (symlink-resolved) cwd — on macOS t.TempDir() is under a
	// /var -> /private/var symlink, so build the fixtures from Getwd().
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if got, want := DisplayPath(filepath.Join(cwd, "sub", "x.cpp")), filepath.Join("sub", "x.cpp"); got != want {
		t.Errorf("DisplayPath(under cwd) = %q, want %q", got, want)
	}
	if got := DisplayPath(cwd); got != cwd {
		t.Errorf("DisplayPath(cwd) = %q, want it unchanged (rel is \".\")", got)
	}
	outside := filepath.Join(filepath.Dir(cwd), "elsewhere", "y.cpp")
	if got := DisplayPath(outside); got != outside {
		t.Errorf("DisplayPath(outside cwd) = %q, want it unchanged (rel starts with \"..\")", got)
	}
}
