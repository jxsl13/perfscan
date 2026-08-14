package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestExampleSampleFindings drives the whole binary (real clang-tidy) over the
// COMMITTED examples/sample.cpp and asserts it still reports the two anti-patterns
// the example's README promises — performance-for-range-copy (PX1001) and
// performance-unnecessary-value-param (PX1002). It is a doc-rot guard: an edit to
// sample.cpp that drops an anti-pattern, or a catalog change that stops detecting
// one, would silently break the documented "smoke perfscanxx end-to-end" example.
// compile_commands.json is machine-specific (gitignored), so it is synthesized in
// a temp dir pointing at the real committed source. Skips without clang-tidy.
func TestExampleSampleFindings(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	sample, err := filepath.Abs(filepath.Join("examples", "sample.cpp"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sample); err != nil {
		t.Fatalf("committed example missing: %v", err)
	}
	exDir := filepath.Dir(sample)

	dir := t.TempDir() // holds only the compile db, pointing at the real source
	cc := `[{"directory":"` + exDir + `","file":"` + sample + `","command":"clang++ -std=c++17 -c sample.cpp"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	args := []string{"-tidy", bin, "-checks", "PX1001,PX1002", "-json", "-p", dir}
	if runtime.GOOS == "darwin" {
		out, xerr := exec.Command("xcrun", "--show-sdk-path").Output()
		if xerr != nil {
			t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
		}
		args = append(args, "-extra-arg=-isysroot", "-extra-arg="+strings.TrimSpace(string(out)))
	}
	args = append(args, sample)

	out, errOut, code := runCLI(args...)
	if strings.Contains(errOut, "fatal error:") || strings.Contains(errOut, "file not found") {
		t.Skipf("toolchain could not parse the sample headers; skipping:\n%s", errOut)
	}
	if code != 1 {
		t.Fatalf("example smoke: exit=%d, want 1 (findings); stderr:\n%s", code, errOut)
	}
	for _, want := range []string{`"id": "PX1001"`, `"id": "PX1002"`} {
		if !strings.Contains(out, want) {
			t.Errorf("the documented example no longer reports %s — sample.cpp or the catalog drifted:\n%s", want, out)
		}
	}
}
