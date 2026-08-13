package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exportFindingPlusParseError builds a --export-fixes YAML with one real catalog
// finding (performance-for-range-copy = PX1001) AND one pass-through
// clang-diagnostic-error (a compile failure clang-tidy reports but that is not a
// performance check).
func exportFindingPlusParseError(src string) string {
	return "MainSourceFile: '" + src + "'\n" +
		"Diagnostics:\n" +
		"  - DiagnosticName: performance-for-range-copy\n" +
		"    DiagnosticMessage:\n" +
		"      Message: 'loop variable is copied'\n" +
		"      FilePath: '" + src + "'\n" +
		"      FileOffset: 5\n" +
		"      Replacements: []\n" +
		"  - DiagnosticName: clang-diagnostic-error\n" +
		"    DiagnosticMessage:\n" +
		"      Message: \"use of undeclared identifier 'items'\"\n" +
		"      FilePath: '" + src + "'\n" +
		"      FileOffset: 13\n" +
		"      Replacements: []\n"
}

// TestPassThroughDiagnosticsExcludedFromMachineOutput pins the contract that
// clang-diagnostic-* pass-through diagnostics (compile errors/warnings, not
// catalog performance checks) are surfaced ONLY as the human-facing
// "did not fully parse" summary on stderr — never mixed into the -json / -sarif
// findings on stdout, which machines consume (a leaked compile error would
// pollute GitHub Code Scanning with non-performance noise). It also pins that a
// pass-through diagnostic does not, by itself, flip the exit code.
func TestPassThroughDiagnosticsExcludedFromMachineOutput(t *testing.T) {
	newSrc := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		src := filepath.Join(dir, "main.cpp")
		if err := os.WriteFile(src, []byte("for (auto x : items) {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return src
	}

	t.Run("-json excludes clang-diagnostic-*", func(t *testing.T) {
		src := newSrc(t)
		defer stubTidy(t, exportFindingPlusParseError(src), nil, nil)()
		out, errOut, _ := runCLI("-json", src)

		if strings.Contains(out, "clang-diagnostic") {
			t.Errorf("-json stdout leaked a pass-through diagnostic:\n%s", out)
		}
		var findings []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(out), &findings); err != nil {
			t.Fatalf("-json is not a valid array: %v\n%s", err, out)
		}
		if len(findings) != 1 || findings[0].ID != "PX1001" {
			t.Errorf("-json should carry exactly the PX1001 finding, got %+v", findings)
		}
		if !strings.Contains(errOut, "did not fully parse") {
			t.Errorf("the parse error should be summarized on stderr, got:\n%s", errOut)
		}
	})

	t.Run("-sarif excludes clang-diagnostic-*", func(t *testing.T) {
		src := newSrc(t)
		defer stubTidy(t, exportFindingPlusParseError(src), nil, nil)()
		out, _, _ := runCLI("-sarif", src)

		if strings.Contains(out, "clang-diagnostic") {
			t.Errorf("-sarif stdout leaked a pass-through diagnostic:\n%s", out)
		}
		var log struct {
			Runs []struct {
				Tool struct {
					Driver struct {
						Rules []struct {
							ID string `json:"id"`
						} `json:"rules"`
					} `json:"driver"`
				} `json:"tool"`
				Results []struct {
					RuleID string `json:"ruleId"`
				} `json:"results"`
			} `json:"runs"`
		}
		if err := json.Unmarshal([]byte(out), &log); err != nil {
			t.Fatalf("-sarif is not valid JSON: %v\n%s", err, out)
		}
		if len(log.Runs) != 1 {
			t.Fatalf("runs = %d, want 1", len(log.Runs))
		}
		run := log.Runs[0]
		if len(run.Tool.Driver.Rules) != 1 || run.Tool.Driver.Rules[0].ID != "PX1001" {
			t.Errorf("SARIF rules should be exactly {PX1001}, got %+v", run.Tool.Driver.Rules)
		}
		if len(run.Results) != 1 || run.Results[0].RuleID != "PX1001" {
			t.Errorf("SARIF results should be exactly the PX1001 finding, got %+v", run.Results)
		}
	})

	t.Run("a parse error alone does not flip the exit code", func(t *testing.T) {
		src := newSrc(t)
		// Only a clang-diagnostic-error, no real finding.
		export := "MainSourceFile: '" + src + "'\n" +
			"Diagnostics:\n" +
			"  - DiagnosticName: clang-diagnostic-error\n" +
			"    DiagnosticMessage:\n" +
			"      Message: 'boom'\n" +
			"      FilePath: '" + src + "'\n" +
			"      FileOffset: 0\n" +
			"      Replacements: []\n"
		defer stubTidy(t, export, nil, nil)()
		out, errOut, code := runCLI(src)
		if code != 0 {
			t.Errorf("a pass-through diagnostic alone must not set exit 1 (no real findings): code=%d", code)
		}
		if !strings.Contains(errOut, "did not fully parse") {
			t.Errorf("expected the parse-error summary on stderr, got:\n%s", errOut)
		}
		if strings.Contains(out, "clang-diagnostic") {
			t.Errorf("stdout must stay clean of pass-through diagnostics, got:\n%s", out)
		}
	})
}
