package baseline

import (
	"path/filepath"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/report"
)

func f(file, id, msg string) report.Finding {
	return report.Finding{File: file, ID: id, Message: msg}
}

func TestWriteThenFilterSuppressesEverything(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bl.yaml")
	findings := []report.Finding{
		f("a.cpp", "PX1001", "copy"),
		f("a.cpp", "PX2001", "reserve"),
		f("b.cpp", "PX1001", "copy"),
	}
	n, err := Write(path, findings)
	if err != nil || n != 3 {
		t.Fatalf("Write = %d, %v; want 3, nil", n, err)
	}
	kept, suppressed, err := Filter(path, findings)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 0 || suppressed != 3 {
		t.Fatalf("Filter of the baselined set: kept=%d suppressed=%d; want 0, 3", len(kept), suppressed)
	}
}

func TestFilterReportsOnlyNewFindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bl.yaml")
	base := []report.Finding{f("a.cpp", "PX1001", "copy")}
	if _, err := Write(path, base); err != nil {
		t.Fatal(err)
	}
	// A later run: the baselined one plus a regression in another file.
	later := []report.Finding{
		f("a.cpp", "PX1001", "copy"),   // baselined -> suppressed
		f("c.cpp", "PX3007", "by val"), // NEW -> reported
	}
	kept, suppressed, err := Filter(path, later)
	if err != nil {
		t.Fatal(err)
	}
	if suppressed != 1 || len(kept) != 1 || kept[0].ID != "PX3007" {
		t.Fatalf("kept=%v suppressed=%d; want the single PX3007 regression kept, 1 suppressed", kept, suppressed)
	}
}

func TestFilterIsCountedPerKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bl.yaml")
	// Baseline accepts TWO identical {file,id,message} findings.
	if _, err := Write(path, []report.Finding{
		f("a.cpp", "PX2101", "grow"),
		f("a.cpp", "PX2101", "grow"),
	}); err != nil {
		t.Fatal(err)
	}
	// Later there are THREE — the third is a regression.
	kept, suppressed, err := Filter(path, []report.Finding{
		f("a.cpp", "PX2101", "grow"),
		f("a.cpp", "PX2101", "grow"),
		f("a.cpp", "PX2101", "grow"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if suppressed != 2 || len(kept) != 1 {
		t.Fatalf("counted suppression: kept=%d suppressed=%d; want 1 kept, 2 suppressed", len(kept), suppressed)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.yaml")
	if Exists(missing) {
		t.Error("Exists reported a missing file as present")
	}
	present := filepath.Join(dir, "there.yaml")
	if _, err := Write(present, nil); err != nil {
		t.Fatal(err)
	}
	if !Exists(present) {
		t.Error("Exists reported a written file as missing")
	}
}
