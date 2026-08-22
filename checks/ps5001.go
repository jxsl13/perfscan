package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS5001 reports divides by a loop-invariant scalar on every iteration.
// MOST FINDINGS SHOULD BE DECLINED — see the documentation.
var PS5001 = register(&lint.Check{
	ID:       "PS5001",
	Category: "arith",
	Slug:     "loop-invariant-divide",
	Level:    lint.LevelAggressive,
	Doc: lint.Documentation{
		Title: "a divide by a loop-invariant scalar on every element",
		Text: `Hoisting inv := 1/d and multiplying trades the per-element
divide for a multiply. MOST FINDINGS SHOULD BE DECLINED: the
reciprocal-multiply rewrite is NOT bit-identical, and its speedup
evaporates as soon as the loop touches memory — which an elementwise
tensor loop always does.

The reference measured it precisely: pure arithmetic with 8 independent
chains and no memory -28.4%; the same arithmetic reading an L1-resident
slice indistinguishable (p=0.168); a load-divide-store loop pure noise.
Accuracy cost over 200k values: up to 1 ulp on 24–66% of inputs — exact
ONLY for power-of-two divisors.

Rank by whether the divide sits on a memory-free path; if it does not,
decline. Never apply where the result feeds a discrete step
(round/quantize/argmax). Reduction accumulators (softmax denominators) are
excluded — their divide is a normalization, not a config-scalar divide.`,
		Before: `for i := range xs {
	xs[i] = xs[i] / temp // memory-bound: hoisting buys nothing
}`,
		After: `invTemp := 1 / temp // only on a genuinely memory-free path,
for i := range xs {   // and only where a 1-ulp shift rides a tolerance
	xs[i] = xs[i] * invTemp
}`,
		MeasuredWin: "reference corpus: -28.4% on memory-free arithmetic; indistinguishable from noise on anything that loads or stores — the quoted 1.2–1.5x is the memory-free ceiling",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5001",
		Doc:  "loop-invariant scalar divide per element",
		Run:  runPS5001,
	},
})

func runPS5001(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			// Reduction accumulators are excluded: dividing by a softmax Σ
			// is a normalization whose divide is minor or parity-locked.
			accumulated := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				a, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				switch a.Tok {
				case token.ADD_ASSIGN, token.SUB_ASSIGN, token.MUL_ASSIGN:
					for _, lhs := range a.Lhs {
						if id, ok := lhs.(*ast.Ident); ok {
							accumulated[id.Name] = true
						}
					}
				}
				return true
			})
			type key struct {
				loop ast.Node
				name string
			}
			reported := map[key]bool{}
			astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
				be, ok := n.(*ast.BinaryExpr)
				if !ok || be.Op != token.QUO {
					return true
				}
				d, ok := be.Y.(*ast.Ident)
				if !ok || accumulated[d.Name] {
					return true
				}
				t := pass.TypesInfo.TypeOf(be.Y)
				if t == nil {
					return true
				}
				b, ok := t.Underlying().(*types.Basic)
				if !ok || b.Info()&types.IsFloat == 0 {
					return true
				}
				loop, inLoop := astutil.InLoop(stack)
				if !inLoop {
					return true
				}
				body := astutil.LoopBody(loop)
				if body == nil || reassigns(body, d.Name) || isLoopVar(loop, d.Name) {
					return true
				}
				k := key{loop, d.Name}
				if reported[k] {
					return true
				}
				reported[k] = true
				pass.Report(analysis.Diagnostic{
					Pos:     be.Pos(),
					End:     be.End(),
					Message: "divide by loop-invariant " + d.Name + " on every iteration; a reciprocal multiply is NOT bit-identical (≤1 ulp on up to 2/3 of inputs) and only pays on a memory-free path — MOST FINDINGS SHOULD BE DECLINED",
				})
				return true
			})
		}
	}
	return nil, nil
}
