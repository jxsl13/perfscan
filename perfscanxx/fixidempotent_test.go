package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFixIsIdempotent pins convergence of the -fix pipeline: applying it, then
// applying it again, must leave the source byte-identical — a fix that
// re-matched its own output would make a -fix CI loop oscillate. Uses PX3013
// (modernize-use-equals-default: `~S(){}` -> `~S() = default`) on a headerless
// TU so no sysroot is needed. Skipped when clang-tidy is unavailable.
func TestFixIsIdempotent(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found; skipping -fix idempotency test")
	}
	dir := t.TempDir()
	cpp := filepath.Join(dir, "t.cpp")
	if err := os.WriteFile(cpp, []byte("struct S {\n  ~S() {}\n};\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"clang++ -std=c++17 -c t.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	fix := func() { runCLI("-tidy", bin, "-fix", "-checks", "PX3013", "-p", dir, cpp) }

	fix()
	after1, err := os.ReadFile(cpp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after1), "= default") {
		t.Fatalf("first -fix did not apply PX3013; got:\n%s", after1)
	}

	fix()
	after2, err := os.ReadFile(cpp)
	if err != nil {
		t.Fatal(err)
	}
	if string(after1) != string(after2) {
		t.Errorf("second -fix changed the file — not idempotent:\n--- after pass 1 ---\n%s\n--- after pass 2 ---\n%s", after1, after2)
	}
}
