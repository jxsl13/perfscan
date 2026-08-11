package report

import (
	"bytes"
	"os"
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
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }

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
	s := sarifBuf.String()
	for _, want := range []string{`"version": "2.1.0"`, `"ruleId": "PX1001"`, `"uri": "/src/demo.cpp"`} {
		if !strings.Contains(s, want) {
			t.Errorf("SARIF output lacks %s", want)
		}
	}
}
