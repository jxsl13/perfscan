package runner

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/lint"
)

// -diff is the dry-run twin of -fix: same checks, same -level gating, same
// baseline filtering, but it renders unified diffs instead of writing files
// and signals "changes pending" with exit 1.

const diffGoMod = `module diffcorpus

go 1.23
`

// diffFixable carries one PS3104 site (sort.Ints → slices.Sort, L1
// auto-fix); its fix also rewrites the import, so the diff spans two hunks.
const diffFixable = `package main

import "sort"

func main() {
	xs := []int{3, 1, 2}
	sort.Ints(xs)
	_ = xs
}
`

// diffClean has no findings at any level.
const diffClean = `package main

func main() {}
`

func runDiffMode(t *testing.T, mainSrc string) (stdout, stderr string, code int, onDisk []byte) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", diffGoMod)
	write("main.go", mainSrc)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	var out, errBuf bytes.Buffer
	code = Run(checks.All(), Options{
		Patterns: []string{"./..."},
		MaxLevel: lint.LevelAggressive,
		Diff:     true,
		Stdout:   &out,
		Stderr:   &errBuf,
	})
	onDisk, err = os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	return out.String(), errBuf.String(), code, onDisk
}

func TestDiffModeShowsPendingFixWithoutApplying(t *testing.T) {
	stdout, stderr, code, onDisk := runDiffMode(t, diffFixable)

	// (a) dry run: the file on disk is byte-for-byte untouched.
	if string(onDisk) != diffFixable {
		t.Errorf("-diff must not modify files; on disk now:\n%s", onDisk)
	}
	// (c) changes pending → exit 1.
	if code != 1 {
		t.Fatalf("want exit 1 with a pending fix, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	// (b) a valid unified diff with the expected old/new lines.
	for _, want := range []string{"--- a/main.go\n", "+++ b/main.go\n", "@@ -"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("diff output missing %q:\n%s", want, stdout)
		}
	}
	var sawMinus, sawPlus bool
	for line := range strings.Lines(stdout) {
		if strings.HasPrefix(line, "-") && strings.Contains(line, "sort.Ints(xs)") {
			sawMinus = true
		}
		if strings.HasPrefix(line, "+") && strings.Contains(line, "slices.Sort(xs)") {
			sawPlus = true
		}
	}
	if !sawMinus || !sawPlus {
		t.Errorf("expected -sort.Ints(xs) / +slices.Sort(xs) lines, got:\n%s", stdout)
	}
	// Findings text is suppressed in diff mode — stdout is only the patch.
	if strings.Contains(stdout, "PS3104") {
		t.Errorf("findings text must not pollute the patch on stdout:\n%s", stdout)
	}
	if !strings.Contains(stderr, "would change") {
		t.Errorf("expected a pending-changes summary on stderr, got:\n%s", stderr)
	}
}

func TestDiffModeCleanFileNoDiffExitZero(t *testing.T) {
	stdout, stderr, code, onDisk := runDiffMode(t, diffClean)
	if string(onDisk) != diffClean {
		t.Errorf("-diff must not modify files; on disk now:\n%s", onDisk)
	}
	if code != 0 {
		t.Fatalf("want exit 0 on a clean tree, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if stdout != "" {
		t.Errorf("clean tree must print no diff, got:\n%s", stdout)
	}
}

func TestDiffAndFixAreMutuallyExclusive(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := Run(checks.All(), Options{
		Patterns: []string{"./..."},
		Diff:     true,
		Fix:      true,
		Stdout:   &out,
		Stderr:   &errBuf,
	})
	if code != 2 {
		t.Fatalf("want exit 2 for -diff -fix, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "-diff and -fix are mutually exclusive") {
		t.Errorf("expected mutual-exclusion message, got:\n%s", errBuf.String())
	}
}

func TestUnifiedDiffRendering(t *testing.T) {
	a := []byte("one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n")
	b := []byte("one\ntwo\nthree\nfour\nFIVE\nsix\nseven\neight\nnine\nten\n")
	got := unifiedDiff("f.txt", a, b)
	want := `--- a/f.txt
+++ b/f.txt
@@ -2,7 +2,7 @@
 two
 three
 four
-five
+FIVE
 six
 seven
 eight
`
	if got != want {
		t.Errorf("unified diff mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestUnifiedDiffMergesNearbyChangesAndMarksNoNewline(t *testing.T) {
	a := []byte("a\nb\nc\nd\ne\nf\ng")
	b := []byte("a\nB\nc\nd\ne\nF\ng")
	got := unifiedDiff("f.txt", a, b)
	// Changes 2 lines apart (< 2*context equal lines between) merge into
	// one hunk; the unterminated last line gets the standard marker.
	want := `--- a/f.txt
+++ b/f.txt
@@ -1,7 +1,7 @@
 a
-b
+B
 c
 d
 e
-f
+F
 g
\ No newline at end of file
`
	if got != want {
		t.Errorf("unified diff mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
	if unifiedDiff("f.txt", a, a) != "" {
		t.Error("equal contents must render an empty diff")
	}
}

// TestUnifiedDiffAsymmetricNoNewlineOnChangedLine pins the real perfscan case the
// symmetric test above misses: the ORIGINAL file lacks a final newline, but the
// fixed output has one because format.Source always terminates the file. When a
// fix rewrites that last line, the diff must attach `\ No newline at end of file`
// ONLY to the removed (unterminated) side and leave the added (terminated) side
// unmarked — exactly how git/patch represent gaining a trailing newline. A
// regression that keyed the marker off the wrong side, or emitted it for both,
// would produce a patch git apply rejects.
func TestUnifiedDiffAsymmetricNoNewlineOnChangedLine(t *testing.T) {
	a := []byte("keep\nold")   // no final newline; last line "old"
	b := []byte("keep\nnew\n") // format.Source added the newline; last line "new"
	got := unifiedDiff("f.txt", a, b)
	want := `--- a/f.txt
+++ b/f.txt
@@ -1,2 +1,2 @@
 keep
-old
\ No newline at end of file
+new
`
	if got != want {
		t.Errorf("asymmetric no-newline diff mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestUnifiedDiffSplitsFarApartChangesIntoSeparateHunks pins the multi-hunk path:
// two changes separated by more than 2*diffContext (6) equal lines must render as
// TWO independent @@ hunks, each with its own correct start line and count. This
// is the offset-computation the single-hunk (TestUnifiedDiffRendering) and
// merge-nearby (TestUnifiedDiffMergesNearbyChangesAndMarksNoNewline) cases do not
// exercise: the SECOND hunk's `-12` start comes from aOff/bOff accumulated across
// the first change and the intervening context, so an off-by-one there would make
// the diff misapply (git apply / patch reject it) while the +/- lines still look
// right. 13 equal lines between the two edits forces the split.
func TestUnifiedDiffSplitsFarApartChangesIntoSeparateHunks(t *testing.T) {
	a := []byte("chg-A-old\n" + strings.Repeat("eq\n", 13) + "chg-B-old\n")
	b := []byte("chg-A-new\n" + strings.Repeat("eq\n", 13) + "chg-B-new\n")
	got := unifiedDiff("f.txt", a, b)
	want := `--- a/f.txt
+++ b/f.txt
@@ -1,4 +1,4 @@
-chg-A-old
+chg-A-new
 eq
 eq
 eq
@@ -12,4 +12,4 @@
 eq
 eq
 eq
-chg-B-old
+chg-B-new
`
	if got != want {
		t.Errorf("multi-hunk diff mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
	if n := strings.Count(got, "@@ -"); n != 2 {
		t.Errorf("expected exactly 2 hunks, got %d:\n%s", n, got)
	}
}

// TestUnifiedDiffIsValidPatch feeds a rendered diff to the real `patch` binary
// (skipped if absent) and confirms it reproduces the patched bytes — the acceptance
// test the exact-string cases above cannot give: it proves the hunk headers'
// line counts and offsets are correct, not merely that the +/- lines match a
// snapshot. Covers the realistic multi-hunk + mid-file-deletion shape a multi-fix
// run produces.
func TestUnifiedDiffIsValidPatch(t *testing.T) {
	patchBin, err := exec.LookPath("patch")
	if err != nil {
		t.Skip("patch not available")
	}
	// region 1: change line 2 (-> X) and delete line 3; region 2 (far apart, 7
	// equal lines away -> a separate hunk): change line 11 (-> Y). Two hunks, one
	// carrying a deletion.
	orig := []byte("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n")
	patched := []byte("1\nX\n4\n5\n6\n7\n8\n9\n10\nY\n12\n")
	diff := unifiedDiff("s.txt", orig, patched)
	if n := strings.Count(diff, "@@ -"); n < 2 {
		t.Fatalf("expected a multi-hunk diff, got %d hunk(s):\n%s", n, diff)
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "s.txt")
	if err := os.WriteFile(src, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"-p1", "-d", dir}
	if runtime.GOOS == "windows" {
		// Git-for-Windows ships GNU patch in text mode by default. --binary
		// preserves the LF bytes produced by unifiedDiff.
		args = append(args, "--binary")
	}
	cmd := exec.Command(patchBin, args...)
	cmd.Stdin = strings.NewReader(diff)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("patch rejected the diff: %v\n%s\ndiff:\n%s", err, out, diff)
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, patched) {
		t.Errorf("patched file = %q, want %q\ndiff:\n%s", got, patched, diff)
	}
}

func TestUnifiedDiffInsertionAtStart(t *testing.T) {
	a := []byte("x\ny\n")
	b := []byte("new\nx\ny\n")
	got := unifiedDiff("f.txt", a, b)
	want := `--- a/f.txt
+++ b/f.txt
@@ -1,2 +1,3 @@
+new
 x
 y
`
	if got != want {
		t.Errorf("unified diff mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}
