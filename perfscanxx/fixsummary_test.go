package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/report"
)

// TestSummarizeFindings pins the plain-text report summary: a count of findings
// and distinct files when non-empty, a clear "no findings" when clean.
func TestSummarizeFindings(t *testing.T) {
	var buf bytes.Buffer
	summarizeFindings(&buf, []report.Finding{
		{ID: "PX3013", File: "a.h"},
		{ID: "PX3004", File: "a.h"}, // same file -> still 1 file
		{ID: "PX3015", File: "b.cpp"},
	})
	if got := buf.String(); !strings.Contains(got, "3 finding(s) across 2 file(s)") {
		t.Errorf("summary = %q, want '3 finding(s) across 2 file(s)'", got)
	}

	buf.Reset()
	summarizeFindings(&buf, nil)
	if got := strings.TrimSpace(buf.String()); got != "perfscanxx: no findings" {
		t.Errorf("empty summary = %q, want 'perfscanxx: no findings'", got)
	}
}

// TestFixTargets pins the -fix summary counting: only findings that carry a
// fix-it count, and files are de-duplicated (a header fixed via several
// findings is one file). Hermetic — no clang-tidy needed.
func TestFixTargets(t *testing.T) {
	findings := []report.Finding{
		{ID: "PX3013", File: "a.h", Fixes: 1},
		{ID: "PX3013", File: "a.h", Fixes: 1}, // same file, still 1 file
		{ID: "PX3004", File: "b.cpp", Fixes: 2},
		{ID: "PX3020", File: "c.cpp", Fixes: 0}, // advisory, no fix-it: excluded
		{ID: "PX3022", File: "d.cpp", Fixes: 0}, // advisory: excluded
	}
	n, files := fixTargets(findings)
	if n != 3 {
		t.Errorf("fixTargets findings-with-fix = %d, want 3", n)
	}
	if files != 2 {
		t.Errorf("fixTargets distinct files = %d, want 2 (a.h, b.cpp)", files)
	}

	// No fixable findings -> zero, zero (drives the "no fix-it" message).
	none := []report.Finding{{ID: "PX3020", File: "x.cpp", Fixes: 0}}
	if n, files := fixTargets(none); n != 0 || files != 0 {
		t.Errorf("fixTargets(no-fix) = (%d,%d), want (0,0)", n, files)
	}
}
