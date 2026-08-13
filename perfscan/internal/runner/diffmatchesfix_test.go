package runner

import (
	"strings"
	"testing"
)

// TestDiffPreviewMatchesFixResult pins the invariant -diff mode advertises: the
// preview is exactly what -fix would write. Both diffFixes and applyFixes derive
// their bytes from the same patchedFiles, but nothing tested that the RENDERED
// -diff output corresponds to the -fix RESULT. Here we run -fix to capture the
// bytes it writes, run -diff to capture the preview (asserting the file is left
// untouched and exit is 1), and assert the preview equals the production
// unifiedDiff of orig -> fix-result. A divergence between the two paths, or an
// edit lost in the diff renderer, fails here.
//
// The composite spans checks that add imports (PS3104 slices, PS2129 io via the
// fmt.Fprint rewrite), prune them (fmt, sort orphaned), and a plain in-line
// rewrite (PS2107 Sprintf->strconv), so the diff must faithfully reflect a
// multi-edit, import-reshaping patch.
func TestDiffPreviewMatchesFixResult(t *testing.T) {
	const src = `package p

import (
	"bytes"
	"fmt"
	"sort"
)

func f(buf *bytes.Buffer, fields []string, s string, n int) {
	sort.Strings(fields)
	fmt.Fprint(buf, s)
	_ = fmt.Sprintf("%d", n)
}
`
	fixed := string(runFixMode(t, src))
	if fixed == src {
		t.Fatal("expected -fix to change the file")
	}

	patch, stderr, code, onDisk := runDiffMode(t, src)
	if string(onDisk) != src {
		t.Fatalf("-diff modified the file on disk:\n%s", onDisk)
	}
	if code != 1 {
		t.Fatalf("-diff with pending changes: exit = %d, want 1\nstderr: %s", code, stderr)
	}
	// Reuse the exact path label the runner emitted, so only the hunk bodies —
	// which depend solely on (orig, fix-result) — are under test.
	path := diffHeaderPath(patch)
	if path == "" {
		t.Fatalf("could not parse the path from the -diff header:\n%s", patch)
	}
	want := unifiedDiff(path, []byte(src), []byte(fixed))
	if patch != want {
		t.Errorf("-diff preview != unifiedDiff(orig, -fix result):\n--- got ---\n%s\n--- want ---\n%s", patch, want)
	}
}

// diffHeaderPath extracts PATH from the "--- a/PATH" line of a unified diff.
func diffHeaderPath(patch string) string {
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "--- a/") {
			return strings.TrimPrefix(line, "--- a/")
		}
	}
	return ""
}
