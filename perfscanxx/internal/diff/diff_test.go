package diff

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestUnifiedEqual(t *testing.T) {
	if got := Unified("f", "f", []byte("same\n"), []byte("same\n")); got != "" {
		t.Errorf("equal input should yield empty diff, got %q", got)
	}
	if got := Unified("f", "f", nil, nil); got != "" {
		t.Errorf("empty input should yield empty diff, got %q", got)
	}
}

// TestUnifiedEmptySide pins splitLines' empty-input branch via the real diff:
// an empty original diffed against content (a file created / fully populated)
// and content diffed against empty (a file emptied). Both must render a valid
// unified diff that adds/removes every line, not a panic or an empty result.
func TestUnifiedEmptySide(t *testing.T) {
	created := Unified("f", "f", nil, []byte("added one\nadded two\n"))
	if created == "" || !strings.HasPrefix(created, "--- a/f\n+++ b/f\n") ||
		!strings.Contains(created, "+added one\n") || !strings.Contains(created, "+added two\n") {
		t.Errorf("empty->content diff malformed:\n%s", created)
	}
	if strings.Contains(created, "-added") {
		t.Errorf("empty->content diff must not delete anything:\n%s", created)
	}

	emptied := Unified("f", "f", []byte("gone one\ngone two\n"), nil)
	if emptied == "" || !strings.Contains(emptied, "-gone one\n") || !strings.Contains(emptied, "-gone two\n") {
		t.Errorf("content->empty diff malformed:\n%s", emptied)
	}
	if strings.Contains(emptied, "+gone") {
		t.Errorf("content->empty diff must not add anything:\n%s", emptied)
	}
}

func TestUnifiedSingleLineChange(t *testing.T) {
	orig := "line one\nline two\nline three\n"
	patched := "line one\nline TWO\nline three\n"
	got := Unified("a.txt", "a.txt", []byte(orig), []byte(patched))
	want := "--- a/a.txt\n" +
		"+++ b/a.txt\n" +
		"@@ -1,3 +1,3 @@\n" +
		" line one\n" +
		"-line two\n" +
		"+line TWO\n" +
		" line three\n"
	if got != want {
		t.Errorf("diff mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestUnifiedInsertion(t *testing.T) {
	orig := "a\nb\nc\n"
	patched := "a\nb\ninserted\nc\n"
	got := Unified("f", "f", []byte(orig), []byte(patched))
	want := "--- a/f\n" +
		"+++ b/f\n" +
		"@@ -1,3 +1,4 @@\n" +
		" a\n" +
		" b\n" +
		"+inserted\n" +
		" c\n"
	if got != want {
		t.Errorf("diff mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestUnifiedNoNewlineAtEOF(t *testing.T) {
	orig := "keep\nold"      // no trailing newline
	patched := "keep\nnew\n" // adds newline, changes last line
	got := Unified("f", "f", []byte(orig), []byte(patched))
	want := "--- a/f\n" +
		"+++ b/f\n" +
		"@@ -1,2 +1,2 @@\n" +
		" keep\n" +
		"-old\n" +
		"\\ No newline at end of file\n" +
		"+new\n"
	if got != want {
		t.Errorf("diff mismatch:\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestUnifiedPatchedNoNewlineAtEOF(t *testing.T) {
	orig := "a\nb\n"
	patched := "a\nB" // last line changed AND trailing newline removed
	got := Unified("f", "f", []byte(orig), []byte(patched))
	want := "--- a/f\n" +
		"+++ b/f\n" +
		"@@ -1,2 +1,2 @@\n" +
		" a\n" +
		"-b\n" +
		"+B\n" +
		"\\ No newline at end of file\n"
	if got != want {
		t.Errorf("diff mismatch:\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestUnifiedTwoDistantChangesTwoHunks(t *testing.T) {
	orig := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n"
	patched := "1\nX\n3\n4\n5\n6\n7\n8\n9\n10\nY\n12\n"
	got := Unified("f", "f", []byte(orig), []byte(patched))
	if n := strings.Count(got, "@@ -"); n != 2 {
		t.Errorf("distant changes should yield 2 hunks, got %d:\n%s", n, got)
	}
	if !strings.Contains(got, "-2\n") || !strings.Contains(got, "+X\n") ||
		!strings.Contains(got, "-11\n") || !strings.Contains(got, "+Y\n") {
		t.Errorf("hunks missing expected edits:\n%s", got)
	}
}

// TestUnifiedIsValidPatch feeds the rendered diff to `patch` (if available) and
// confirms it reproduces the patched bytes — the real acceptance test.
func TestUnifiedIsValidPatch(t *testing.T) {
	patchBin, err := exec.LookPath("patch")
	if err != nil {
		t.Skip("patch not available")
	}
	orig := "one\ntwo\nthree\nfour\nfive\n"
	patched := "one\nTWO\nthree\nfour\nFIVE\n"
	diff := Unified("s.txt", "s.txt", []byte(orig), []byte(patched))

	dir := t.TempDir()
	// Reconstruct the file at the path the diff expects (strip a/ b/ with -p1).
	src := dir + "/s.txt"
	if err := os.WriteFile(src, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(patchBin, "-p1", "-d", dir)
	cmd.Stdin = strings.NewReader(diff)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("patch rejected diff: %v\n%s\ndiff:\n%s", err, out, diff)
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != patched {
		t.Errorf("patched file = %q, want %q", got, patched)
	}
}

// TestUnifiedMidFileDeletionIsValidPatch pins a mid-file PURE DELETION — lines
// removed from the middle, none added, surrounding lines kept. This is the shape
// a real perfscanxx fix produces (PX3026 removes an out-of-line
// `T::~T() = default;` definition from a .cc, keeping the code around it), and it
// is distinct from the covered mid-file cases (single-line replace, insertion):
// the hunk carries only '-' lines in the changed region. It must render a valid
// patch that the real `patch` binary applies to reproduce the shortened file.
func TestUnifiedMidFileDeletionIsValidPatch(t *testing.T) {
	patchBin, err := exec.LookPath("patch")
	if err != nil {
		t.Skip("patch not available")
	}
	orig := "keep1\ndrop1\ndrop2\nkeep2\n"
	patched := "keep1\nkeep2\n"
	diff := Unified("s.txt", "s.txt", []byte(orig), []byte(patched))

	if !strings.Contains(diff, "-drop1\n") || !strings.Contains(diff, "-drop2\n") {
		t.Errorf("deletion hunk missing the removed lines:\n%s", diff)
	}
	if strings.Contains(diff, "+drop") {
		t.Errorf("a pure deletion must not re-add the dropped lines:\n%s", diff)
	}

	dir := t.TempDir()
	src := dir + "/s.txt"
	if err := os.WriteFile(src, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(patchBin, "-p1", "-d", dir)
	cmd.Stdin = strings.NewReader(diff)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("patch rejected the deletion diff: %v\n%s\ndiff:\n%s", err, out, diff)
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != patched {
		t.Errorf("patched file = %q, want %q", got, patched)
	}
}

// TestUnifiedContextLineNoFinalNewline pins the "\ No newline at end of file"
// marker on a CONTEXT line (the change is on an earlier line, so the newline-less
// last line appears unchanged in the hunk) — the writeHunk ' ' branch that the
// last-line-changed cases don't reach.
func TestUnifiedContextLineNoFinalNewline(t *testing.T) {
	// "ccc" is unchanged context, and neither side ends in a newline.
	got := Unified("f", "f", []byte("aaa\nbbb\nccc"), []byte("AAA\nbbb\nccc"))
	if !strings.Contains(got, " ccc\n\\ No newline at end of file\n") {
		t.Errorf("expected the context-line no-newline marker after \" ccc\":\n%s", got)
	}
	// Sanity: the change itself is rendered too.
	if !strings.Contains(got, "-aaa\n") || !strings.Contains(got, "+AAA\n") {
		t.Errorf("expected the -aaa/+AAA change lines:\n%s", got)
	}
}

// TestUnifiedTrailingNewlineOnly pins the len(hunks)==0 branch: when the two
// inputs differ ONLY in the trailing newline (same single line content), there
// is no LINE-level difference, so Unified returns "" — the newline flip is
// captured in the noNL flags but has no hunk to hang a "\ No newline" marker on.
// -diff therefore shows nothing for a fix that only toggles a trailing newline,
// which is safe: no spurious or misleading diff. Both directions return "".
func TestUnifiedTrailingNewlineOnly(t *testing.T) {
	if got := Unified("f", "f", []byte("abc"), []byte("abc\n")); got != "" {
		t.Errorf(`Unified("abc","abc\n") = %q, want "" (no line-level difference)`, got)
	}
	if got := Unified("f", "f", []byte("abc\n"), []byte("abc")); got != "" {
		t.Errorf(`Unified("abc\n","abc") = %q, want ""`, got)
	}
}

// TestUnifiedContextLineNoNewlineOnBSide pins the writeHunk branch where the
// "\ No newline at end of file" marker attaches to a CONTEXT line for the
// PATCHED (B) side: the patched file's last line is unchanged but lacks a
// trailing newline, while the original DOES have one (so the a-side condition is
// false and rendering falls to the b-side else-if). The marker must follow the
// context line, not a +/- line.
func TestUnifiedContextLineNoNewlineOnBSide(t *testing.T) {
	got := Unified("f", "f", []byte("aaa\nkeep\n"), []byte("bbb\nkeep"))
	if !strings.Contains(got, " keep\n\\ No newline at end of file\n") {
		t.Errorf("expected the no-newline marker on the b-side context line:\n%s", got)
	}
	// The change line itself must not carry the marker (its a-side "aaa" line had
	// a trailing newline).
	if strings.Contains(got, "-aaa\n\\ No newline") {
		t.Errorf("no-newline marker wrongly attached to the -aaa line:\n%s", got)
	}
}

// TestDiffHunksEmptyInputs pins diffHunks's len(raw)==0 guard directly: two
// empty (or identical) line sets produce no LCS ops, so diffHunks returns nil
// rather than an empty-hunk artifact. Unified reaches this only after its
// byte-level bytes.Equal early return, so exercise diffHunks itself.
func TestDiffHunksEmptyInputs(t *testing.T) {
	if h := diffHunks(nil, nil, 3); h != nil {
		t.Errorf("diffHunks(nil, nil) = %v, want nil", h)
	}
	if h := diffHunks([]string{}, []string{}, 3); h != nil {
		t.Errorf("diffHunks([], []) = %v, want nil", h)
	}
	// Identical non-empty inputs: all context, no change -> also nil.
	if h := diffHunks([]string{"a", "b"}, []string{"a", "b"}, 3); h != nil {
		t.Errorf("diffHunks(identical) = %v, want nil", h)
	}
}

// TestUnifiedAbsolutePathHeader pins the fix for the out-of-tree `-p build`
// case: when the file path is ABSOLUTE (the compilation database records files
// outside the working directory), the header must NOT carry git's a/ b/ prefix
// — "--- a//abs" is meaningless and applies nowhere (git apply rejects absolute
// paths, patch -p1 strips into a non-existent path). A bare absolute header is a
// plain `diff -u` that applies with `patch -p0`. Regression for the double-slash
// bug seen on corpus scans.
func TestUnifiedAbsolutePathHeader(t *testing.T) {
	dir := t.TempDir()
	abs := dir + "/sample.cc"
	orig := "struct X {\n  ~X() {}\n};\n"
	patched := "struct X {\n  ~X() = default;\n};\n"
	if err := os.WriteFile(abs, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	got := Unified(abs, abs, []byte(orig), []byte(patched))
	if strings.Contains(got, "a//") || strings.Contains(got, "b//") {
		t.Fatalf("absolute header must not double-slash; got:\n%s", got)
	}
	if !strings.HasPrefix(got, "--- "+abs+"\n+++ "+abs+"\n") {
		t.Fatalf("absolute path should get a bare (unprefixed) header; got:\n%s", got)
	}

	// It must actually apply. `patch -p0` locates the absolute file directly.
	if _, err := exec.LookPath("patch"); err != nil {
		t.Skip("patch not available")
	}
	cmd := exec.Command("patch", "-p0")
	cmd.Stdin = strings.NewReader(got)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("patch -p0 failed to apply the preview: %v\n%s", err, out)
	}
	after, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != patched {
		t.Fatalf("applied result mismatch:\ngot:  %q\nwant: %q", string(after), patched)
	}
}

// TestUnifiedRelativePathKeepsPrefix pins that a relative path still gets git's
// a/ b/ prefix (git apply / patch -p1 from the tree root) — the fix must not
// regress the in-tree case.
func TestUnifiedRelativePathKeepsPrefix(t *testing.T) {
	got := Unified("util/cache.cc", "util/cache.cc",
		[]byte("a\nb\n"), []byte("a\nc\n"))
	if !strings.HasPrefix(got, "--- a/util/cache.cc\n+++ b/util/cache.cc\n") {
		t.Fatalf("relative path should keep the a/ b/ prefix; got:\n%s", got)
	}
}
