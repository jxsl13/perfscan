package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// exportThreeFindings builds a synthetic --export-fixes YAML with three
// diagnostics across TWO distinct rules and TWO SARIF levels: two
// performance-for-range-copy (PX1001, L1 -> "warning") and one
// performance-no-int-to-ptr (PX3021, L3 -> "note"). This exercises ruleIndex
// deduplication (the repeated rule must collapse to one rules[] entry) and the
// level mapping across tiers.
func exportThreeFindings(src string) string {
	diag := func(name string, off int) string {
		return "" +
			"  - DiagnosticName: " + name + "\n" +
			"    DiagnosticMessage:\n" +
			"      Message: 'x'\n" +
			"      FilePath: '" + src + "'\n" +
			"      FileOffset: " + strconv.Itoa(off) + "\n" +
			"      Replacements: []\n"
	}
	return "MainSourceFile: '" + src + "'\n" +
		"Diagnostics:\n" +
		diag("performance-for-range-copy", 4) +
		diag("performance-no-int-to-ptr", 9) +
		diag("performance-for-range-copy", 14)
}

// TestSARIFOutputIsStructurallyValid drives `perfscanxx -sarif` end-to-end and
// validates the SARIF 2.1.0 contract GitHub Code Scanning consumes — beyond the
// unit test's shallow checks. In particular it pins the invariants easiest to
// break silently: every result.ruleIndex points into tool.driver.rules and
// rules[ruleIndex].id == result.ruleId, every level is a real SARIF enum value
// that matches the referenced rule's defaultConfiguration.level, a repeated rule
// collapses to ONE rules[] entry, and the L1->warning / L3->note tier mapping
// holds. A drift here (e.g. a ruleIndex off-by-one after a refactor) would make
// GitHub silently mis-attribute or reject findings.
func TestSARIFOutputIsStructurallyValid(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.cpp")
	if err := os.WriteFile(src, []byte("int f() { return 0; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer stubTidy(t, exportThreeFindings(src), nil, nil)()

	out, errOut, code := runCLI("-sarif", src)
	if code == 2 {
		t.Fatalf("-sarif exited 2 (error)\nstderr: %s", errOut)
	}

	var log struct {
		Schema string `json:"$schema"`
		Ver    string `json:"version"`
		Runs   []struct {
			Tool struct {
				Driver struct {
					Name  string `json:"name"`
					Rules []struct {
						ID                   string `json:"id"`
						HelpURI              string `json:"helpUri"`
						DefaultConfiguration *struct {
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
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &log); err != nil {
		t.Fatalf("SARIF is not valid JSON: %v\n%s", err, out)
	}

	if log.Ver != "2.1.0" {
		t.Errorf("version = %q, want 2.1.0", log.Ver)
	}
	if !strings.Contains(log.Schema, "sarif-2.1.0") {
		t.Errorf("$schema = %q, want a sarif-2.1.0 schema URI", log.Schema)
	}
	if len(log.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(log.Runs))
	}
	run := log.Runs[0]
	if run.Tool.Driver.Name != "perfscanxx" {
		t.Errorf("driver.name = %q, want perfscanxx", run.Tool.Driver.Name)
	}
	rules := run.Tool.Driver.Rules
	validLevels := map[string]bool{"warning": true, "note": true, "error": true}

	// A repeated rule must yield exactly one rules[] entry, each with a valid
	// defaultConfiguration.level.
	seen := map[string]bool{}
	for _, r := range rules {
		if seen[r.ID] {
			t.Errorf("duplicate rule id %q in tool.driver.rules", r.ID)
		}
		seen[r.ID] = true
		if r.DefaultConfiguration == nil || !validLevels[r.DefaultConfiguration.Level] {
			t.Errorf("rule %s: defaultConfiguration.level invalid (%+v)", r.ID, r.DefaultConfiguration)
		}
		// PX1001/PX3021 are built-in checks, so each rule must link to its
		// upstream clang-tidy page.
		if !strings.HasPrefix(r.HelpURI, "https://clang.llvm.org/extra/clang-tidy/checks/") {
			t.Errorf("rule %s: helpUri %q is not an upstream clang-tidy doc URL", r.ID, r.HelpURI)
		}
	}
	if len(rules) != 2 {
		t.Errorf("rules = %d, want 2 (PX1001, PX3021)", len(rules))
	}
	if len(run.Results) != 3 {
		t.Errorf("results = %d, want 3", len(run.Results))
	}

	for i, res := range run.Results {
		if res.RuleIndex < 0 || res.RuleIndex >= len(rules) {
			t.Errorf("result[%d] ruleIndex %d out of range [0,%d)", i, res.RuleIndex, len(rules))
			continue
		}
		if rules[res.RuleIndex].ID != res.RuleID {
			t.Errorf("result[%d] ruleIndex %d -> rule %q, but ruleId is %q", i, res.RuleIndex, rules[res.RuleIndex].ID, res.RuleID)
		}
		if !validLevels[res.Level] {
			t.Errorf("result[%d] level %q is not a SARIF enum value", i, res.Level)
		}
		if dc := rules[res.RuleIndex].DefaultConfiguration; dc != nil && dc.Level != res.Level {
			t.Errorf("result[%d] level %q != referenced rule defaultConfiguration.level %q", i, res.Level, dc.Level)
		}
		if len(res.Locations) == 0 || res.Locations[0].PhysicalLocation.ArtifactLocation.URI == "" {
			t.Errorf("result[%d] missing a location uri", i)
		}
	}

	// The tier -> SARIF-severity mapping: PX1001 (L1) -> warning, PX3021 (L3) -> note.
	levelOf := map[string]string{}
	for _, res := range run.Results {
		levelOf[res.RuleID] = res.Level
	}
	if levelOf["PX1001"] != "warning" {
		t.Errorf("PX1001 (L1) SARIF level = %q, want warning", levelOf["PX1001"])
	}
	if levelOf["PX3021"] != "note" {
		t.Errorf("PX3021 (L3) SARIF level = %q, want note", levelOf["PX3021"])
	}
}
