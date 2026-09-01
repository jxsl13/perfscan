package runner

import (
	"bytes"
	"encoding/json"
	"go/token"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/lint"
)

// TestSARIFStructure validates emitSARIF produces well-formed SARIF 2.1.0 for
// GitHub Code Scanning (which silently rejects malformed SARIF): the required
// version/schema, one run with a named tool driver, rules deduped per check ID,
// and results whose ruleId resolves to a declared rule with a physical location
// (uri + 1-based startLine). It also pins the fix-level → severity mapping
// (L1/L2 warning, L3 note).
func TestSARIFStructure(t *testing.T) {
	t.Parallel()

	c1 := &lint.Check{ID: "PS9001", Category: "alloc", Level: lint.LevelIdiomatic, Doc: lint.Documentation{Title: "t1"}}
	c3 := &lint.Check{ID: "PS9003", Category: "misc", Level: lint.LevelAggressive, Doc: lint.Documentation{Title: "t3"}}
	findings := []Finding{
		{Check: c1, Pos: token.Position{Filename: "a.go", Line: 12, Column: 3}, End: token.Position{Filename: "a.go", Line: 12, Column: 20}, Message: "m1"},
		{Check: c1, Pos: token.Position{Filename: "b.go", Line: 5, Column: 1}, End: token.Position{Filename: "b.go", Line: 5, Column: 6}, Message: "m1b"}, // same rule -> deduped
		{Check: c3, Pos: token.Position{Filename: "a.go", Line: 40, Column: 2}, End: token.Position{Filename: "a.go", Line: 40, Column: 9}, Message: "m3"},
	}

	var buf bytes.Buffer
	metadata := fakeEvidenceMetadata("go1.25.3", "1.25.0")
	emitSARIF(&buf, findings, metadata)

	var log struct {
		Version string `json:"version"`
		Schema  string `json:"$schema"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name           string `json:"name"`
					InformationURI string `json:"informationUri"`
					Rules          []struct {
						ID                   string `json:"id"`
						HelpURI              string `json:"helpUri"`
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
			Properties evidenceMetadata `json:"properties"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("SARIF is not valid JSON: %v\n%s", err, buf.String())
	}
	if log.Version != "2.1.0" {
		t.Errorf("version = %q, want 2.1.0", log.Version)
	}
	if log.Schema == "" {
		t.Error("missing $schema")
	}
	if len(log.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(log.Runs))
	}
	run := log.Runs[0]
	if run.Properties.Toolchain != metadata.Toolchain {
		t.Errorf("run.properties.toolchain = %+v, want %+v", run.Properties.Toolchain, metadata.Toolchain)
	}
	if run.Tool.Driver.Name != "perfscan" {
		t.Errorf("tool.driver.name = %q, want perfscan", run.Tool.Driver.Name)
	}
	// The driver advertises its homepage (informationUri) for GitHub Code Scanning.
	if !strings.HasPrefix(run.Tool.Driver.InformationURI, "https://") {
		t.Errorf("tool.driver.informationUri = %q, want an https URL", run.Tool.Driver.InformationURI)
	}
	// Rules deduped per check ID: PS9001 (fired twice) + PS9003 -> exactly 2.
	ruleIDs := map[string]bool{}
	for _, r := range run.Tool.Driver.Rules {
		ruleIDs[r.ID] = true
		// Every rule links to its check's doc page (helpUri).
		if !strings.HasPrefix(r.HelpURI, "https://") {
			t.Errorf("rule %s: helpUri = %q, want an https URL", r.ID, r.HelpURI)
		}
	}
	if len(run.Tool.Driver.Rules) != 2 {
		t.Errorf("rules = %d, want 2 (deduped)", len(run.Tool.Driver.Rules))
	}
	if !ruleIDs["PS9001"] || !ruleIDs["PS9003"] {
		t.Errorf("rules missing PS9001/PS9003: %v", ruleIDs)
	}
	if len(run.Results) != 3 {
		t.Fatalf("results = %d, want 3", len(run.Results))
	}
	wantLevel := map[string]string{"PS9001": "warning", "PS9003": "note"} // L1 -> warning, L3 -> note
	rules := run.Tool.Driver.Rules
	for i, res := range run.Results {
		if !ruleIDs[res.RuleID] {
			t.Errorf("result[%d] ruleId %q is not declared in tool.driver.rules", i, res.RuleID)
		}
		// ruleIndex must point into rules[] and resolve to the same id as ruleId.
		if res.RuleIndex < 0 || res.RuleIndex >= len(rules) {
			t.Errorf("result[%d] ruleIndex %d out of range [0,%d)", i, res.RuleIndex, len(rules))
		} else {
			if rules[res.RuleIndex].ID != res.RuleID {
				t.Errorf("result[%d] ruleIndex %d -> rule %q, but ruleId is %q", i, res.RuleIndex, rules[res.RuleIndex].ID, res.RuleID)
			}
			// The rule's defaultConfiguration.level must match the result level.
			if dl := rules[res.RuleIndex].DefaultConfiguration.Level; dl != res.Level {
				t.Errorf("result[%d] level %q != rule defaultConfiguration.level %q", i, res.Level, dl)
			}
		}
		if res.Level != wantLevel[res.RuleID] {
			t.Errorf("result[%d] (%s) level = %q, want %q", i, res.RuleID, res.Level, wantLevel[res.RuleID])
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

func TestSARIFEmptyRunMetadataIsAdditiveForLegacyDecoders(t *testing.T) {
	t.Parallel()

	metadata := fakeEvidenceMetadata("go1.25.3", "1.25.0")
	var buffer bytes.Buffer
	emitSARIF(&buffer, nil, metadata)

	// This models a consumer compiled against the old run shape. SARIF property
	// bags are additive; the decoder must retain the standard run/results shape
	// while ignoring the newly populated run.properties value.
	var legacy struct {
		Runs []struct {
			Tool    sarifTool     `json:"tool"`
			Results []sarifResult `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(buffer.Bytes(), &legacy); err != nil {
		t.Fatalf("legacy SARIF decoder rejected additive run properties: %v", err)
	}
	if len(legacy.Runs) != 1 || legacy.Runs[0].Tool.Driver.Name != "perfscan" || legacy.Runs[0].Results == nil {
		t.Fatalf("legacy SARIF run shape changed: %+v", legacy.Runs)
	}

	var current sarifLog
	if err := json.Unmarshal(buffer.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if current.Runs[0].Properties.Toolchain != metadata.Toolchain {
		t.Fatalf("empty SARIF run toolchain = %+v, want %+v", current.Runs[0].Properties.Toolchain, metadata.Toolchain)
	}
}
