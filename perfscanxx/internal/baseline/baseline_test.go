package baseline

import (
	"os"
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

// TestFilterErrorAndEdgePaths pins Filter's non-happy paths: a missing baseline
// and a MALFORMED one must return an error (never panic, and never silently
// suppress a regression by treating a broken baseline as "everything allowed"),
// while a well-formed but EMPTY baseline suppresses nothing.
func TestFilterErrorAndEdgePaths(t *testing.T) {
	dir := t.TempDir()
	findings := []report.Finding{f("a.cpp", "PX1001", "copy")}

	t.Run("missing baseline is an error", func(t *testing.T) {
		if _, _, err := Filter(filepath.Join(dir, "nope.yaml"), findings); err == nil {
			t.Error("Filter of a missing baseline: want error, got nil")
		}
	})

	t.Run("malformed baseline is an error, not silent suppression", func(t *testing.T) {
		p := filepath.Join(dir, "bad.yaml")
		if err := os.WriteFile(p, []byte("entries: [this is: not, valid: yaml"), 0o644); err != nil {
			t.Fatal(err)
		}
		kept, suppressed, err := Filter(p, findings)
		if err == nil {
			t.Fatalf("Filter of malformed YAML: want error; got kept=%d suppressed=%d", len(kept), suppressed)
		}
	})

	t.Run("empty baseline suppresses nothing", func(t *testing.T) {
		p := filepath.Join(dir, "empty.yaml")
		if _, err := Write(p, nil); err != nil {
			t.Fatal(err)
		}
		kept, suppressed, err := Filter(p, findings)
		if err != nil {
			t.Fatal(err)
		}
		if len(kept) != 1 || suppressed != 0 {
			t.Fatalf("empty baseline: kept=%d suppressed=%d; want 1, 0 (nothing baselined)", len(kept), suppressed)
		}
	})
}

// TestWriteIsDeterministicallySorted pins that Write emits byte-identical,
// stably-sorted output regardless of input order (by file, then id, then
// message) — so a re-generated baseline never churns in version control and its
// diffs stay reviewable.
func TestWriteIsDeterministicallySorted(t *testing.T) {
	dir := t.TempDir()
	// Same finding set, two different input orders (including two entries that
	// differ ONLY in message, to exercise the third sort key).
	setA := []report.Finding{
		f("b.cpp", "PX2", "zed"),
		f("a.cpp", "PX1", "beta"),
		f("a.cpp", "PX1", "alpha"),
		f("a.cpp", "PX2", "gamma"),
	}
	setB := []report.Finding{
		f("a.cpp", "PX2", "gamma"),
		f("a.cpp", "PX1", "alpha"),
		f("b.cpp", "PX2", "zed"),
		f("a.cpp", "PX1", "beta"),
	}
	pA := filepath.Join(dir, "a.yaml")
	pB := filepath.Join(dir, "b.yaml")
	if _, err := Write(pA, setA); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(pB, setB); err != nil {
		t.Fatal(err)
	}
	dA, _ := os.ReadFile(pA)
	dB, _ := os.ReadFile(pB)
	if string(dA) != string(dB) {
		t.Errorf("Write is not order-independent:\n--- A ---\n%s\n--- B ---\n%s", dA, dB)
	}
	// The three sort keys must appear in ascending order in the serialized form.
	body := string(dA)
	order := []string{"alpha", "beta", "gamma", "zed"}
	last := -1
	for _, tok := range order {
		i := indexOf(body, tok)
		if i < 0 {
			t.Fatalf("message %q missing from baseline:\n%s", tok, body)
		}
		if i < last {
			t.Errorf("messages not in (file,id,message) sorted order (%q at %d before %d):\n%s", tok, i, last, body)
		}
		last = i
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
