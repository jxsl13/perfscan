package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6100ImpossibleInnerLoopPackageSkipsSummaries(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "inner_loops.go", `package p
func external()
func siblings(xs []int) { for range xs {}; for range xs {} }
func oneScan(xs []int) { for range xs { for range xs {} } }
func chain(xs []int) { for range xs { for range xs { for range xs {} } } }
func literalScans(xs []int) { for range xs { func(){ for range xs {}; for range xs {} }() } }
`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	// These shapes cannot populate any nearest-outer-loop group with two scans.
	// Syntax-only input pins the early return before type-consuming summaries.
	pass := &analysis.Pass{Files: []*ast.File{file}}
	if result, err := runPS6100(pass); result != nil || err != nil {
		t.Fatalf("result=%v err=%v", result, err)
	}
}

func TestPS6100RepeatedInnerLoopShapes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"empty", ``, false},
		{"top-level pair", `for range xs {}; for range xs {}`, false},
		{"outer with one scan", `for range xs { for range xs {} }`, false},
		{"nested chain", `for range xs { for range xs { for range xs {} } }`, false},
		{"different owners", `for range xs { for range xs {} }; for range xs { for range xs {} }`, false},
		{"sibling scans", `for range xs { for range xs {}; for range xs {} }`, true},
		{"mixed loop kinds", `for { for range xs {}; for i:=0; i<len(xs); i++ {} }`, true},
		{"branched siblings", `for range xs { if flag { for range xs {} } else { for range xs {} } }`, true},
		{"dead siblings", `for range xs { if false { for range xs {}; for range xs {} } }`, true},
		{"deeper siblings", `for range xs { for range xs { for range xs {}; for range xs {} } }`, true},
		{"stored literal", `for range xs { _ = func(){ for range xs {}; for range xs {} } }`, false},
		{"invoked literal", `for range xs { func(){ for range xs {}; for range xs {} }() }`, false},
		{"range-expression literal", `for range func() []int { for range xs {}; for range xs {}; return xs }() { for range xs {} }`, false},
		{"for-header literal", `for func(){ for range xs {}; for range xs {} }(); flag; { for range xs {} }`, false},
		{"labels and branches", `outer: for range xs { for range xs { break outer }; inner: for range xs { continue inner } }`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "shape.go", "package p; func f(xs []int, flag bool){"+tc.body+"}", parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			body := file.Decls[0].(*ast.FuncDecl).Body
			if got := ps6100HasRepeatedInnerLoops(body); got != tc.want {
				t.Fatalf("repeated inner loops=%v want=%v", got, tc.want)
			}
		})
	}
}
