package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS1002 reports per-element visitor helpers fed a closure — an indirect
// call per element.
var PS1002 = register(&lint.Check{
	ID:          "PS1002",
	Category:    "access",
	Slug:        "per-element-closure",
	Level:       lint.LevelStructured,
	NeedsConfig: true,
	Vocab:       []string{"perElementVisitors"},
	Doc: lint.Documentation{
		Title: "a per-element visitor fed a closure (an indirect call per element)",
		Text: `A visitor helper (readGen, fillGen, …) that takes a closure calls
it once per element: an indirect call the compiler cannot inline, per
element. Replace the closure with a typed bulk loop over the backing slice,
or specialize the visitor for the hot dtype.

Domain check: which helpers count as per-element visitors is project
vocabulary (perElementVisitors in perfscan.json).`,
		Before: `readGen(t, func(i int, v float64) {
	sum += v
})`,
		After: `if xs, ok := t.flatF64(); ok {
	for _, v := range xs {
		sum += v
	}
} else {
	readGen(t, func(i int, v float64) { sum += v })
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1002",
		Doc:  "per-element visitor fed a closure",
		Run:  runPS1002,
	},
})

func runPS1002(pass *analysis.Pass) (any, error) {
	visitors := config.Current().PerElementVisitors
	if len(visitors) == 0 {
		return nil, nil
	}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := astutil.CalleeName(call.Fun)
			if !visitors[name] {
				return true
			}
			hasClosure := false
			for _, a := range call.Args {
				if _, ok := a.(*ast.FuncLit); ok {
					hasClosure = true
					break
				}
			}
			if !hasClosure {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("per-element visitor %s is fed a closure — an indirect call per element; use a typed bulk loop over the backing slice for the hot dtype", name),
			})
			return true
		})
	}
	return nil, nil
}
