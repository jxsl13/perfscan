// Package fixes models clang-tidy's --export-fixes YAML file.
//
// The schema is defined by the LLVM serialization traits in
// clang/include/clang/Tooling/DiagnosticsYaml.h and ReplacementsYaml.h
// (verified against llvm-project main):
//
//	MainSourceFile: '/abs/path/main.cpp'
//	Diagnostics:
//	  - DiagnosticName:  performance-unnecessary-value-param
//	    DiagnosticMessage:
//	      Message:    'the parameter ''s'' is copied ...'
//	      FilePath:   '/abs/path/main.cpp'
//	      FileOffset: 123
//	      Replacements:
//	        - FilePath:        '/abs/path/main.cpp'
//	          Offset:          100
//	          Length:          11
//	          ReplacementText: 'const std::string&'
//	    Level:          Warning
//	    BuildDirectory: '/abs/build'
//
// Since LLVM 9 the message payload is nested under DiagnosticMessage;
// perfscanxx targets that modern schema only (brew's llvm is far newer).
package fixes

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// ExportFile is one --export-fixes document
// (clang::tooling::TranslationUnitDiagnostics).
type ExportFile struct {
	MainSourceFile string       `yaml:"MainSourceFile"`
	Diagnostics    []Diagnostic `yaml:"Diagnostics"`
}

// Diagnostic is a single clang-tidy diagnostic
// (clang::tooling::Diagnostic).
type Diagnostic struct {
	// DiagnosticName is the clang-tidy check name,
	// e.g. "performance-for-range-copy" or "clang-diagnostic-error".
	DiagnosticName    string              `yaml:"DiagnosticName"`
	DiagnosticMessage DiagnosticMessage   `yaml:"DiagnosticMessage"`
	Notes             []DiagnosticMessage `yaml:"Notes,omitempty"`
	// Level is "Warning", "Error" or "Remark".
	Level          string `yaml:"Level,omitempty"`
	BuildDirectory string `yaml:"BuildDirectory,omitempty"`
}

// DiagnosticMessage carries the message text, its anchor position and the
// fix-its (clang::tooling::DiagnosticMessage).
type DiagnosticMessage struct {
	Message  string `yaml:"Message"`
	FilePath string `yaml:"FilePath,omitempty"`
	// FileOffset is a BYTE offset into FilePath, not a line number.
	FileOffset   int             `yaml:"FileOffset,omitempty"`
	Replacements []Replacement   `yaml:"Replacements"`
	Ranges       []FileByteRange `yaml:"Ranges,omitempty"`
}

// Replacement is a single fix-it edit (clang::tooling::Replacement):
// replace Length bytes at byte Offset in FilePath with ReplacementText.
type Replacement struct {
	FilePath        string `yaml:"FilePath"`
	Offset          int    `yaml:"Offset"`
	Length          int    `yaml:"Length"`
	ReplacementText string `yaml:"ReplacementText"`
}

// FileByteRange is a highlighted source range
// (clang::tooling::FileByteRange).
type FileByteRange struct {
	FilePath   string `yaml:"FilePath"`
	FileOffset int    `yaml:"FileOffset"`
	Length     int    `yaml:"Length"`
}

// blockScalarIndicator matches a YAML block-scalar header that carries an
// EXPLICIT indentation indicator digit — `ReplacementText: |4-` and friends.
// LLVM's YAML emitter writes multi-line ReplacementText (e.g. the member-
// initializer insertion of cppcoreguidelines-prefer-member-initializer) as a
// literal block scalar with an explicit indentation indicator, and gopkg.in/
// yaml.v3 mis-parses that form ("did not find expected key"), failing the whole
// export — and, in a parallel run, the whole scan. The indicator is stripped so
// go-yaml auto-detects the block indentation: `|4-` -> `|-`, `|4` -> `|`,
// `>4-` -> `>-`. This only ever touches MULTI-LINE replacement text (single-line
// replacements are plain/quoted scalars), which perfscanxx never consumes —
// singleReplacement rejects multi-line fixes and -fix applies via clang-tidy
// itself — so recovering the diagnostic (its name, message and location) is what
// matters, and the exact leading whitespace of that unused text is not.
var blockScalarIndicator = regexp.MustCompile(`(?m)([:\-]\s+[|>])[0-9]+([+-]?)[ \t]*$`)

// Parse decodes one --export-fixes YAML document.
// An empty input yields an empty ExportFile (clang-tidy writes nothing or an
// empty file when there are no diagnostics).
func Parse(data []byte) (*ExportFile, error) {
	var f ExportFile
	if len(data) == 0 {
		return &f, nil
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		// Retry once with LLVM's explicit block-scalar indentation indicators
		// normalized away — the one construct go-yaml chokes on in a real
		// clang-tidy export. If it still fails, report the original error.
		normalized := blockScalarIndicator.ReplaceAll(data, []byte("$1$2"))
		if len(normalized) != len(data) {
			var f2 ExportFile
			if err2 := yaml.Unmarshal(normalized, &f2); err2 == nil {
				return &f2, nil
			}
		}
		return nil, fmt.Errorf("fixes: invalid --export-fixes YAML: %w", err)
	}
	return &f, nil
}

// Marshal serializes an ExportFile back to the --export-fixes YAML form (the
// inverse of Parse), used to merge several parallel-worker exports into one
// payload the rest of the pipeline consumes exactly as a single clang-tidy run's.
func Marshal(f *ExportFile) ([]byte, error) {
	data, err := yaml.Marshal(f)
	if err != nil {
		return nil, fmt.Errorf("fixes: marshaling merged export: %w", err)
	}
	return data, nil
}
