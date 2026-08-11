package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS5008 reports functions calling BOTH math.Sin(x) and math.Cos(x) on the
// same argument expression — fusable to math.Sincos.
var PS5008 = register(&lint.Check{
	ID:       "PS5008",
	Category: "arith",
	Slug:     "sincos-fusable",
	Level:    lint.LevelIdiomatic,
	Doc: lint.Documentation{
		Title: "math.Sin and math.Cos on the same argument (fusable to math.Sincos)",
		Text: `Each of math.Sin(x) and math.Cos(x) performs the full argument
reduction of x independently; sin, cos := math.Sincos(x) does one reduction
and both polynomials. Go's math.Sincos shares Sin/Cos's exact reduction and
polynomials, so the fusion is bit-identical.

Wins only where trig dominates the kernel (positional encodings, rotations);
elsewhere it is a readability-neutral cleanup that costs nothing.`,
		Before: `s := math.Sin(theta)
c := math.Cos(theta)`,
		After: `s, c := math.Sincos(theta)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5008",
		Doc:  "Sin and Cos on the same argument",
		Run:  runPS5008,
	},
})

func runPS5008(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			sinArgs := map[string]*ast.CallExpr{}
			cosArgs := map[string]*ast.CallExpr{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					return true
				}
				name, ok := astutil.PkgFuncCall(call.Fun, "math", map[string]bool{"Sin": true, "Cos": true})
				if !ok {
					return true
				}
				key := exprTextRendered(call.Args[0])
				switch name {
				case "Sin":
					if _, dup := sinArgs[key]; !dup {
						sinArgs[key] = call
					}
				case "Cos":
					if _, dup := cosArgs[key]; !dup {
						cosArgs[key] = call
					}
				}
				return true
			})
			for key, sinCall := range sinArgs {
				if _, ok := cosArgs[key]; !ok {
					continue
				}
				pass.Report(analysis.Diagnostic{
					Pos:     sinCall.Pos(),
					End:     sinCall.End(),
					Message: "math.Sin and math.Cos are both computed on " + key + " — each repeats the full argument reduction; fuse to sin, cos := math.Sincos(" + key + ") (bit-identical)",
				})
			}
		}
	}
	return nil, nil
}
