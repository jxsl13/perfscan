package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitApply writes orig to a fresh git repo, applies patch, and returns the file
// contents — the ground truth for "is this a valid unified diff that reproduces
// the target". Skips when git is unavailable.
func gitApply(t *testing.T, orig, patch string) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(git, args...)
		cmd.Dir = dir
		if out, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), e, out)
		}
	}
	run("init", "-q")
	f := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(f, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.diff"), []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}
	// --check first so a malformed patch fails loudly, then apply for real.
	run("apply", "-p1", "--check", "p.diff")
	run("apply", "-p1", "p.diff")
	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

// TestUnifiedCoalescedHunkAppliesCleanly pins the trickiest hunk path: two
// changes closer than 2*context (=6) lines must merge into ONE hunk whose
// combined @@ range spec is correct — verified by having real git apply the
// patch and reproduce the target byte-for-byte.
func TestUnifiedCoalescedHunkAppliesCleanly(t *testing.T) {
	orig := "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\nl11\nl12\n"
	patched := "l1\nl2\nL3\nl4\nl5\nL6\nl7\nl8\nl9\nl10\nl11\nl12\n" // changes at 3 and 6 (gap 2)
	d := Unified("f.txt", "f.txt", []byte(orig), []byte(patched))
	if n := strings.Count(d, "@@ -"); n != 1 {
		t.Fatalf("nearby changes must coalesce into 1 hunk, got %d:\n%s", n, d)
	}
	if got := gitApply(t, orig, d); got != patched {
		t.Errorf("coalesced patch did not reproduce target:\n got %q\nwant %q", got, patched)
	}
}

// TestUnifiedDeletionAppliesCleanly covers a pure deletion (a shape the other
// tests skip): removing interior lines must produce a valid, correct patch.
func TestUnifiedDeletionAppliesCleanly(t *testing.T) {
	orig := "a\nb\nc\nd\ne\nf\ng\n"
	patched := "a\nb\nf\ng\n" // delete c,d,e
	d := Unified("f.txt", "f.txt", []byte(orig), []byte(patched))
	if got := gitApply(t, orig, d); got != patched {
		t.Errorf("deletion patch did not reproduce target:\n got %q\nwant %q\npatch:\n%s", got, patched, d)
	}
}

// TestUnifiedFirstAndLastLineChange exercises the context-clamping boundaries:
// a change on the very first and very last line (leading/trailing context
// truncated at the file edges) must still apply cleanly.
func TestUnifiedFirstAndLastLineChange(t *testing.T) {
	orig := "a\nb\nc\nd\ne\nf\ng\nh\n"
	patched := "A\nb\nc\nd\ne\nf\ng\nH\n" // change first and last (distant -> 2 hunks)
	d := Unified("f.txt", "f.txt", []byte(orig), []byte(patched))
	if got := gitApply(t, orig, d); got != patched {
		t.Errorf("boundary patch did not reproduce target:\n got %q\nwant %q\npatch:\n%s", got, patched, d)
	}
}
