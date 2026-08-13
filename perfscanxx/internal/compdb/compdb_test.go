package compdb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// TestFindWalkNotFound covers the auto-discovery walk that reaches the
// filesystem root without finding a build database (buildDir empty): the error
// must name the searched build subdirs so the user knows to pass -p or run
// cmake. A nested start dir with no db in it or any ancestor exercises the
// walk-to-root termination branch (distinct from the -p error TestFindMissing
// hits).
func TestFindWalkNotFound(t *testing.T) {
	start := filepath.Join(t.TempDir(), "a", "b")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Find("", start)
	if err == nil {
		t.Fatal("expected a not-found error when no compile_commands.json exists up to root")
	}
	if !strings.Contains(err.Error(), "no "+Name+" found") {
		t.Errorf("error = %q, want it to mention no %s found", err, Name)
	}
}

// TestLoadParseErrors pins the actionable diagnosis for a malformed database:
// each case must name the likely cause and how to fix it, and must never leak
// the internal Go type name from encoding/json.
func TestLoadParseErrors(t *testing.T) {
	cases := []struct {
		name, content, wantSub string
	}{
		{"object-not-array", `{"directory":"/x","file":"a.cpp"}`, "single JSON object"},
		{"truncated-array", `[{"directory":"/x",`, "malformed JSON"},
		{"lfs-pointer", "version https://git-lfs.github.com/spec/v1\noid sha256:deadbeef", "does not look like JSON"},
		{"empty-file", "", "file is empty"},
		{"bom-then-object", "\xEF\xBB\xBF{}", "single JSON object"},
		// Leading whitespace/newlines before the first real byte must be skipped
		// so the diagnosis keys off the actual content, not the blank prefix.
		{"whitespace-then-object", "\n\n\t  {\"file\":\"a.cpp\"}", "single JSON object"},
		{"whitespace-only", "  \t\r\n  ", "file is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, Name)
			if err := os.WriteFile(p, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(p)
			if err == nil {
				t.Fatalf("Load(%q) succeeded, want error", tc.content)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("error %q does not contain %q", msg, tc.wantSub)
			}
			if !strings.Contains(msg, "cmake") {
				t.Errorf("error %q lacks the regenerate hint", msg)
			}
			if strings.Contains(msg, "compdb.entry") || strings.Contains(msg, "Go value") {
				t.Errorf("error %q leaks the internal Go type name", msg)
			}
		})
	}
}

// TestLoadRelativeDirectoryResolvesAgainstDBDir pins the fallback for a
// non-spec-compliant compile_commands.json whose entries carry a RELATIVE or
// EMPTY "directory" (some generators emit these): the translation unit is
// resolved against the DATABASE's own location, not the process CWD, so
// `perfscanxx -p build` yields the same TUs no matter where it is invoked.
func TestLoadRelativeDirectoryResolvesAgainstDBDir(t *testing.T) {
	root := t.TempDir()
	build := filepath.Join(root, "build")
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	db := writeDB(t, build, []map[string]string{
		{"directory": "", "file": "a.cpp"},    // empty directory -> resolve against build/
		{"directory": "sub", "file": "b.cpp"}, // relative directory -> build/sub/b.cpp
	})
	tus, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(build, "a.cpp"), filepath.Join(build, "sub", "b.cpp")}
	if len(tus) != 2 || tus[0] != want[0] || tus[1] != want[1] {
		t.Errorf("Load = %v, want %v (relative/empty directory must resolve against the db dir)", tus, want)
	}
}

// TestPlural pins the singular/plural suffix used in the "loaded N director(y/ies)"
// message so a count of 1 reads correctly and any other count pluralizes.
func TestPlural(t *testing.T) {
	if got := plural(1); got != "y" {
		t.Errorf("plural(1) = %q, want y", got)
	}
	for _, n := range []int{0, 2, 5} {
		if got := plural(n); got != "ies" {
			t.Errorf("plural(%d) = %q, want ies", n, got)
		}
	}
}
