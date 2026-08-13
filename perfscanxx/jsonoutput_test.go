package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestJSONFindingsOutput drives `perfscanxx -json` end-to-end and validates the
// findings JSON contract machine consumers depend on. report.JSON is unit-tested
// directly (TestJSONAndSARIF) and `-list -json` (the catalog) is covered, but the
// FINDINGS -json path through the CLI was not — this pins that -json emits a
// valid JSON array of findings carrying the documented fields (id, check, level,
// message, file, fixes count) with the catalog-resolved values.
func TestJSONFindingsOutput(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.cpp")
	if err := os.WriteFile(src, []byte("for (auto x : items) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer stubTidy(t, exportOneFinding(src), nil, nil)()

	out, errOut, code := runCLI("-json", src)
	if code == 2 {
		t.Fatalf("-json exited 2 (error)\nstderr: %s", errOut)
	}

	var findings []struct {
		ID      string `json:"id"`
		Check   string `json:"check"`
		Level   string `json:"level"`
		Message string `json:"message"`
		File    string `json:"file"`
		Fixes   int    `json:"fixes"`
	}
	if err := json.Unmarshal([]byte(out), &findings); err != nil {
		t.Fatalf("-json output is not a valid JSON array of findings: %v\n%s", err, out)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1\n%s", len(findings), out)
	}
	f := findings[0]
	if f.ID != "PX1001" {
		t.Errorf("id = %q, want PX1001 (catalog-resolved from the TidyName)", f.ID)
	}
	if f.Check != "performance-for-range-copy" {
		t.Errorf("check = %q, want performance-for-range-copy", f.Check)
	}
	if f.Level != "L1" {
		t.Errorf("level = %q, want L1", f.Level)
	}
	if f.Message == "" || f.File == "" {
		t.Errorf("message and file must be populated: %+v", f)
	}
	if f.Fixes != 1 {
		t.Errorf("fixes = %d, want 1 (the single replacement in the export)", f.Fixes)
	}
}
