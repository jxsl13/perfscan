package checks

import (
	"go/ast"
	"go/token"

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
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "math.Sin and math.Cos on the same argument (fusable to math.Sincos)",
		Text: `Each of math.Sin(x) and math.Cos(x) performs the full argument
reduction of x independently; sin, cos := math.Sincos(x) does one reduction
and both polynomials. Go's math.Sincos shares Sin/Cos's exact reduction and
polynomials, so the fusion is bit-identical.

Wins only where trig dominates the kernel (positional encodings, rotations);
elsewhere it is a readability-neutral cleanup that costs nothing.

The automatic fix applies only when the two calls are two consecutive simple
:= definitions in the same block (either order) on the identical argument
expression; those collapse to a single sin, cos := math.Sincos(x) definition.
All other placements are reported without a fix.`,
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
				diag := analysis.Diagnostic{
					Pos:     sinCall.Pos(),
					End:     sinCall.End(),
					Message: "math.Sin and math.Cos are both computed on " + key + " — each repeats the full argument reduction; fuse to sin, cos := math.Sincos(" + key + ") (bit-identical)",
				}
				if fix := sincosFuseFix(fn.Body, sinCall, key); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
				pass.Report(diag)
			}
		}
	}
	return nil, nil
}

// sincosFuseFix builds the fusion fix for the narrow eligible shape: two
// CONSECUTIVE statements in the same block, both simple single-name :=
// defines of the form `sv := math.Sin(arg)` and `cv := math.Cos(arg)`
// (either order, identical rendered arg), where the reported sinCall is one
// of the pair. The pair is replaced by `sv, cv := math.Sincos(arg)` — the
// Lhs order follows Sincos's (sin, cos) results regardless of which
// statement came first. Any other placement stays advisory (nil).
func sincosFuseFix(body *ast.BlockStmt, sinCall *ast.CallExpr, argText string) *analysis.SuggestedFix {
	var fix *analysis.SuggestedFix
	ast.Inspect(body, func(n ast.Node) bool {
		if fix != nil {
			return false
		}
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i := 0; i+1 < len(block.List); i++ {
			aName, aCall, ok := simpleMathTrigDefine(block.List[i])
			if !ok {
				continue
			}
			bName, bCall, ok := simpleMathTrigDefine(block.List[i+1])
			if !ok {
				continue
			}
			var sinName, cosName string
			switch {
			case aCall == sinCall && isMathCallNamed(bCall, "Cos"):
				sinName, cosName = aName, bName
			case bCall == sinCall && isMathCallNamed(aCall, "Cos"):
				sinName, cosName = bName, aName
			default:
				continue
			}
			if exprTextRendered(aCall.Args[0]) != argText || exprTextRendered(bCall.Args[0]) != argText {
				continue
			}
			fix = &analysis.SuggestedFix{
				Message: "fuse math.Sin and math.Cos into math.Sincos",
				TextEdits: []analysis.TextEdit{{
					Pos:     block.List[i].Pos(),
					End:     block.List[i+1].End(),
					NewText: []byte(sinName + ", " + cosName + " := math.Sincos(" + argText + ")"),
				}},
			}
			return false
		}
		return true
	})
	return fix
}

// simpleMathTrigDefine matches `name := math.Sin(x)` / `name := math.Cos(x)`
// with exactly one Lhs identifier and one Rhs call, returning the bound name
// and the call.
func simpleMathTrigDefine(stmt ast.Stmt) (string, *ast.CallExpr, bool) {
	as, ok := stmt.(*ast.AssignStmt)
	if !ok || as.Tok != token.DEFINE || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return "", nil, false
	}
	id, ok := as.Lhs[0].(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	call, ok := as.Rhs[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return "", nil, false
	}
	if _, ok := astutil.PkgFuncCall(call.Fun, "math", map[string]bool{"Sin": true, "Cos": true}); !ok {
		return "", nil, false
	}
	return id.Name, call, true
}

// isMathCallNamed reports whether call is math.<name>(...).
func isMathCallNamed(call *ast.CallExpr, name string) bool {
	_, ok := astutil.PkgFuncCall(call.Fun, "math", map[string]bool{name: true})
	return ok
}
