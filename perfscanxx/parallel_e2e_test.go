package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestParallelMatchesSequentialEndToEnd drives the WHOLE binary (runCLI, real
// clang-tidy — not a stub) over a multi-TU project and asserts that the parallel
// analysis (-j 4) yields byte-identical -json to the sequential one (-j 1). This
// is the end-to-end guarantee behind the -j feature: the split + per-worker
// export + merge + sort path must reproduce a single run exactly, on real
// clang-tidy output. The stub-based TestParallelReportRun covers the orchestration
// shape; this covers the real toolchain. Skips when clang-tidy is unavailable.
func TestParallelMatchesSequentialEndToEnd(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	sysroot := ""
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("xcrun", "--show-sdk-path").Output()
		if err != nil {
			t.Skip("xcrun --show-sdk-path failed; cannot locate the C++ sysroot")
		}
		sysroot = strings.TrimSpace(string(out))
	}

	dir := t.TempDir()
	var files, entries []string
	// A handful of TUs, each with a distinct PX3008 (container-size-empty) finding,
	// so the merge has real cross-worker content to reassemble.
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		p := filepath.Join(dir, n+".cpp")
		src := "#include <vector>\nbool " + n + "f(const std::vector<int>& v){return v.size()==0;}\n"
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
		entries = append(entries, `{"directory":"`+dir+`","file":"`+p+`","command":"clang++ -std=c++17 -c `+n+`.cpp"}`)
	}
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte("["+strings.Join(entries, ",")+"]"), 0o644); err != nil {
		t.Fatal(err)
	}

	runJSON := func(jobs string) (string, int) {
		args := []string{"-tidy", bin, "-checks", "PX3008", "-j", jobs, "-json", "-p", dir}
		if sysroot != "" {
			args = append(args, "-extra-arg=-isysroot", "-extra-arg="+sysroot)
		}
		args = append(args, files...)
		out, errOut, code := runCLI(args...)
		if strings.Contains(errOut, "fatal error:") || strings.Contains(errOut, "file not found") {
			t.Skipf("toolchain could not parse the fixture; skipping:\n%s", errOut)
		}
		return out, code
	}

	par, parCode := runJSON("4")
	seq, seqCode := runJSON("1")

	if strings.TrimSpace(par) == "" || par == "[]" {
		t.Fatalf("parallel run produced no findings (json=%q, exit=%d) — fixture/clang-tidy issue", par, parCode)
	}
	if par != seq {
		t.Errorf("-j 4 and -j 1 json differ (must be byte-identical):\n--- -j 4 ---\n%s\n--- -j 1 ---\n%s", par, seq)
	}
	if parCode != seqCode {
		t.Errorf("exit codes differ: -j4=%d, -j1=%d", parCode, seqCode)
	}
}
