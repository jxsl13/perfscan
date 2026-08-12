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

// TestExcludeKeepsFixOffIncludedHeader is the regression for -exclude leaking a
// fix into an EXCLUDED header that a non-excluded TU includes. Before the
// --exclude-header-filter wiring, `-fix -exclude deps/` still rewrote deps/dep.h
// (clang-tidy --fix touches any file with a fix-it). Uses PX3013 in a header so
// no sysroot is needed. Skipped when clang-tidy is unavailable.
func TestExcludeKeepsFixOffIncludedHeader(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	dir := t.TempDir()
	proj := filepath.Join(dir, "proj")
	deps := filepath.Join(dir, "deps")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deps, 0o755); err != nil {
		t.Fatal(err)
	}
	const header = "#pragma once\nstruct Dep {\n  ~Dep() {}\n};\n" // PX3013 fixable
	hpath := filepath.Join(deps, "dep.h")
	if err := os.WriteFile(hpath, []byte(header), 0o644); err != nil {
		t.Fatal(err)
	}
	cpp := filepath.Join(proj, "a.cpp")
	if err := os.WriteFile(cpp, []byte("#include \"dep.h\"\nDep* make() { return new Dep(); }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cc := `[{"directory":"` + proj + `","file":"` + cpp + `","command":"clang++ -std=c++17 -I` + deps + ` -c a.cpp"}]`
	if err := os.WriteFile(filepath.Join(proj, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Control: WITHOUT -exclude, clang-tidy fixes the header (proving the setup
	// actually triggers a header fix-it, so the exclude assertion is meaningful).
	runCLI("-tidy", bin, "-fix", "-checks", "PX3013", "-p", proj, proj)
	if got, _ := os.ReadFile(hpath); !strings.Contains(string(got), "= default") {
		t.Skipf("setup did not produce a header fix-it (clang-tidy header scope differs); got:\n%s", got)
	}
	// Reset and run WITH -exclude deps/ — the header must be left untouched.
	if err := os.WriteFile(hpath, []byte(header), 0o644); err != nil {
		t.Fatal(err)
	}
	runCLI("-tidy", bin, "-fix", "-checks", "PX3013", "-exclude", "deps/", "-p", proj, proj)
	if got, _ := os.ReadFile(hpath); string(got) != header {
		t.Errorf("-exclude deps/ did not keep -fix off the excluded header:\n%s", got)
	}
}

// TestDiffExcludeKeepsHeaderOutOfPreview pins that -diff honors -exclude for an
// included header exactly as -fix does (both flow the same --exclude-header-filter):
// the preview must not show a change to an excluded header, or -diff would promise
// a change -fix would (correctly) refuse to make. Skipped when clang-tidy absent.
func TestDiffExcludeKeepsHeaderOutOfPreview(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	dir := t.TempDir()
	proj := filepath.Join(dir, "proj")
	deps := filepath.Join(dir, "deps")
	for _, d := range []string{proj, deps} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(deps, "dep.h"), []byte("#pragma once\nstruct Dep {\n  ~Dep() {}\n};\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "a.cpp"), []byte("#include \"dep.h\"\nDep* make() { return new Dep(); }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cc := `[{"directory":"` + proj + `","file":"` + filepath.Join(proj, "a.cpp") + `","command":"clang++ -std=c++17 -I` + deps + ` -c a.cpp"}]`
	if err := os.WriteFile(filepath.Join(proj, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Control: without -exclude, the header change appears in the preview.
	out, _, _ := runCLI("-tidy", bin, "-diff", "-checks", "PX3013", "-p", proj, proj)
	if !strings.Contains(out, "dep.h") {
		t.Skipf("setup did not preview a header change (clang-tidy header scope differs):\n%s", out)
	}
	// With -exclude deps/, the excluded header must NOT appear.
	outEx, _, _ := runCLI("-tidy", bin, "-diff", "-checks", "PX3013", "-exclude", "deps/", "-p", proj, proj)
	if strings.Contains(outEx, "dep.h") {
		t.Errorf("-diff -exclude deps/ still previewed the excluded header:\n%s", outEx)
	}
}
