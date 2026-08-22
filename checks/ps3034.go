package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3034 reports a serial multi-level nest whose every indexed write names
// the outermost loop variable, in a package that declares a fan-out helper
// this function never calls.
var PS3034 = register(&lint.Check{
	ID:          "PS3034",
	Category:    "indirect",
	Slug:        "serial-nest-disjoint-writes",
	Level:       lint.LevelAggressive,
	NeedsConfig: true,
	Vocab:       []string{"fanOutHelpers"},
	Doc: lint.Documentation{
		Title: "a serial nest whose every write names the outermost loop variable",
		Text: `When every indexed write in a multi-level nest names the
outermost loop variable, the outer iterations own disjoint output rows and
the nest is bandable: split the outer loop across workers.

The function never calls the package's fan-out helper — the transform is
available (a sibling declares or uses it) and this nest was left behind.

L3 (aggressive): verify the band owns disjoint output, then gate with BOTH
a bit-exact digest and -race. An accumulated destination (+=) double-counts
on an overlap so both gates fire; a pure permutation writes the same value
twice and only -race sees it. Any scratch the body reuses must become
per-worker.`,
		Before: `for b := 0; b < batch; b++ { // serial; dst rows disjoint by b
	for j := 0; j < d; j++ {
		dst[b][j] = f(src[b][j])
	}
}`,
		After: `parallel.For(batch, func(b int) { // digest + -race gated
	for j := 0; j < d; j++ {
		dst[b][j] = f(src[b][j])
	}
})`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3034",
		Doc:  "serial nest with outer-variable-owned writes",
		Run:  runPS3034,
	},
})

func runPS3034(pass *analysis.Pass) (any, error) {
	fanout := config.Current().FanOutHelpers
	if len(fanout) == 0 || !packageHasFanOut(pass, fanout) {
		return nil, nil
	}
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || callsFanOut(fn.Body, fanout) {
				continue
			}
			topLevelNests(fn.Body, func(nest ast.Node) {
				outerVar := outermostLoopVar(nest)
				if outerVar == "" {
					return
				}
				writes := nestWrites(nest)
				if len(writes) == 0 {
					return
				}
				for _, w := range writes {
					if !indexNames(w, outerVar) {
						return // PS3059's territory (derived bases) or not disjoint
					}
				}
				body := astutil.LoopBody(nest)
				pass.Report(analysis.Diagnostic{
					Pos:     nest.Pos(),
					End:     body.Lbrace,
					Message: "every write in this serial nest names outer variable " + outerVar + " — the outer iterations own disjoint output and a fan-out helper exists in this package; band the outer loop and gate with BOTH a bit-exact digest and -race",
				})
			})
		}
	}
	return nil, nil
}

// indexNames reports whether the write's full index chain mentions v.
func indexNames(ix *ast.IndexExpr, v string) bool {
	for {
		if exprMentions(ix.Index, v) {
			return true
		}
		inner, ok := ix.X.(*ast.IndexExpr)
		if !ok {
			return false
		}
		ix = inner
	}
}
