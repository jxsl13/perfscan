package report

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
	"github.com/jxsl13/perfscan/perfscanxx/internal/fixes"
)

func sampleExport() *fixes.ExportFile {
	return &fixes.ExportFile{
		MainSourceFile: "/src/demo.cpp",
		Diagnostics: []fixes.Diagnostic{
			{
				DiagnosticName: "performance-for-range-copy", // PX1001, L1
				DiagnosticMessage: fixes.DiagnosticMessage{
					Message:    "loop variable is copied but only used as const reference",
					FilePath:   "/src/demo.cpp",
					FileOffset: 10,
					Replacements: []fixes.Replacement{
						{FilePath: "/src/demo.cpp", Offset: 10, Length: 4, ReplacementText: "const auto&"},
					},
				},
			},
			{
				DiagnosticName: "performance-inefficient-vector-operation", // PX2001, L2
				DiagnosticMessage: fixes.DiagnosticMessage{
					Message:    "'push_back' is called inside a loop; consider pre-allocating",
					FilePath:   "/src/demo.cpp",
					FileOffset: 40,
				},
			},
		},
	}
}

func TestFromExportLevelGating(t *testing.T) {
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }

	all := FromExport(sampleExport(), catalog.LevelAggressive)
	if len(all) != 2 {
		t.Fatalf("L3 findings = %d, want 2", len(all))
	}
	if all[0].ID != "PX1001" || all[0].Level != "L1" || all[0].Fixes != 1 {
		t.Errorf("finding[0] = %+v, want PX1001/L1 with 1 fix", all[0])
	}

	l1 := FromExport(sampleExport(), catalog.LevelIdiomatic)
	if len(l1) != 1 || l1[0].ID != "PX1001" {
		t.Fatalf("L1 findings = %+v, want only PX1001", l1)
	}
}

func TestFromExportPassThrough(t *testing.T) {
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }

	ef := &fixes.ExportFile{Diagnostics: []fixes.Diagnostic{{
		DiagnosticName: "clang-diagnostic-error",
		DiagnosticMessage: fixes.DiagnosticMessage{
			Message: "unknown type name 'strng'", FilePath: "/src/x.cpp", FileOffset: 3,
		},
	}}}
	got := FromExport(ef, catalog.LevelIdiomatic)
	if len(got) != 1 || got[0].ID != "clang-diagnostic-error" {
		t.Fatalf("pass-through = %+v, want ungated clang-diagnostic-error", got)
	}
}

func TestLineColAndText(t *testing.T) {
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	// offset 10 lands on line 2: "0123456\n" is 8 bytes, so 10 -> col 3.
	ReadFile = func(string) ([]byte, error) { return []byte("0123456\nabcdef\n"), nil }

	findings := FromExport(&fixes.ExportFile{Diagnostics: []fixes.Diagnostic{{
		DiagnosticName: "performance-for-range-copy",
		DiagnosticMessage: fixes.DiagnosticMessage{
			Message: "msg", FilePath: "/src/demo.cpp", FileOffset: 10,
			Replacements: []fixes.Replacement{{FilePath: "/src/demo.cpp"}},
		},
	}}}, catalog.LevelAggressive)

	if findings[0].Line != 2 || findings[0].Col != 3 {
		t.Fatalf("line:col = %d:%d, want 2:3", findings[0].Line, findings[0].Col)
	}

	var buf bytes.Buffer
	Text(&buf, findings)
	out := buf.String()
	want := "/src/demo.cpp:2:3: msg (PX1001 L1, fix available)\n"
	if out != want {
		t.Errorf("Text = %q, want %q", out, want)
	}
}

func TestJSONAndSARIF(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SARIF/JSON fixtures use POSIX absolute paths; path handling is filepath-based, covered on unix")
	}
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	// Return real content so the sample offsets (10, 40) resolve to 1-based
	// line numbers — exercising the real SARIF location path (offset 10 -> line 1,
	// offset 40 -> line 3).
	ReadFile = func(string) ([]byte, error) {
		return []byte("aaaaaaaaaa\nbbbbbbbbbbbbbbbbbbbb\ncccccccccccccccccccc\n"), nil
	}

	findings := FromExport(sampleExport(), catalog.LevelAggressive)

	var jsonBuf bytes.Buffer
	if err := JSON(&jsonBuf, findings); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if !strings.Contains(jsonBuf.String(), `"id": "PX1001"`) {
		t.Errorf("JSON output lacks PX1001: %s", jsonBuf.String())
	}

	var sarifBuf bytes.Buffer
	if err := SARIF(&sarifBuf, findings); err != nil {
		t.Fatalf("SARIF: %v", err)
	}

	// Validate the SARIF STRUCTURALLY (not just by substring): GitHub Code
	// Scanning silently rejects malformed SARIF, so assert the required 2.1.0
	// shape — version, one run with a named tool driver, and every result
	// carrying a ruleId that resolves to a declared rule plus a physical
	// location with a uri and 1-based startLine.
	var log struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name  string `json:"name"`
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID    string `json:"ruleId"`
				Locations []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region struct {
							StartLine int `json:"startLine"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(sarifBuf.Bytes(), &log); err != nil {
		t.Fatalf("SARIF is not valid JSON: %v\n%s", err, sarifBuf.String())
	}
	if log.Version != "2.1.0" {
		t.Errorf("SARIF version = %q, want 2.1.0", log.Version)
	}
	if len(log.Runs) != 1 {
		t.Fatalf("SARIF runs = %d, want 1", len(log.Runs))
	}
	run := log.Runs[0]
	if run.Tool.Driver.Name == "" {
		t.Error("SARIF tool.driver.name is empty")
	}
	ruleIDs := map[string]bool{}
	for _, r := range run.Tool.Driver.Rules {
		ruleIDs[r.ID] = true
	}
	if len(run.Results) == 0 {
		t.Fatal("SARIF has no results")
	}
	for i, res := range run.Results {
		if res.RuleID == "" {
			t.Errorf("result[%d] has empty ruleId", i)
		} else if !ruleIDs[res.RuleID] {
			t.Errorf("result[%d] ruleId %q is not declared in tool.driver.rules", i, res.RuleID)
		}
		if len(res.Locations) == 0 {
			t.Errorf("result[%d] (%s) has no locations", i, res.RuleID)
			continue
		}
		loc := res.Locations[0].PhysicalLocation
		if loc.ArtifactLocation.URI == "" {
			t.Errorf("result[%d] (%s) location has no uri", i, res.RuleID)
		}
		if loc.Region.StartLine < 1 {
			t.Errorf("result[%d] (%s) startLine = %d, want >= 1", i, res.RuleID, loc.Region.StartLine)
		}
	}
}
