package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6100IndependentOwnLoopShapes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"empty", ``, false},
		{"one for", `for { break }`, false},
		{"one range", `for range xs {}`, false},
		{"two for", `for { break }; for { break }`, true},
		{"two range", `for range xs {}; for range xs {}`, true},
		{"for then range", `for { break }; for range xs {}`, true},
		{"range then for", `for range xs {}; for { break }`, true},
		{"nested own for", `for { for { break }; break }`, true},
		{"stored literal only", `_ = func(){for range xs {}; for range xs {}}`, false},
		{"invoked literal only", `func(){for range xs {}; for range xs {}}()`, false},
		{"one own plus literal", `for range xs {}; _ = func(){for range xs {}; for range xs {}}`, false},
		{"literal in range expression", `for range func() []int { for range xs {}; for range xs {}; return xs }() {}`, false},
		{"own loops around literal", `for range xs {}; _ = func(){for range xs {}}; for { break }`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "shape.go", "package p; func f(xs []int){"+tc.body+"}", parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			body := file.Decls[0].(*ast.FuncDecl).Body
			if got := ps6100HasNecessaryLoops(body); got != tc.want {
				t.Fatalf("necessary=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestPS6100IndependentImpossiblePackageEarlyReturn(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "empty.go", `package p
func external()
func one(xs []int) { for range xs {} }
func literal() { func(){ for {}; for {} }() }
`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately provide syntax only: an impossible package must return before
	// any of the expensive package summaries consult type information.
	pass := &analysis.Pass{Files: []*ast.File{file}}
	if result, err := runPS6100(pass); result != nil || err != nil {
		t.Fatalf("result=%v err=%v", result, err)
	}
}
