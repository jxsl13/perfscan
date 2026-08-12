package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestFixAppliesPX3023 pins that PX3023 (modernize-shrink-to-fit) actually
// applies its clang-tidy fix-it end-to-end: the copy-and-swap capacity-shed
// idiom std::vector<T>(v).swap(v) is rewritten to v.shrink_to_fit(). PX3023 was
// added to the catalog with HasFix:true on the strength of a manual
// `clang-tidy --fix` run; this makes that a standing regression (the catalog's
// "HasFix true only if a fix-it applies" discipline).
//
// The fixture needs the C++ standard library <vector>, so on darwin it
// discovers the SDK sysroot via xcrun. If the toolchain cannot parse <vector>
// (a stripped CI image without libc++ headers) the test skips rather than
// fails, matching the repo's setup-difference idiom. Skipped when clang-tidy
// is absent.
func TestFixAppliesPX3023(t *testing.T) {
	bin := findClangTidyForTest()
	if bin == "" {
		t.Skip("clang-tidy not found")
	}
	dir := t.TempDir()
	cpp := filepath.Join(dir, "s.cpp")
	const src = "#include <vector>\n" +
		"void f(std::vector<int>& v) {\n" +
		"  std::vector<int>(v).swap(v);\n" +
		"}\n"
	if err := os.WriteFile(cpp, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	compile := "clang++ -std=c++17"
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("xcrun", "--show-sdk-path").Output()
		if err != nil {
			t.Skipf("xcrun --show-sdk-path failed (%v); cannot locate the C++ sysroot", err)
		}
		compile += " -isysroot " + strings.TrimSpace(string(out))
	}
	compile += " -c s.cpp"
	cc := `[{"directory":"` + dir + `","file":"` + cpp + `","command":"` + compile + `"}]`
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(cc), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, _ := runCLI("-tidy", bin, "-fix", "-checks", "PX3023", "-p", dir, cpp)

	got, err := os.ReadFile(cpp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "shrink_to_fit") {
		if strings.Contains(string(got), ".swap(v)") {
			t.Errorf("PX3023 left the copy-and-swap idiom in place alongside the fix:\n%s", got)
		}
		return // fix applied cleanly — success
	}

	// Not applied. Distinguish a genuine regression from an environment that
	// cannot parse <vector> (no libc++ headers): clang-tidy prints a fatal
	// "file not found" / "fatal error" for the missing header, and applies no
	// fix-it when the TU does not parse.
	if strings.Contains(stderr, "file not found") || strings.Contains(stderr, "fatal error") {
		t.Skipf("toolchain could not parse <vector>; skipping PX3023 apply check. stderr:\n%s", stderr)
	}
	t.Errorf("PX3023 -fix did not rewrite std::vector<int>(v).swap(v) to shrink_to_fit():\n%s", got)
}
