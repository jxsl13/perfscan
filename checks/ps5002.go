package checks

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5002 reports a nested loop accumulating a full symmetric matrix.
var PS5002 = register(&lint.Check{
	ID:       "PS5002",
	Category: "arith",
	Slug:     "symmetric-accumulation",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "a nested loop accumulating a full symmetric matrix",
		Text: `m[i][j] += x[i]*x[j] over full ranges of i and j computes every
off-diagonal element twice: the result is symmetric by construction.
Accumulate one triangle (j ≤ i) and mirror it once at the end — half the
multiplies and half the memory traffic, and the mirrored value is the SAME
float, so the result is bit-identical.

Check that the consumer does not rely on the accumulation ORDER of the
mirrored half (a running use of m mid-build would observe the difference).`,
		Before: `for i := range x {
	for j := range x {
		m[i][j] += x[i] * x[j]
	}
}`,
		After: `for i := range x {
	for j := 0; j <= i; j++ {
		m[i][j] += x[i] * x[j]
	}
}
for i := range x { // mirror once
	for j := i + 1; j < len(x); j++ {
		m[i][j] = m[j][i]
	}
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5002",
		Doc:  "full symmetric matrix accumulation",
		Run:  runPS5002,
	},
})

func runPS5002(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			outerVar, outerBody := loopVarAndBody(n)
			if outerVar == "" || outerBody == nil {
				return true
			}
			for _, stmt := range outerBody.List {
				innerVar, innerBody := loopVarAndBody(stmt)
				if innerVar == "" || innerBody == nil || innerVar == outerVar {
					continue
				}
				// Inner loop must cover the full range, not a triangle
				// (j := i / j <= i).
				if innerIsTriangular(stmt, outerVar) {
					continue
				}
				for _, is := range innerBody.List {
					if base, ok := symmetricAccum(is, outerVar, innerVar); ok {
						pass.Report(analysis.Diagnostic{
							Pos:     is.Pos(),
							End:     is.End(),
							Message: base + " accumulates a symmetric product over full ranges of " + outerVar + " and " + innerVar + " — every off-diagonal element is computed twice; accumulate one triangle and mirror it once (bit-identical)",
						})
					}
				}
			}
			return true
		})
	}
	return nil, nil
}

// innerIsTriangular reports whether the inner loop's init or condition
// references the outer variable (j := i, j <= i, j < i) — the
// already-triangular form.
func innerIsTriangular(s ast.Stmt, outerVar string) bool {
	l, ok := s.(*ast.ForStmt)
	if !ok {
		return false
	}
	if as, ok := l.Init.(*ast.AssignStmt); ok && len(as.Rhs) == 1 && exprMentions(as.Rhs[0], outerVar) {
		return true
	}
	return l.Cond != nil && exprMentions(l.Cond, outerVar)
}

// symmetricAccum matches m[i][j] += x[i] * x[j] (same base x, indices in
// either order), returning the accumulation target's rendered text.
func symmetricAccum(s ast.Stmt, i, j string) (string, bool) {
	as, ok := s.(*ast.AssignStmt)
	if !ok || as.Tok != token.ADD_ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return "", false
	}
	target, ok := as.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return "", false
	}
	row, ok := target.X.(*ast.IndexExpr)
	if !ok {
		return "", false
	}
	ri, ok1 := row.Index.(*ast.Ident)
	ci, ok2 := target.Index.(*ast.Ident)
	if !ok1 || !ok2 {
		return "", false
	}
	if !(ri.Name == i && ci.Name == j) && !(ri.Name == j && ci.Name == i) {
		return "", false
	}
	mul, ok := as.Rhs[0].(*ast.BinaryExpr)
	if !ok || mul.Op != token.MUL {
		return "", false
	}
	xi, ok1 := mul.X.(*ast.IndexExpr)
	xj, ok2 := mul.Y.(*ast.IndexExpr)
	if !ok1 || !ok2 {
		return "", false
	}
	if baseIdentName(xi.X) == "" || baseIdentName(xi.X) != baseIdentName(xj.X) {
		return "", false
	}
	ii, ok1 := xi.Index.(*ast.Ident)
	jj, ok2 := xj.Index.(*ast.Ident)
	if !ok1 || !ok2 {
		return "", false
	}
	if !(ii.Name == i && jj.Name == j) && !(ii.Name == j && jj.Name == i) {
		return "", false
	}
	return exprTextRendered(target), true
}
