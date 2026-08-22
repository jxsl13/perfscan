package runner

import (
	"bytes"
	"encoding/json"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/lint"
)

// TestFixJSONAppliesAndEmitsJSON pins the -fix + -json COMBINATION through Run():
// -fix applies its fixes and then falls through to the output switch (only -diff
// short-circuits), so -fix -json must both rewrite the file on disk AND print
// valid findings JSON. The -fix tests only inspect the file and emitJSON is unit
// tested in isolation — neither exercises the two together.
func TestFixJSONAppliesAndEmitsJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(corpusGoMod), 0o644); err != nil {
		t.Fatal(err)
	}
	// A version-gate-free fixable finding: fmt.Sprintf("%d", n) -> strconv.Itoa(n).
	const src = "package main\n\nimport \"fmt\"\n\nfunc F(n int) string { return fmt.Sprintf(\"%d\", n) }\n"
	mainGo := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainGo, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	var out, errBuf bytes.Buffer
	Run(checks.All(), Options{
		Patterns: []string{"./..."},
		MaxLevel: lint.LevelAggressive,
		Fix:      true,
		JSON:     true,
		Stdout:   &out,
		Stderr:   &errBuf,
	})

	// (a) the fix was applied on disk.
	got, err := os.ReadFile(mainGo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "strconv.Itoa(n)") {
		t.Errorf("-fix should have rewritten the fmt.Sprintf to strconv.Itoa(n):\n%s\nstderr: %s", got, errBuf.String())
	}

	// (b) stdout is valid findings JSON listing PS2107 — -fix must not suppress it.
	var findings []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out.Bytes(), &findings); err != nil {
		t.Fatalf("-fix -json stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if len(findings) != 1 || findings[0].ID != "PS2107" {
		t.Errorf("-fix -json must emit the PS2107 finding as JSON, got:\n%s", out.String())
	}
}

// TestJSONOutputStructure covers emitJSON (the -json output consumed by tooling
// and editors): parse the array and assert each finding's fields — id, category,
// level, autoFix, file, 1-based line/col, message — plus the nested fix/edit shape
// (message + edits with a resolved file:line, byte start<end, and the new text).
func TestJSONOutputStructure(t *testing.T) {
	c1 := &lint.Check{ID: "PS9001", Category: "indirect", Level: lint.LevelIdiomatic, AutoFix: true, Doc: lint.Documentation{Title: "t1"}}
	c2 := &lint.Check{ID: "PS9002", Category: "alloc", Level: lint.LevelStructured, Doc: lint.Documentation{Title: "t2"}}

	// A finding WITH a fix needs a fset so the edit's byte offsets/positions resolve.
	fset := token.NewFileSet()
	file := fset.AddFile("x.go", fset.Base(), 100)
	edit := analysis.TextEdit{Pos: file.Pos(10), End: file.Pos(19), NewText: []byte("slices.Sort")}

	findings := []Finding{
		{
			Check: c1, fset: fset,
			Pos:     token.Position{Filename: "x.go", Line: 3, Column: 5},
			End:     token.Position{Filename: "x.go", Line: 3, Column: 20},
			Message: "m1",
			Fixes:   []analysis.SuggestedFix{{Message: "use slices.Sort", TextEdits: []analysis.TextEdit{edit}}},
		},
		{
			Check:   c2,
			Pos:     token.Position{Filename: "y.go", Line: 8, Column: 1},
			End:     token.Position{Filename: "y.go", Line: 8, Column: 4},
			Message: "m2",
		},
	}

	var buf bytes.Buffer
	emitJSON(&buf, findings)

	var got []struct {
		ID       string `json:"id"`
		Category string `json:"category"`
		Level    string `json:"level"`
		AutoFix  bool   `json:"autoFix"`
		File     string `json:"file"`
		Line     int    `json:"line"`
		Col      int    `json:"col"`
		Message  string `json:"message"`
		Fixes    []struct {
			Message string `json:"message"`
			Edits   []struct {
				File    string `json:"file"`
				Line    int    `json:"line"`
				NewText string `json:"newText"`
				Start   int    `json:"start"`
				End     int    `json:"end"`
			} `json:"edits"`
		} `json:"fixes"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("-json is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(got) != 2 {
		t.Fatalf("-json has %d findings, want 2", len(got))
	}

	f0 := got[0]
	if f0.ID != "PS9001" || f0.Category != "indirect" || f0.Level != "L1" || !f0.AutoFix {
		t.Errorf("finding[0] core fields = %+v, want PS9001/indirect/L1/autoFix", f0)
	}
	if f0.File == "" || f0.Line != 3 || f0.Col != 5 || f0.Message != "m1" {
		t.Errorf("finding[0] pos/message = %+v", f0)
	}
	if len(f0.Fixes) != 1 {
		t.Fatalf("finding[0] fixes = %d, want 1", len(f0.Fixes))
	}
	fx := f0.Fixes[0]
	if fx.Message != "use slices.Sort" {
		t.Errorf("fix message = %q", fx.Message)
	}
	if len(fx.Edits) != 1 {
		t.Fatalf("fix edits = %d, want 1", len(fx.Edits))
	}
	ed := fx.Edits[0]
	if ed.NewText != "slices.Sort" {
		t.Errorf("edit newText = %q, want slices.Sort", ed.NewText)
	}
	if ed.File == "" || ed.Line < 1 {
		t.Errorf("edit location = %+v, want a resolved file:line", ed)
	}
	if ed.End <= ed.Start {
		t.Errorf("edit byte range %d..%d is not start<end", ed.Start, ed.End)
	}

	f1 := got[1]
	if f1.ID != "PS9002" || f1.Level != "L2" || f1.AutoFix {
		t.Errorf("finding[1] = %+v, want PS9002/L2/no-autoFix", f1)
	}
	if len(f1.Fixes) != 0 {
		t.Errorf("finding[1] should have no fixes, got %d", len(f1.Fixes))
	}
}
