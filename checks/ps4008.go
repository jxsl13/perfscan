package checks

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS4008 reports a matmul-shaped nest whose innermost loop is a serial
// scalar dot accumulator.
var PS4008 = register(&lint.Check{
	ID:       "PS4008",
	Category: "vector",
	Slug:     "serial-dot-matmul",
	Level:    lint.LevelAggressive,
	Doc: lint.Documentation{
		Title: "a matmul whose innermost loop is a serial scalar dot accumulator",
		Text: `sum += a[…]*b[…] in the innermost loop of a ≥3-deep nest chains
every fused multiply-add on the previous one: the loop runs at FMA latency,
not throughput. An ikj/axpy loop order (or a small block of independent
accumulators) breaks the dependency chain and lets the core retire several
FMAs per cycle.

L3 (aggressive): reassociating a floating-point reduction changes the
rounding order, so the result is NOT bit-identical. Gate the rewrite with a
tolerance-based oracle or restructure so each output element keeps one
accumulation order (the ikj form does: each c[i][j] still sums k ascending).`,
		Before: `for i := range a {
	for j := range b[0] {
		sum := 0.0
		for k := range b {
			sum += a[i][k] * b[k][j] // latency chain
		}
		c[i][j] = sum
	}
}`,
		After: `for i := range a {
	for k := range b {
		aik := a[i][k]
		for j := range b[0] { // axpy: independent accumulators per j
			c[i][j] += aik * b[k][j]
		}
	}
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS4008",
		Doc:  "serial scalar dot accumulator in a matmul nest",
		Run:  runPS4008,
	},
})

func runPS4008(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			if !astutil.IsLoop(n) {
				return true
			}
			body := astutil.LoopBody(n)
			if body == nil || containsLoop(body) {
				return true
			}
			// Depth ≥3: at least two enclosing loops.
			depth := 1
			for _, anc := range stack {
				if astutil.IsLoop(anc) {
					depth++
				}
			}
			if depth < 3 {
				return true
			}
			if !hasSerialDotAccum(body) {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     n.Pos(),
				End:     body.Lbrace,
				Message: "innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain (reassociation is not bit-identical — gate with a tolerance oracle)",
			})
			return true
		})
	}
	return nil, nil
}

func containsLoop(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		if n != body && astutil.IsLoop(n) {
			found = true
			return false
		}
		return true
	})
	return found
}

// hasSerialDotAccum matches `s += a[…] * b[…]` with a plain scalar target.
func hasSerialDotAccum(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Tok != token.ADD_ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		if _, ok := as.Lhs[0].(*ast.Ident); !ok {
			return true
		}
		mul, ok := as.Rhs[0].(*ast.BinaryExpr)
		if !ok || mul.Op != token.MUL {
			return true
		}
		_, xIdx := mul.X.(*ast.IndexExpr)
		_, yIdx := mul.Y.(*ast.IndexExpr)
		if xIdx && yIdx {
			found = true
			return false
		}
		return true
	})
	return found
}
