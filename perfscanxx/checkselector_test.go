package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChecksTypoWarnsButStillRuns pins the end-to-end -checks typo behavior
// wired in main.go: a selector mixing a real check with a garbage token must
// (1) print a WARNING naming the unmatched token, and (2) STILL run the valid
// check rather than aborting — so a fat-fingered pattern degrades to a warning,
// never a silent "ran nothing, all clear". The unit test covers
// UnmatchedPatterns() in isolation; this covers the main.go wiring + that the
// run proceeds.
func TestChecksTypoWarnsButStillRuns(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.cpp")
	if err := os.WriteFile(src, []byte("for (auto x : items) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer stubTidy(t, exportOneFinding(src), nil, nil)()

	// PX1001 is real (and the stub reports a finding for it); PX9999 is a typo.
	out, errOut, code := runCLI("-checks", "PX1001,PX9999", src)

	if !strings.Contains(errOut, "PX9999") || !strings.Contains(errOut, "no known check") {
		t.Errorf("expected a typo warning naming PX9999 on stderr, got:\n%s", errOut)
	}
	// The valid PX1001 still ran: its finding was reported, so exit is 1 (not 2,
	// which would mean the selector matched nothing / aborted).
	if code != 1 {
		t.Errorf("run with a valid+typo selector: exit = %d, want 1 (valid check still runs)\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	if !strings.Contains(out+errOut, "PX1001") {
		t.Errorf("the valid PX1001 finding should still be reported, got stdout:\n%s", out)
	}
}
