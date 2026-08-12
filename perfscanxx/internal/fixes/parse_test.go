package fixes

import "testing"

// fixture mirrors a real clang-tidy --export-fixes file (schema per
// clang/include/clang/Tooling/DiagnosticsYaml.h, LLVM >= 9).
const fixture = `---
MainSourceFile:  '/src/demo.cpp'
Diagnostics:
  - DiagnosticName:  performance-unnecessary-value-param
    DiagnosticMessage:
      Message:         'the parameter ''name'' is copied for each invocation but only used as a const reference; consider making it a const reference'
      FilePath:        '/src/demo.cpp'
      FileOffset:      58
      Replacements:
        - FilePath:        '/src/demo.cpp'
          Offset:          46
          Length:          11
          ReplacementText: 'const std::string&'
        - FilePath:        '/src/demo.cpp'
          Offset:          70
          Length:          0
          ReplacementText: ' '
    Level:           Warning
    BuildDirectory:  '/src/build'
  - DiagnosticName:  performance-avoid-endl
    DiagnosticMessage:
      Message:         'do not use ''std::endl'' with streams; use ''\n'' instead'
      FilePath:        '/src/demo.cpp'
      FileOffset:      120
      Replacements:    []
    Level:           Warning
    BuildDirectory:  '/src/build'
...
`

func TestParse(t *testing.T) {
	ef, err := Parse([]byte(fixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := ef.MainSourceFile, "/src/demo.cpp"; got != want {
		t.Errorf("MainSourceFile = %q, want %q", got, want)
	}
	if got, want := len(ef.Diagnostics), 2; got != want {
		t.Fatalf("len(Diagnostics) = %d, want %d", got, want)
	}

	d := ef.Diagnostics[0]
	if got, want := d.DiagnosticName, "performance-unnecessary-value-param"; got != want {
		t.Errorf("DiagnosticName = %q, want %q", got, want)
	}
	if got, want := d.Level, "Warning"; got != want {
		t.Errorf("Level = %q, want %q", got, want)
	}
	if got, want := d.BuildDirectory, "/src/build"; got != want {
		t.Errorf("BuildDirectory = %q, want %q", got, want)
	}
	m := d.DiagnosticMessage
	if m.Message == "" || m.FilePath != "/src/demo.cpp" || m.FileOffset != 58 {
		t.Errorf("DiagnosticMessage = %+v: want message text, FilePath=/src/demo.cpp, FileOffset=58", m)
	}
	if got, want := len(m.Replacements), 2; got != want {
		t.Fatalf("len(Replacements) = %d, want %d", got, want)
	}
	r := m.Replacements[0]
	want := Replacement{
		FilePath:        "/src/demo.cpp",
		Offset:          46,
		Length:          11,
		ReplacementText: "const std::string&",
	}
	if r != want {
		t.Errorf("Replacements[0] = %+v, want %+v", r, want)
	}
	if got := m.Replacements[1]; got.Length != 0 || got.ReplacementText != " " {
		t.Errorf("Replacements[1] = %+v: want zero-length insertion of %q", got, " ")
	}

	d2 := ef.Diagnostics[1]
	if got, want := d2.DiagnosticName, "performance-avoid-endl"; got != want {
		t.Errorf("Diagnostics[1].DiagnosticName = %q, want %q", got, want)
	}
	if got := len(d2.DiagnosticMessage.Replacements); got != 0 {
		t.Errorf("Diagnostics[1] Replacements = %d, want 0", got)
	}
}

func TestParseEmpty(t *testing.T) {
	ef, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil): %v", err)
	}
	if len(ef.Diagnostics) != 0 {
		t.Errorf("Parse(nil) diagnostics = %d, want 0", len(ef.Diagnostics))
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := Parse([]byte("{unbalanced")); err == nil {
		t.Fatal("Parse of invalid YAML: want error, got nil")
	}
}

// realClangTidyExport is a verbatim --export-fixes document captured from
// clang-tidy/LLVM 22 (performance-unnecessary-value-param, which emits a
// TWO-replacement fix-it, plus performance-faster-string-find). Unlike the
// hand-written fixture above it validates the struct's YAML tags against GROUND
// TRUTH — an upstream key rename or structure change would silently zero a field
// (e.g. Offset) and this test would catch it.
const realClangTidyExport = `---
MainSourceFile:  '/tmp/ctexport/t.cpp'
Diagnostics:
  - DiagnosticName:  performance-unnecessary-value-param
    DiagnosticMessage:
      Message:         'the parameter ''x'' of type ''Big'' is copied'
      FilePath:        '/tmp/ctexport/t.cpp'
      FileOffset:      66
      Replacements:
        - FilePath:        '/tmp/ctexport/t.cpp'
          Offset:          62
          Length:          0
          ReplacementText: 'const '
        - FilePath:        '/tmp/ctexport/t.cpp'
          Offset:          65
          Length:          0
          ReplacementText: '&'
    Level:           Warning
    BuildDirectory:  '/tmp/ctexport'
  - DiagnosticName:  performance-faster-string-find
    DiagnosticMessage:
      Message:         '''find'' called with a single-character string literal'
      FilePath:        '/tmp/ctexport/t.cpp'
      FileOffset:      174
      Replacements:
        - FilePath:        '/tmp/ctexport/t.cpp'
          Offset:          174
          Length:          3
          ReplacementText: '''z'''
    Level:           Warning
    BuildDirectory:  '/tmp/ctexport'
...
`

// TestParseRealClangTidyExport pins the parser against real clang-tidy 22 output.
func TestParseRealClangTidyExport(t *testing.T) {
	ef, err := Parse([]byte(realClangTidyExport))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if ef.MainSourceFile != "/tmp/ctexport/t.cpp" {
		t.Errorf("MainSourceFile = %q", ef.MainSourceFile)
	}
	if len(ef.Diagnostics) != 2 {
		t.Fatalf("Diagnostics = %d, want 2", len(ef.Diagnostics))
	}
	// A single diagnostic with a TWO-replacement fix-it (const + &) — the count
	// of Replacements is what report.go surfaces as "fix available".
	d0 := ef.Diagnostics[0]
	if d0.DiagnosticName != "performance-unnecessary-value-param" || d0.Level != "Warning" ||
		d0.BuildDirectory != "/tmp/ctexport" || d0.DiagnosticMessage.FileOffset != 66 {
		t.Errorf("Diagnostics[0] = %+v", d0)
	}
	want0 := []Replacement{
		{FilePath: "/tmp/ctexport/t.cpp", Offset: 62, Length: 0, ReplacementText: "const "},
		{FilePath: "/tmp/ctexport/t.cpp", Offset: 65, Length: 0, ReplacementText: "&"},
	}
	if got := d0.DiagnosticMessage.Replacements; len(got) != 2 || got[0] != want0[0] || got[1] != want0[1] {
		t.Errorf("Diagnostics[0].Replacements = %+v, want %+v", got, want0)
	}
	// Second: a single replacement of a char literal.
	d1 := ef.Diagnostics[1]
	want1 := Replacement{FilePath: "/tmp/ctexport/t.cpp", Offset: 174, Length: 3, ReplacementText: "'z'"}
	if d1.DiagnosticName != "performance-faster-string-find" || d1.DiagnosticMessage.FileOffset != 174 ||
		len(d1.DiagnosticMessage.Replacements) != 1 || d1.DiagnosticMessage.Replacements[0] != want1 {
		t.Errorf("Diagnostics[1] = %+v", d1)
	}
}
