package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS1005 reports a per-element accessor whose 2+ index arguments are
// enclosing-loop variables — a manual multi-dimensional walk via dispatch
// that PS1001's element-count-loop check misses.
var PS1005 = register(&lint.Check{
	ID:          "PS1005",
	Category:    "access",
	Slug:        "manual-walk-dispatch",
	Level:       lint.LevelAggressive,
	NeedsConfig: true,
	Vocab:       []string{"elementAccessors"},
	Doc: lint.Documentation{
		Title: "a per-element accessor whose 2+ index args are enclosing-loop variables (a manual tensor walk)",
		Text: `t.AtF64(i, j) inside for i { for j { … } } is a manual
multi-dimensional tensor walk paying one dispatch per element — the shape
with the biggest measured payoff in the reference corpus, and also one of the
most share-dependent, so both numbers matter.

Rank candidates by (a) the dispatch count — the product of the loop bounds
times dispatches per iteration — and (b) the loop's SHARE of its enclosing
function: a walk bolted to a matmul-dominated path will not move the wall
clock however many dispatches it removes.

L3 (aggressive): the cost of the fix is decided by whether the dtype is
statically fixed. A constructor-pinned dtype makes it three lines (take the
typed storage once and index it) with no fallback needed; a dtype from an
input needs a dual path, which is a bit-identity claim between two arms
that must then be tested on BOTH. Read the construction before estimating
the work.`,
		Before: `for i := 0; i < n; i++ {
	for j := 0; j < d; j++ {
		out.SetF64(w.AtF64(i, j)*x, i, j)
	}
}`,
		After: `ws, os := w.Storage().F64(), out.Storage().F64() // dtype pinned by constructor
for i := 0; i < n; i++ {
	row := i * d
	for j := 0; j < d; j++ {
		os[row+j] = ws[row+j] * x
	}
}`,
		MeasuredWin: "reference corpus: SoftmaxRegression.PredictProba geomean -50.10% (2.0x), allocs -97.95%; but the same transform on a matmul-dominated path measured -0.61% — an 80x outcome spread from loop share",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1005",
		Doc:  "manual multi-dim tensor walk via per-element dispatch",
		Run:  runPS1005,
	},
})

func runPS1005(pass *analysis.Pass) (any, error) {
	ns := config.Current()
	if len(ns.ElementAccessors) == 0 {
		return nil, nil
	}
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ff := collectFuncFacts(fn, ns)
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
				// PS1001 owns accessor calls inside its element-count
				// domain; this check covers the manual walk outside it.
				if anyAncestorPerElem(ff.parent, ff.perElemLoop, call) {
					return true
				}
				loopVars := enclosingLoopVars(ff.parent, call)
				if len(loopVars) == 0 {
					return true
				}
				indexArgs := 0
				for _, a := range call.Args {
					if id, ok := a.(*ast.Ident); ok && loopVars[id.Name] {
						indexArgs++
					}
				}
				if indexArgs < 2 {
					return true
				}
				loop := nearestLoop(ff.parent, call)
				if loop == nil || reportedLoop[loop] {
					return true
				}
				reportedLoop[loop] = true
				pass.Report(analysis.Diagnostic{
					Pos:     call.Pos(),
					End:     call.End(),
					Message: fmt.Sprintf(".%s with %d enclosing-loop index args is a manual tensor walk paying a dispatch per element; with a constructor-pinned dtype, take the typed storage once and index it (dual path + bit-identity tests needed otherwise)", name, indexArgs),
				})
				return true
			})
		}
	}
	return nil, nil
}
