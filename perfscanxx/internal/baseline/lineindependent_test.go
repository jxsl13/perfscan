package baseline

import (
	"path/filepath"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/report"
)

// TestFilterIsLineIndependent pins the ratchet's central, documented promise:
// finding identity is {file, id, message} — deliberately line-INDEPENDENT — so
// an unrelated edit that shifts a baselined finding to a different line does NOT
// resurrect it. The other baseline tests use the f() helper, which leaves Line
// at 0, so none of them actually exercise a line shift; adding f.Line to keyOf
// (a plausible "make it precise" change) would break the ratchet — CI passing on
// unrelated diffs is the entire value proposition — with no test to catch it.
func TestFilterIsLineIndependent(t *testing.T) {
	fl := func(file, id, msg string, line int) report.Finding {
		return report.Finding{File: file, ID: id, Message: msg, Line: line}
	}
	path := filepath.Join(t.TempDir(), "bl.yaml")
	if _, err := Write(path, []report.Finding{fl("a.cpp", "PX1001", "copy", 10)}); err != nil {
		t.Fatal(err)
	}

	kept, suppressed, err := Filter(path, []report.Finding{
		fl("a.cpp", "PX1001", "copy", 99),        // same finding, shifted 10 -> 99: still suppressed
		fl("a.cpp", "PX1002", "value param", 10), // a genuinely different finding: reported
	})
	if err != nil {
		t.Fatal(err)
	}
	if suppressed != 1 {
		t.Errorf("a line-shifted baselined finding must stay suppressed; suppressed=%d", suppressed)
	}
	if len(kept) != 1 || kept[0].ID != "PX1002" {
		t.Errorf("a different finding must surface regardless of line; kept=%v", kept)
	}
}
