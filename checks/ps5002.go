package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5002 reports a nested loop accumulating a full symmetric matrix.
var PS5002 = register(&lint.Check{
	ID:       "PS5002",
	Category: "arith",
	Slug:     "symmetric-accumulation",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a nested loop accumulating a full symmetric matrix",
		Text: `m[i][j] += x[i]*x[j] over full ranges of i and j computes every
off-diagonal element twice: the result is symmetric by construction.
Accumulate one triangle (j ≤ i) and mirror it once at the end — half the
multiplies and half the memory traffic, and the mirrored value is the SAME
float, so the result is bit-identical.

Check that the consumer does not rely on the accumulation ORDER of the
mirrored half (a running use of m mid-build would observe the difference).

The automatic fix rewrites only the exact canonical nest — an outer
'for i := range x' or 'for i := 0; i < n; i++' whose body is EXACTLY the
same-form full-range inner loop over the same bound, whose body is EXACTLY
m[i][j] += x[i] * x[j] — into the triangle-plus-mirror form below. Any
other shape keeps the advisory report.`,
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
						diag := analysis.Diagnostic{
							Pos:     is.Pos(),
							End:     is.End(),
							Message: base + " accumulates a symmetric product over full ranges of " + outerVar + " and " + innerVar + " — every off-diagonal element is computed twice; accumulate one triangle and mirror it once (bit-identical)",
						}
						if len(outerBody.List) == 1 && len(innerBody.List) == 1 {
							if fix := ps5002TriangleFix(pass, n, stmt, is, outerVar, innerVar); fix != nil {
								diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
							}
						}
						pass.Report(diag)
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

// ps5002TriangleFix builds the Doc.After rewrite for the exact canonical
// nest and nothing else:
//
//	for i := range x { for j := range x { m[i][j] += x[i] * x[j] } }
//	for i := 0; i < n; i++ { for j := 0; j < n; j++ { m[i][j] += x[i] * x[j] } }
//
// Requirements, all checked here: the outer body is exactly the inner loop
// and the inner body exactly the accumulation (enforced by the caller via
// the len==1 checks and re-checked structurally); inner and outer loops
// have the same form and their bounds render to the same text (range form:
// the same range subject); the accumulation row index is the OUTER
// variable; matrix, vector and bound are simple ident/selector chains that
// do not involve the loop variables; matrix and range subject are of
// slice/array type so the mirror nest compiles. Anything else returns nil
// and the diagnostic stays advisory.
func ps5002TriangleFix(pass *analysis.Pass, outer ast.Node, inner ast.Stmt, accum ast.Stmt, i, j string) *analysis.SuggestedFix {
	as, ok := accum.(*ast.AssignStmt)
	if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return nil
	}
	target, ok := as.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return nil
	}
	row, ok := target.X.(*ast.IndexExpr)
	if !ok {
		return nil
	}
	// Canonical orientation only: the row index must be the outer variable
	// (symmetricAccum also accepts the transposed m[j][i]; the mirror nest
	// below is written for the m[i][j] orientation).
	if ri, ok := row.Index.(*ast.Ident); !ok || ri.Name != i {
		return nil
	}
	mText := simpleExprText(row.X)
	if mText == "" || rootIdentName(row.X) == i || rootIdentName(row.X) == j {
		return nil
	}
	// Both index levels must be slice/array so m[i][j] = m[j][i] compiles
	// with the int loop variables of the rewrite.
	if !ps5002Indexable(pass.TypesInfo.TypeOf(row.X)) || !ps5002Indexable(pass.TypesInfo.TypeOf(target.X)) {
		return nil
	}
	mul, ok := as.Rhs[0].(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	xi, ok1 := mul.X.(*ast.IndexExpr)
	xj, ok2 := mul.Y.(*ast.IndexExpr)
	if !ok1 || !ok2 {
		return nil
	}
	xText := simpleExprText(xi.X)
	if xText == "" || xText != simpleExprText(xj.X) || xText == mText {
		return nil
	}
	if r := rootIdentName(xi.X); r == i || r == j {
		return nil
	}

	var outerHeader, bound string
	switch o := outer.(type) {
	case *ast.RangeStmt:
		if o.Tok != token.DEFINE || o.Value != nil || len(o.Body.List) != 1 {
			return nil
		}
		src := simpleExprText(o.X)
		if src == "" {
			return nil
		}
		if r := rootIdentName(o.X); r == i || r == j {
			return nil
		}
		// Rules out maps, strings, channels and int ranges, where the
		// counted mirror nest would not compile or not be equivalent.
		if !ps5002Indexable(pass.TypesInfo.TypeOf(o.X)) {
			return nil
		}
		in, ok := inner.(*ast.RangeStmt)
		if !ok || in.Tok != token.DEFINE || in.Value != nil || len(in.Body.List) != 1 {
			return nil
		}
		if simpleExprText(in.X) != src {
			return nil
		}
		outerHeader = "for " + i + " := range " + src + " {"
		bound = "len(" + src + ")"
	case *ast.ForStmt:
		ov, ob, ok := ps5002Counted(o)
		if !ok || ov != i || len(o.Body.List) != 1 {
			return nil
		}
		obText, obRoot := ps5002BoundText(ob)
		if obText == "" || obRoot == i || obRoot == j {
			return nil
		}
		in, ok := inner.(*ast.ForStmt)
		if !ok {
			return nil
		}
		iv, ib, ok := ps5002Counted(in)
		if !ok || iv != j || len(in.Body.List) != 1 {
			return nil
		}
		ibText, _ := ps5002BoundText(ib)
		if ibText != obText {
			return nil
		}
		outerHeader = fmt.Sprintf("for %s := 0; %s < %s; %s++ {", i, i, obText, i)
		bound = obText
	default:
		return nil
	}

	ind := strings.Repeat("\t", pass.Fset.Position(outer.Pos()).Column-1)
	accumText := exprTextRendered(as.Lhs[0]) + " += " + exprTextRendered(as.Rhs[0])
	var b strings.Builder
	b.WriteString(outerHeader + "\n")
	fmt.Fprintf(&b, "%s\tfor %s := 0; %s <= %s; %s++ {\n", ind, j, j, i, j)
	b.WriteString(ind + "\t\t" + accumText + "\n")
	b.WriteString(ind + "\t}\n")
	b.WriteString(ind + "}\n")
	b.WriteString(ind + outerHeader + " // mirror once\n")
	fmt.Fprintf(&b, "%s\tfor %s := %s + 1; %s < %s; %s++ {\n", ind, j, i, j, bound, j)
	fmt.Fprintf(&b, "%s\t\t%s[%s][%s] = %s[%s][%s]\n", ind, mText, i, j, mText, j, i)
	b.WriteString(ind + "\t}\n")
	b.WriteString(ind + "}")
	return &analysis.SuggestedFix{
		Message: "accumulate one triangle and mirror it once",
		TextEdits: []analysis.TextEdit{
			{Pos: outer.Pos(), End: outer.End(), NewText: []byte(b.String())},
		},
	}
}

// ps5002Counted matches the exact counted header `for v := 0; v < B; v++`,
// returning the variable name and the bound expression B.
func ps5002Counted(l *ast.ForStmt) (string, ast.Expr, bool) {
	init, ok := l.Init.(*ast.AssignStmt)
	if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return "", nil, false
	}
	v, ok := init.Lhs[0].(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	zero, ok := init.Rhs[0].(*ast.BasicLit)
	if !ok || zero.Kind != token.INT || zero.Value != "0" {
		return "", nil, false
	}
	cond, ok := l.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.LSS {
		return "", nil, false
	}
	if cv, ok := cond.X.(*ast.Ident); !ok || cv.Name != v.Name {
		return "", nil, false
	}
	post, ok := l.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC {
		return "", nil, false
	}
	if pv, ok := post.X.(*ast.Ident); !ok || pv.Name != v.Name {
		return "", nil, false
	}
	return v.Name, cond.Y, true
}

// ps5002BoundText renders a counted-loop bound when it is loop-invariant by
// construction — a plain ident/selector chain or len() of one — returning
// the rendered text and the root identifier (for loop-variable exclusion).
func ps5002BoundText(e ast.Expr) (string, string) {
	if t := simpleExprText(e); t != "" {
		return t, rootIdentName(e)
	}
	if call, ok := e.(*ast.CallExpr); ok && astCalleeName(call) == "len" && len(call.Args) == 1 {
		if t := simpleExprText(call.Args[0]); t != "" {
			return "len(" + t + ")", rootIdentName(call.Args[0])
		}
	}
	return "", ""
}

// ps5002Indexable reports whether t's underlying type is a slice or array,
// i.e. indexable by the rewrite's int loop variables.
func ps5002Indexable(t types.Type) bool {
	if t == nil {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Slice, *types.Array:
		return true
	}
	return false
}
