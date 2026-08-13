package report

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime"
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

// TestTextOffsetFallbackAndNoFix pins the two human-facing Text branches that
// TestLineColAndText does not: when the source file can't be read, lineCol
// yields (0,0) and Text falls back to `file:#<offset>` instead of `file:line:col`;
// and a finding with no fix-it omits the ", fix available" suffix.
func TestTextOffsetFallbackAndNoFix(t *testing.T) {
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist } // unreadable

	findings := FromExport(&fixes.ExportFile{Diagnostics: []fixes.Diagnostic{{
		DiagnosticName: "performance-for-range-copy", // PX1001, level L1
		DiagnosticMessage: fixes.DiagnosticMessage{
			Message: "msg", FilePath: "/src/x.cpp", FileOffset: 42,
			// no Replacements -> Fixes == 0 -> no ", fix available"
		},
	}}}, catalog.LevelAggressive)

	if findings[0].Line != 0 {
		t.Fatalf("unreadable file should yield Line 0, got %d", findings[0].Line)
	}
	var buf bytes.Buffer
	Text(&buf, findings)
	want := "/src/x.cpp:#42: msg (PX1001 L1)\n"
	if got := buf.String(); got != want {
		t.Errorf("Text (offset fallback, no fix) = %q, want %q", got, want)
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
	// Validate the -json output STRUCTURALLY (tooling/editors consume it): parse
	// the array and assert each finding's fields — id, level, category, message,
	// file, a 1-based line, and the fix count.
	var jf []struct {
		ID       string `json:"id"`
		Check    string `json:"check"`
		Level    string `json:"level"`
		Category string `json:"category"`
		Message  string `json:"message"`
		File     string `json:"file"`
		Line     int    `json:"line"`
		Fixes    int    `json:"fixes"`
	}
	if err := json.Unmarshal(jsonBuf.Bytes(), &jf); err != nil {
		t.Fatalf("-json is not valid JSON: %v\n%s", err, jsonBuf.String())
	}
	if len(jf) != 2 {
		t.Fatalf("-json has %d findings, want 2", len(jf))
	}
	if jf[0].ID != "PX1001" || jf[0].Level != "L1" || jf[0].Category == "" ||
		jf[0].Check != "performance-for-range-copy" || jf[0].File != "/src/demo.cpp" ||
		jf[0].Line < 1 || jf[0].Message == "" || jf[0].Fixes != 1 {
		t.Errorf("-json finding[0] = %+v, want PX1001/L1/1-fix with a resolved location", jf[0])
	}
	if jf[1].ID != "PX2001" || jf[1].Level != "L2" || jf[1].Fixes != 0 {
		t.Errorf("-json finding[1] = %+v, want PX2001/L2/0-fix", jf[1])
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
						ID               string `json:"id"`
						ShortDescription struct {
							Text string `json:"text"`
						} `json:"shortDescription"`
						DefaultConfiguration struct {
							Level string `json:"level"`
						} `json:"defaultConfiguration"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID    string `json:"ruleId"`
				RuleIndex int    `json:"ruleIndex"`
				Level     string `json:"level"`
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
		// Curated PX rules carry the catalog's one-line summary so GitHub
		// Code Scanning shows what the rule means. PX1001 is catalogued.
		if r.ID == "PX1001" {
			if e, ok := catalog.ByID("PX1001"); ok && r.ShortDescription.Text != e.Title {
				t.Errorf("rule PX1001 shortDescription = %q, want catalog Title %q", r.ShortDescription.Text, e.Title)
			}
		}
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
		// ruleIndex must point at the declared rule with the same id
		// (GitHub uses it to resolve the rule without a name lookup).
		if res.RuleIndex < 0 || res.RuleIndex >= len(run.Tool.Driver.Rules) {
			t.Errorf("result[%d] (%s) ruleIndex %d out of range [0,%d)", i, res.RuleID, res.RuleIndex, len(run.Tool.Driver.Rules))
		} else if got := run.Tool.Driver.Rules[res.RuleIndex].ID; got != res.RuleID {
			t.Errorf("result[%d] ruleIndex %d points at rule %q, want %q", i, res.RuleIndex, got, res.RuleID)
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

// TestSARIFLevelMapping pins the catalog-level -> SARIF triage-level mapping:
// L1/L2 (actionable) -> "warning", L3 (aggressive/niche) -> "note", on both the
// rule's defaultConfiguration and each result. GitHub Code Scanning reads these.
func TestSARIFLevelMapping(t *testing.T) {
	findings := []Finding{
		{ID: "PX1001", TidyName: "performance-for-range-copy", Level: "L1", Message: "m", File: "a.cpp", Line: 1},
		{ID: "PX2001", TidyName: "x", Level: "L2", Message: "m", File: "b.cpp", Line: 1},
		{ID: "PX3022", TidyName: "performance-enum-size", Level: "L3", Message: "m", File: "c.cpp", Line: 1},
	}
	var buf bytes.Buffer
	if err := SARIF(&buf, findings); err != nil {
		t.Fatal(err)
	}
	var log struct {
		Runs []struct {
			Tool struct {
				Driver struct {
					Rules []struct {
						ID                   string `json:"id"`
						DefaultConfiguration struct {
							Level string `json:"level"`
						} `json:"defaultConfiguration"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID string `json:"ruleId"`
				Level  string `json:"level"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("invalid SARIF: %v", err)
	}
	want := map[string]string{"PX1001": "warning", "PX2001": "warning", "PX3022": "note"}
	run := log.Runs[0]
	for _, r := range run.Tool.Driver.Rules {
		if got := r.DefaultConfiguration.Level; got != want[r.ID] {
			t.Errorf("rule %s defaultConfiguration.level = %q, want %q", r.ID, got, want[r.ID])
		}
	}
	for _, res := range run.Results {
		if got := res.Level; got != want[res.RuleID] {
			t.Errorf("result %s level = %q, want %q", res.RuleID, got, want[res.RuleID])
		}
	}
}
