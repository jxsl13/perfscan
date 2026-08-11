package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS1004 reports variadic accessor spread calls (At(idx...)) in loops
// outside PS1001's element-count domain.
var PS1004 = register(&lint.Check{
	ID:          "PS1004",
	Category:    "access",
	Slug:        "spread-accessor-in-loop",
	Level:       lint.LevelStructured,
	NeedsConfig: true,
	Vocab:       []string{"elementAccessors"},
	Doc: lint.Documentation{
		Title: "a variadic accessor spread call (At(idx...)) in a loop",
		Text: `t.AtF64(idx...) rebuilds the flat offset from the index slice
and bounds-checks each dimension on every call. In a loop that cost repeats
per iteration — and the spread form usually means the indices were already
computed once and stored, so the flat offset could be carried instead.

Covers the loops OUTSIDE PS1001's element-count domain (PS1001 owns
Numel/Unravel loops).`,
		Before: `for _, idx := range positions {
	v := t.AtF64(idx...) // offset rebuilt per call
	process(v)
}`,
		After: `for _, off := range flatOffsets { // offsets computed once
	process(data[off])
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1004",
		Doc:  "variadic accessor spread call in a loop",
		Run:  runPS1004,
	},
})

func runPS1004(pass *analysis.Pass) (any, error) {
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
			astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !call.Ellipsis.IsValid() {
					return true
				}
				name := astutil.CalleeName(call.Fun)
				if !ns.ElementAccessors[name] {
					return true
				}
				if _, inLoop := astutil.InLoop(stack); !inLoop {
					return true
				}
				// PS1001 owns the element-count domain.
				if anyAncestorPerElem(ff.parent, ff.perElemLoop, call) {
					return true
				}
				pass.Report(analysis.Diagnostic{
					Pos:     call.Pos(),
					End:     call.End(),
					Message: fmt.Sprintf(".%s(idx...) rebuilds the flat offset and bounds-checks every dimension per call; carry the flat offset instead", name),
				})
				return true
			})
		}
	}
	return nil, nil
}
