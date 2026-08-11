package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3065 reports a single loop writing an indexed destination from a call
// to a function that itself loops, in a package that declares a fan-out
// helper this function never calls.
var PS3065 = register(&lint.Check{
	ID:          "PS3065",
	Category:    "indirect",
	Slug:        "serial-loop-over-an-expensive-call",
	Level:       lint.LevelAggressive,
	NeedsConfig: true,
	Vocab:       []string{"fanOutHelpers"},
	Doc: lint.Documentation{
		Title: "a single serial loop over an expensive call, with a fan-out helper available",
		Text: `Every nest-based serial check requires depth 2 or more — and a
loop whose per-item work lives inside a CALLEE has depth 1 however
expensive it is. When each iteration writes its own slot from a call to a
function that itself loops, the items are independent and the loop is
bandable.

L3 (aggressive): expect AMDAHL, NOT the core count — the reference's win
was 6x on the parallelized 40% of a profile, which is 1.27x overall. The
rewrite is bit-identical (each entry performs exactly the arithmetic it did
before; only which goroutine performs it moves), but gate with -race and
size the test above the fan-out helper's work gate.`,
		Before: `for j := range cols {
	cache[j] = kernelColumn(x, j) // kernelColumn loops over n rows
}`,
		After: `parallel.For(len(cols), func(j int) {
	cache[j] = kernelColumn(x, j) // disjoint slots, bit-identical
})`,
		MeasuredWin: "goai reference: RBF kernel cache BenchmarkSVCFit/n4000_rbf 6.99→5.48ms (-21.5%), from a fit that had scaled at 1.02x on twelve cores",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3065",
		Doc:  "serial single loop over an expensive package call",
		Run:  runPS3065,
	},
})

func runPS3065(pass *analysis.Pass) (any, error) {
	fanout := config.Current().FanOutHelpers
	if len(fanout) == 0 {
		return nil, nil
	}
	// Package facts: which declared functions loop, and whether any file
	// declares or calls a fan-out helper.
	loopingFuncs := map[string]bool{}
	hasFanOut := false
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if fanout[fn.Name.Name] {
				hasFanOut = true
			}
			if callsFanOut(fn.Body, fanout) {
				hasFanOut = true
			}
			if containsLoop(fn.Body) {
				loopingFuncs[fn.Name.Name] = true
			}
		}
	}
	if !hasFanOut {
		return nil, nil
	}

	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || callsFanOut(fn.Body, fanout) {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if !astutil.IsLoop(n) {
					return true
				}
				body := astutil.LoopBody(n)
				if body == nil || containsLoop(body) {
					return true // depth ≥2 belongs to the nest checks
				}
				callee, ok := loopWritesSlotFromCall(body, loopingFuncs)
				if !ok {
					return true
				}
				pass.Report(analysis.Diagnostic{
					Pos:     n.Pos(),
					End:     body.Lbrace,
					Message: fmt.Sprintf("each iteration writes its own slot from %s, which itself loops — depth-1 but expensive per item, and a fan-out helper exists in this package; band the loop (bit-identical; expect Amdahl, gate with -race)", callee),
				})
				return false
			})
		}
	}
	return nil, nil
}

// loopWritesSlotFromCall matches a body statement `dst[...] = call(...)`
// (or op=) where the callee is a package function that loops.
func loopWritesSlotFromCall(body *ast.BlockStmt, loopingFuncs map[string]bool) (string, bool) {
	for _, stmt := range body.List {
		as, ok := stmt.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			continue
		}
		if _, ok := as.Lhs[0].(*ast.IndexExpr); !ok {
			continue
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok {
			continue
		}
		name := astutil.CalleeName(call.Fun)
		if loopingFuncs[name] {
			return name, true
		}
	}
	return "", false
}
