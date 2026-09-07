package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

// Constant strings previously reached constant.Int64Val and crashed scans,
// even when their loop bodies contained no PS6083 candidate at all.
func TestPS6083ConstantStringRanges(t *testing.T) {
	t.Parallel()
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS6083.Analyzer, "ps6083strings")
}

func TestPS6083ConstantLoopMateriality(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		expression string
		wantRange  bool
		wantBound  bool
	}{
		{"integer four", "4", true, true},
		{"integer three", "3", false, false},
		{"negative integer", "-4", false, false},
		{"too large integer", "1 << 100", false, false},
		{"empty string", `""`, false, false},
		{"ascii four", `"abcd"`, true, false},
		{"unicode four", `"éééé"`, true, false},
		{"unicode two not four bytes", `"éé"`, false, false},
		{"unicode one not four bytes", `"😀"`, false, false},
		{"invalid UTF8 four", `"\xff\xff\xff\xff"`, true, false},
		{"invalid UTF8 three", `"\xff\xff\xff"`, false, false},
		{"integral float constant", "4.0", false, false},
		{"fractional constant", "4.5", false, false},
		{"complex constant", "4i", false, false},
		{"boolean constant", "true", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			files := token.NewFileSet()
			expression, err := parser.ParseExprFrom(files, "constant.go", test.expression, 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue)}
			if err := types.CheckExpr(files, nil, token.NoPos, expression, info); err != nil {
				t.Fatal(err)
			}
			pass := &analysis.Pass{Fset: files, TypesInfo: info}
			if got := ps6083MaterialRange(pass, expression); got != test.wantRange {
				t.Errorf("range %s: got %v, want %v", test.expression, got, test.wantRange)
			}
			if got := ps6083MaterialBound(pass, expression); got != test.wantBound {
				t.Errorf("bound %s: got %v, want %v", test.expression, got, test.wantBound)
			}
		})
	}
}
