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

// TestFromExportLevelGatingAllTiers pins the MIDDLE tier of the level gate, which
// sampleExport (L1 + L2 only) leaves untested: -level 2 must include L1 and L2 but
// EXCLUDE L3, and -level 3 must include an L3 finding. The existing test brackets
// the gate from the ends (all-at-L3, only-L1-at-L1); a regression that mishandled
// the L2 selection specifically (e.g. an off-by-one in the level comparison) would
// slip past it. Uses one real check per tier: PX1001 (L1), PX2001 (L2),
// PX3021 (L3, performance-no-int-to-ptr).
func TestFromExportLevelGatingAllTiers(t *testing.T) {
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }

	mk := func(name string, off int) fixes.Diagnostic {
		return fixes.Diagnostic{
			DiagnosticName: name,
			DiagnosticMessage: fixes.DiagnosticMessage{
				Message: "m", FilePath: "/src/demo.cpp", FileOffset: off,
			},
		}
	}
	ef := &fixes.ExportFile{
		MainSourceFile: "/src/demo.cpp",
		Diagnostics: []fixes.Diagnostic{
			mk("performance-for-range-copy", 10),               // PX1001, L1
			mk("performance-inefficient-vector-operation", 40), // PX2001, L2
			mk("performance-no-int-to-ptr", 70),                // PX3021, L3
		},
	}

	ids := func(fs []Finding) map[string]bool {
		m := map[string]bool{}
		for _, f := range fs {
			m[f.ID] = true
		}
		return m
	}

	l1 := ids(FromExport(ef, catalog.LevelIdiomatic))
	if len(l1) != 1 || !l1["PX1001"] {
		t.Errorf("-level 1 = %v, want only PX1001", l1)
	}
	l2 := ids(FromExport(ef, catalog.LevelStructured))
	if len(l2) != 2 || !l2["PX1001"] || !l2["PX2001"] || l2["PX3021"] {
		t.Errorf("-level 2 = %v, want PX1001+PX2001 (NOT the L3 PX3021)", l2)
	}
	l3 := ids(FromExport(ef, catalog.LevelAggressive))
	if len(l3) != 3 || !l3["PX3021"] {
		t.Errorf("-level 3 = %v, want all three including the L3 PX3021", l3)
	}
}

// TestFromExportDeduplicatesRepeatedDiagnostics pins the report-layer guarantee
// that an identical diagnostic (same file + byte offset + check name) is reported
// ONCE, even if the --export-fixes document lists it more than once. When
// clang-tidy runs over several TUs, a finding anchored in a SHARED HEADER can be
// exported per-TU that includes the header; perfscanxx must not double-count it
// in its report/JSON/SARIF/summary. clang-tidy's console output currently
// collapses these upstream, but the report layer owns the guarantee rather than
// depending on that. A DIFFERENT check at the same offset (different name) is a
// distinct finding and must be kept.
func TestFromExportDeduplicatesRepeatedDiagnostics(t *testing.T) {
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }

	dup := fixes.Diagnostic{
		DiagnosticName: "performance-trivially-destructible", // PX3026
		DiagnosticMessage: fixes.DiagnosticMessage{
			Message:    "class can be made trivially destructible",
			FilePath:   "/inc/shared.h",
			FileOffset: 100,
		},
	}
	ef := &fixes.ExportFile{
		MainSourceFile: "/src/a.cpp",
		Diagnostics: []fixes.Diagnostic{
			dup, // as surfaced compiling a.cpp
			dup, // the SAME header finding surfaced again compiling b.cpp
			{ // a different check at the SAME offset — must be kept
				DiagnosticName: "performance-noexcept-swap", // PX3006
				DiagnosticMessage: fixes.DiagnosticMessage{
					Message:    "swap should be noexcept",
					FilePath:   "/inc/shared.h",
					FileOffset: 100,
				},
			},
		},
	}
	got := FromExport(ef, catalog.LevelAggressive)
	if len(got) != 2 {
		t.Fatalf("FromExport deduped = %d findings, want 2 (the repeated PX3026 collapsed, the distinct PX3006 kept):\n%+v", len(got), got)
	}
	ids := map[string]int{}
	for _, f := range got {
		ids[f.ID]++
	}
	if ids["PX3026"] != 1 {
		t.Errorf("PX3026 should appear exactly once after dedup, got %d", ids["PX3026"])
	}
	if ids["PX3006"] != 1 {
		t.Errorf("PX3006 (distinct check at same offset) must be kept, got %d", ids["PX3006"])
	}
}

// TestFromExportKeepsSameCheckAtDifferentOffsets pins the OFFSET component of the
// dedup key: the same check firing at two DIFFERENT byte offsets in one file (two
// trivially-destructible classes in a single header, say) is two genuine findings,
// not one. TestFromExportDeduplicatesRepeatedDiagnostics varies the check name at a
// fixed offset and TestFromExportDedupResolvesPathBeforeKeying varies the path form
// at a fixed offset — neither exercises the offset field, so a regression that
// dropped offset from the key (collapsing distinct occurrences to one) would slip
// past both.
func TestFromExportKeepsSameCheckAtDifferentOffsets(t *testing.T) {
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }

	mk := func(off int) fixes.Diagnostic {
		return fixes.Diagnostic{
			DiagnosticName: "performance-trivially-destructible", // PX3026
			DiagnosticMessage: fixes.DiagnosticMessage{
				Message: "class can be made trivially destructible", FilePath: "/inc/shared.h", FileOffset: off,
			},
		}
	}
	ef := &fixes.ExportFile{
		MainSourceFile: "/src/a.cpp",
		Diagnostics:    []fixes.Diagnostic{mk(100), mk(240)}, // same check, same file, two classes
	}
	got := FromExport(ef, catalog.LevelAggressive)
	if len(got) != 2 {
		t.Fatalf("same check at two offsets must stay two findings, got %d:\n%+v", len(got), got)
	}
	offs := map[int]bool{}
	for _, f := range got {
		offs[f.Offset] = true
	}
	if !offs[100] || !offs[240] {
		t.Errorf("both offsets must survive dedup; got offsets %v", offs)
	}
}

// TestFromExportDedupResolvesPathBeforeKeying pins that the cross-TU dedup keys
// on the RESOLVED absolute path, not the raw FilePath: the same shared-header
// finding can reach --export-fixes via different path forms across TUs — an
// absolute FilePath from one TU and a relative FilePath + BuildDirectory from
// another (a project with per-subdirectory build dirs). Both resolve to the same
// file, so they are one finding. Motivated by abseil-scale scanning (159 TUs)
// where header findings recur widely; a naive raw-FilePath key would not collapse
// these.
func TestFromExportDedupResolvesPathBeforeKeying(t *testing.T) {
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }

	ef := &fixes.ExportFile{
		MainSourceFile: "/proj/build/a.cpp",
		Diagnostics: []fixes.Diagnostic{
			{ // absolute FilePath
				DiagnosticName: "performance-trivially-destructible",
				DiagnosticMessage: fixes.DiagnosticMessage{
					Message: "m", FilePath: "/proj/inc/shared.h", FileOffset: 20,
				},
			},
			{ // relative FilePath + BuildDirectory -> resolves to the SAME abs file
				DiagnosticName: "performance-trivially-destructible",
				BuildDirectory: "/proj",
				DiagnosticMessage: fixes.DiagnosticMessage{
					Message: "m", FilePath: "inc/shared.h", FileOffset: 20,
				},
			},
		},
	}
	got := FromExport(ef, catalog.LevelAggressive)
	if len(got) != 1 {
		t.Fatalf("mixed abs/relative references to the same header must dedup to 1, got %d:\n%+v", len(got), got)
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

// TestTextMetaWithoutLevel pins the meta line for a LEVEL-LESS finding — a
// pass-through clang-diagnostic-* (compile error/warning) has no catalog entry,
// so Level == "". Text must then omit the level entirely, never emit a stray
// space where it would go. The other Text tests all use catalog findings with a
// level (PX1001 = L1), so the empty-level branch was untested; a regression that
// unconditionally appended " " + f.Level would leave a trailing space before the
// closing paren.
func TestTextMetaWithoutLevel(t *testing.T) {
	var buf bytes.Buffer
	Text(&buf, []Finding{
		// level-less, no fix
		{File: "a.cpp", Line: 3, Col: 5, Message: "compile error", ID: "clang-diagnostic-error"},
		// level-less, WITH a fix-it
		{File: "b.cpp", Line: 1, Col: 1, Message: "m2", ID: "clang-diagnostic-warning", Fixes: 2},
	})
	want := "a.cpp:3:5: compile error (clang-diagnostic-error)\n" +
		"b.cpp:1:1: m2 (clang-diagnostic-warning, fix available)\n"
	if got := buf.String(); got != want {
		t.Errorf("Text (level-less) =\n  %q\nwant\n  %q", got, want)
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

// TestSARIFEmptyFindingsIsValid pins the clean-scan SARIF: with NO findings the
// output must still be a well-formed SARIF 2.1.0 log — one run whose rules and
// results are EMPTY ARRAYS (not JSON null). GitHub Code Scanning uploads a SARIF
// file on every run, including clean ones; if results serialized as `null` (a nil
// slice) instead of `[]`, the upload action rejects the file and the whole scan
// job fails. This is the zero-finding counterpart to TestSARIFOutputIsStructurally
// Valid (which uses three findings) and is a pure unit test — no clang-tidy.
func TestSARIFEmptyFindingsIsValid(t *testing.T) {
	var buf bytes.Buffer
	if err := SARIF(&buf, nil); err != nil {
		t.Fatalf("SARIF(nil): %v", err)
	}
	var log struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Rules []json.RawMessage `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []json.RawMessage `json:"results"`
		} `json:"runs"`
	}
	raw := buf.Bytes()
	if err := json.Unmarshal(raw, &log); err != nil {
		t.Fatalf("clean-scan SARIF is not valid JSON: %v\n%s", err, raw)
	}
	if log.Version != "2.1.0" {
		t.Errorf("SARIF version = %q, want 2.1.0", log.Version)
	}
	if len(log.Runs) != 1 {
		t.Fatalf("runs = %d, want exactly 1 even with no findings", len(log.Runs))
	}
	if log.Runs[0].Results == nil {
		t.Errorf("results must be an empty array, not null (GitHub rejects null):\n%s", raw)
	}
	if len(log.Runs[0].Results) != 0 {
		t.Errorf("results = %d, want 0", len(log.Runs[0].Results))
	}
	if log.Runs[0].Tool.Driver.Rules == nil {
		t.Errorf("rules must be an empty array, not null:\n%s", raw)
	}
	// Belt-and-suspenders: the raw bytes must contain the empty arrays, not null.
	if !bytes.Contains(raw, []byte(`"results": []`)) {
		t.Errorf("expected `\"results\": []` literally in the clean-scan SARIF:\n%s", raw)
	}
}

// TestSARIFOmitsRegionForUnreadableOffset pins that a finding whose byte offset
// could not be resolved to a line (Line == 0, e.g. the file is unreadable) emits
// a SARIF location with NO region at all — never a region with startLine 0.
// GitHub Code Scanning rejects a startLine below 1 and can fail the whole upload,
// so the omitempty + nil-region path is a hard contract. TestJSONAndSARIF only
// covers readable findings (it asserts startLine >= 1), so the no-region branch
// was untested.
func TestSARIFOmitsRegionForUnreadableOffset(t *testing.T) {
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist } // lineCol -> 0

	ef := &fixes.ExportFile{
		MainSourceFile: "/src/a.cpp",
		Diagnostics: []fixes.Diagnostic{{
			DiagnosticName: "performance-for-range-copy",
			DiagnosticMessage: fixes.DiagnosticMessage{
				Message: "m", FilePath: "/src/a.cpp", FileOffset: 100,
			},
		}},
	}
	findings := FromExport(ef, catalog.LevelAggressive)
	if len(findings) != 1 || findings[0].Line != 0 {
		t.Fatalf("want 1 finding with Line 0 (unreadable offset), got %+v", findings)
	}

	var buf bytes.Buffer
	if err := SARIF(&buf, findings); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Runs []struct {
			Results []struct {
				Locations []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region *struct {
							StartLine int `json:"startLine"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("SARIF is not valid JSON: %v\n%s", err, buf.Bytes())
	}
	if len(doc.Runs) != 1 || len(doc.Runs[0].Results) != 1 {
		t.Fatalf("want 1 run with 1 result, got %+v", doc.Runs)
	}
	loc := doc.Runs[0].Results[0].Locations[0].PhysicalLocation
	if loc.ArtifactLocation.URI == "" {
		t.Error("location must still carry the file URI even without a region")
	}
	if loc.Region != nil {
		t.Errorf("unreadable-offset finding must emit NO region, got startLine=%d — a startLine below 1 breaks GitHub ingestion:\n%s",
			loc.Region.StartLine, buf.Bytes())
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

// TestJSONEmptyIsArray pins that JSON emits an empty ARRAY (not null) when there
// are no findings — tooling that parses -json expects an array unconditionally,
// and Go's json.Encode would otherwise render a nil slice as `null`.
func TestJSONEmptyIsArray(t *testing.T) {
	for _, findings := range [][]Finding{nil, {}} {
		var buf bytes.Buffer
		if err := JSON(&buf, findings); err != nil {
			t.Fatalf("JSON: %v", err)
		}
		got := strings.TrimSpace(buf.String())
		if got != "[]" {
			t.Errorf("JSON(%v) = %q, want %q", findings, got, "[]")
		}
		// Must be valid JSON that unmarshals to an empty array.
		var arr []Finding
		if err := json.Unmarshal(buf.Bytes(), &arr); err != nil {
			t.Errorf("JSON output not valid: %v", err)
		}
		if len(arr) != 0 {
			t.Errorf("unmarshaled %d findings, want 0", len(arr))
		}
	}
}

// TestLineColBounds pins lineCol's guard: an empty path, an unreadable file, and
// an out-of-range byte offset (negative or past EOF — e.g. a stale export whose
// offsets no longer match the file) all yield (0,0) rather than slicing out of
// range, while a valid offset maps to a 1-based line:col.
func TestLineColBounds(t *testing.T) {
	if l, c := lineCol("", 5); l != 0 || c != 0 {
		t.Errorf("lineCol(\"\", 5) = %d,%d, want 0,0", l, c)
	}

	origRead := ReadFile
	defer func() { ReadFile = origRead }()

	// Unreadable file -> 0,0.
	ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	if l, c := lineCol("x.cpp", 3); l != 0 || c != 0 {
		t.Errorf("lineCol(unreadable) = %d,%d, want 0,0", l, c)
	}

	// Offsets out of range -> 0,0; a valid offset maps to line:col.
	ReadFile = func(string) ([]byte, error) { return []byte("ab\ncd\n"), nil } // len 6
	for _, off := range []int{-1, 7, 100} {
		if l, c := lineCol("x.cpp", off); l != 0 || c != 0 {
			t.Errorf("lineCol(off=%d) = %d,%d, want 0,0 (out of range)", off, l, c)
		}
	}
	// offset 4 is 'd' on line 2 (after "ab\ncd"): line 2, col 2.
	if l, c := lineCol("x.cpp", 4); l != 2 || c != 2 {
		t.Errorf("lineCol(off=4) = %d,%d, want 2,2", l, c)
	}
	// The INCLUSIVE boundary offset == len(data) is a VALID position (a finding
	// at end-of-file), not out of range: only offset > len is rejected. Here the
	// data ends with a newline, so offset 6 sits at the start of a hypothetical
	// line 3 → line 3, col 1. The out-of-range list above starts at 7, so a
	// regression tightening the guard to `>= len` (dropping this position to 0,0)
	// would slip past every other case.
	if l, c := lineCol("x.cpp", 6); l != 3 || c != 1 {
		t.Errorf("lineCol(off=6==len) = %d,%d, want 3,1 (EOF position, not out of range)", l, c)
	}
}

// TestFromExportSortsAcrossFiles pins the primary sort key: findings from
// DIFFERENT files are ordered by file path first (then by offset within a
// file). The sampleExport fixture is single-file, so this exercises the
// file-differs branch of the sort comparator. Two files given out of order,
// each with two out-of-order offsets, must come back fully sorted.
func TestFromExportSortsAcrossFiles(t *testing.T) {
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }

	mk := func(file string, off int) fixes.Diagnostic {
		return fixes.Diagnostic{
			DiagnosticName: "performance-for-range-copy", // PX1001, L1
			DiagnosticMessage: fixes.DiagnosticMessage{
				Message: "x", FilePath: file, FileOffset: off,
			},
		}
	}
	ef := &fixes.ExportFile{
		MainSourceFile: "/src/b.cpp",
		// Deliberately scrambled: file b before a, and offsets descending.
		Diagnostics: []fixes.Diagnostic{
			mk("/src/b.cpp", 50), mk("/src/a.cpp", 40),
			mk("/src/b.cpp", 10), mk("/src/a.cpp", 20),
		},
	}
	got := FromExport(ef, catalog.LevelAggressive)
	if len(got) != 4 {
		t.Fatalf("got %d findings, want 4", len(got))
	}
	type fo struct {
		file string
		off  int
	}
	want := []fo{
		{"/src/a.cpp", 20}, {"/src/a.cpp", 40}, // a before b, ascending offset
		{"/src/b.cpp", 10}, {"/src/b.cpp", 50},
	}
	for i, w := range want {
		if got[i].File != w.file || got[i].Offset != w.off {
			t.Errorf("finding[%d] = (%s, %d), want (%s, %d)", i, got[i].File, got[i].Offset, w.file, w.off)
		}
	}
}

// TestFromExportStableOrderForColocatedChecks pins a TOTAL, input-order-independent
// output order when two DIFFERENT checks fire at the same file+offset. clang-tidy
// emits such co-located diagnostics in TU-processing order, which varies run-to-run
// (parallel runs, TU ordering) and across clang-tidy versions; without an ID/message
// tiebreaker the report order would follow that unstable input order and make CI
// diffs noisy. Feeds the two diagnostics in BOTH orders and requires the same
// ID-sorted result each time.
func TestFromExportStableOrderForColocatedChecks(t *testing.T) {
	origRead := ReadFile
	defer func() { ReadFile = origRead }()
	ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }

	mk := func(name string) fixes.Diagnostic {
		return fixes.Diagnostic{
			DiagnosticName: name,
			DiagnosticMessage: fixes.DiagnosticMessage{
				Message: "m", FilePath: "/src/a.cpp", FileOffset: 100,
			},
		}
	}
	// performance-noexcept-swap -> PX3006, performance-trivially-destructible -> PX3026.
	a, b := mk("performance-noexcept-swap"), mk("performance-trivially-destructible")

	for _, order := range [][]fixes.Diagnostic{{a, b}, {b, a}} {
		ef := &fixes.ExportFile{MainSourceFile: "/src/a.cpp", Diagnostics: order}
		got := FromExport(ef, catalog.LevelAggressive)
		if len(got) != 2 {
			t.Fatalf("got %d findings, want 2 (both co-located checks kept)", len(got))
		}
		// PX3006 < PX3026 lexicographically, so it must come first regardless of
		// the input order.
		if got[0].ID != "PX3006" || got[1].ID != "PX3026" {
			t.Errorf("co-located findings not in stable ID order for input %v/%v: got [%s, %s], want [PX3006, PX3026]",
				order[0].DiagnosticName, order[1].DiagnosticName, got[0].ID, got[1].ID)
		}
	}
}
