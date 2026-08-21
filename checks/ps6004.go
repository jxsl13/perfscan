package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6004 reports dual paths guarded by a configured fast-path helper's
// comma-ok — a bit-identity claim between two arms that tests must reach
// on BOTH sides.
var PS6004 = register(&lint.Check{
	ID:          "PS6004",
	Category:    "verify",
	Slug:        "unverified-dual-path",
	Level:       lint.LevelStructured,
	NeedsConfig: true,
	Vocab:       []string{"fastPathHelpers"},
	Doc: lint.Documentation{
		Title: "a fast-path/fallback dual path whose bit-identity claim needs coverage on both arms",
		Text: `if xs, ok := t.flatF64(); ok { fast } else { fallback } is a
bit-identity CLAIM: both arms must produce identical results forever. That
claim holds only while a test reaches BOTH arms — a suite whose fixtures
are all contiguous exercises the fast path and silently stops testing the
fallback.

This is a verification-gap advisory, not a performance finding: it reports
the population of dual paths so coverage can be checked deliberately. A
planted-panic run per arm (or a coverage assertion) settles each site.`,
		Before: `if xs, ok := t.flatF64(); ok {
	fastKernel(xs)
} else {
	fallbackKernel(t) // when did a test last reach this arm?
}`,
		After: `// same code — plus a test whose fixture is strided/other-dtype,
// asserting bit-identical output against the fast arm.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6004",
		Doc:  "fast-path/fallback dual path needing two-arm coverage",
		Run:  runPS6004,
	},
})

func runPS6004(pass *analysis.Pass) (any, error) {
	fastPaths := config.Current().FastPathHelpers
	if len(fastPaths) == 0 {
		return nil, nil
	}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			ifs, ok := n.(*ast.IfStmt)
			if !ok || ifs.Else == nil || ifs.Init == nil {
				return true
			}
			as, ok := ifs.Init.(*ast.AssignStmt)
			if !ok || len(as.Rhs) != 1 || len(as.Lhs) != 2 {
				return true
			}
			call, ok := as.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				name = fun.Name
			case *ast.SelectorExpr:
				name = fun.Sel.Name
			}
			if !fastPaths[name] {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     ifs.Pos(),
				End:     ifs.Cond.End(),
				Message: "dual path guarded by " + name + "'s comma-ok is a bit-identity claim between two arms; it holds only while a test reaches BOTH — plant a strided/other-dtype fixture for the fallback arm",
			})
			return true
		})
	}
	return nil, nil
}
