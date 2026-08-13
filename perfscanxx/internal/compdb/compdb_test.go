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

// TestFindDefaultStart pins the start=="" default: with no -p and no explicit
// start, Find walks upward from the process CWD (start defaults to "."). Chdir
// into a temp dir holding compile_commands.json and assert Find("","") locates
// it — the auto-discovery entry point the CLI uses when invoked as
// `perfscanxx ./...` from a build root.
func TestFindDefaultStart(t *testing.T) {
	dir := t.TempDir()
	db := writeDB(t, dir, []map[string]string{{"directory": dir, "file": "x.cpp"}})

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	got, err := Find("", "")
	if err != nil {
		t.Fatalf("Find(\"\",\"\") = %v, want it to find the db in CWD", err)
	}
	// t.TempDir may sit under a symlinked path (e.g. /var -> /private/var on
	// macOS); compare by resolved basename+dir rather than exact string.
	if filepath.Base(got) != Name {
		t.Errorf("Find(\"\",\"\") = %q, want a %s path", got, Name)
	}
	if gotAbs, _ := filepath.EvalSymlinks(got); gotAbs != "" {
		if wantAbs, _ := filepath.EvalSymlinks(db); wantAbs != gotAbs {
			t.Errorf("Find(\"\",\"\") = %q (resolved %q), want %q (resolved %q)", got, gotAbs, db, wantAbs)
		}
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

// TestLoadArgumentsFormEntry pins that Load enumerates a translation unit from
// an entry that uses the "arguments" ARRAY form instead of the "command" STRING
// form. The JSON Compilation Database spec allows either, and real generators
// (Bazel, and CMake with some generators) emit "arguments". perfscanxx parses
// only directory+file to enumerate TUs — the compile invocation is clang-tidy's
// concern — so the array-valued "arguments" field must be tolerated (ignored) by
// the parser rather than tripping the unmarshal. Mixed forms in one database
// must both enumerate.
func TestLoadArgumentsFormEntry(t *testing.T) {
	dir := t.TempDir()
	// Written by hand because writeDB only models string fields; "arguments" is
	// an array. One arguments-form entry and one command-form entry.
	raw := `[
	{"directory": ` + jsonStr(dir) + `, "file": "a.cpp", "arguments": ["clang++", "-std=c++17", "-c", "a.cpp"]},
	{"directory": ` + jsonStr(dir) + `, "file": "b.cpp", "command": "clang++ -std=c++17 -c b.cpp"}
]`
	p := filepath.Join(dir, Name)
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	tus, err := Load(p)
	if err != nil {
		t.Fatalf("Load with an arguments-form entry: %v", err)
	}
	want := []string{filepath.Join(dir, "a.cpp"), filepath.Join(dir, "b.cpp")}
	if len(tus) != 2 || tus[0] != want[0] || tus[1] != want[1] {
		t.Errorf("Load = %v, want %v (arguments-form and command-form entries must both enumerate)", tus, want)
	}
}

// jsonStr renders s as a JSON string literal (quoted, backslash-escaped) so a
// Windows-style path in a hand-written database fixture stays valid JSON.
func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// TestLoadNormalizesDotDotInFilePath pins that a translation-unit path with a
// ".." segment is normalized (cleaned) when resolved. An OUT-OF-TREE cmake build
// records each TU relative to the build dir, so a source in a sibling tree comes
// out as "../src/a.cpp"; without cleaning, the resolved path would be
// "<build>/../src/a.cpp", which does not string-match a pattern like ./src/... —
// the TU would be silently skipped. Load must resolve it to the clean
// "<root>/src/a.cpp".
func TestLoadNormalizesDotDotInFilePath(t *testing.T) {
	root := t.TempDir()
	build := filepath.Join(root, "build")
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	db := writeDB(t, build, []map[string]string{
		{"directory": build, "file": "../src/a.cpp"}, // out-of-tree: TU is up a level
	})
	tus, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "src", "a.cpp")
	if len(tus) != 1 || tus[0] != want {
		t.Errorf("Load = %v, want [%s] (the '..' must be cleaned so out-of-tree cmake TUs match path patterns)", tus, want)
	}
	if len(tus) == 1 && strings.Contains(tus[0], "..") {
		t.Errorf("resolved TU path still contains '..': %s", tus[0])
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
