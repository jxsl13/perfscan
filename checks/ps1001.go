package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS1001 reports per-element accessor dispatch in an element-count/Unravel
// loop with no typed fast path in the same function.
var PS1001 = register(&lint.Check{
	ID:          "PS1001",
	Category:    "access",
	Slug:        "per-element-dispatch",
	Level:       lint.LevelStructured,
	NeedsConfig: true,
	Vocab:       []string{"elementAccessors", "elementCountMethods", "indexDecomposeFuncs"},
	Doc: lint.Documentation{
		Title: "per-element accessor dispatch in an element-count loop with no typed fast path",
		Text: `A loop bounded by an element count (n := t.Numel()) or driven by
a flat→multi-index decomposer (Unravel) that reads or writes through a
per-element accessor (AtF64/SetF64) pays an interface dispatch, a flat-offset
recompute and a bounds check per element. Walk the backing slice directly for
the contiguous case, keeping the per-element form as the strided/other-dtype
fallback.

Suppressed when the function already calls a configured typed bulk accessor
(fastPathHelpers): the per-element loop is then only the fallback, not the
hot path. KEEP THAT LIST COMPLETE — a fast-path helper missing from the
config makes this check report the very fallback the fast path exists to
guard.`,
		Before: `n := t.Numel()
for i := 0; i < n; i++ {
	t.SetF64(f(t.AtF64(i)), i)
}`,
		After: `if xs, ok := t.flatF64(); ok { // contiguous fast path
	for i, v := range xs {
		xs[i] = f(v)
	}
} else {
	n := t.Numel() // strided/other-dtype fallback
	for i := 0; i < n; i++ {
		t.SetF64(f(t.AtF64(i)), i)
	}
}`,
		MeasuredWin: "reference corpus: removing only the store half of the dispatches in four weight permutations measured geomean -47.64% (all 8 cells -44.98%..-49.07%, p=0.000)",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1001",
		Doc:  "per-element accessor dispatch in an element-count loop",
		Run:  runPS1001,
	},
})

func runPS1001(pass *analysis.Pass) (any, error) {
	ns := config.Current()
	if len(ns.ElementAccessors) == 0 || (len(ns.ElementCountMethods) == 0 && len(ns.IndexDecomposeFuncs) == 0) {
		return nil, nil
	}
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ff := collectFuncFacts(fn, &ns)
			if ff.hasFlat {
				continue
			}
			reportedLoop := map[ast.Node]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := astutil.CalleeName(call.Fun)
				if !ns.ElementAccessors[name] {
					return true
				}
				loop := nearestLoop(ff.parent, call)
				if loop == nil || reportedLoop[loop] {
					return true
				}
				if !anyAncestorPerElem(ff.parent, ff.perElemLoop, call) {
					return true
				}
				reportedLoop[loop] = true
				pass.Report(analysis.Diagnostic{
					Pos:     call.Pos(),
					End:     call.End(),
					Message: "per-element ." + name + " in an element-count/index loop with no configured typed bulk accessor in " + fn.Name.Name + "(); walk the backing slice directly for the contiguous case, keeping this form as the strided fallback",
				})
				return true
			})
		}
	}
	return nil, nil
}
