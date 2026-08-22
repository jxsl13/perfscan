package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
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
multiplies and half the memory traffic.

The mirror m[i][j] = m[j][i] is bit-identical to the full accumulation ONLY
when m is SYMMETRIC at loop entry: the original leaves init[i][j] + x[i]*x[j]
in the upper cell while the mirror copies init[j][i] + x[j]*x[i], and since
x[i]*x[j] and x[j]*x[i] carry the same bits the two agree exactly when
init[i][j] == init[j][i]. The automatic fix therefore requires the
accumulation target to be a freshly zero-initialized LOCAL matrix — declared
as make([][]T, …), rows allocated only via m[k] = make([]T, …), and not
otherwise written before the loop — because an all-zero matrix is trivially
symmetric. A parameter, field, global, or pre-populated matrix (whose
off-diagonal entries could differ) keeps the advisory report.

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
// slice/array type so the mirror nest compiles; and — the bit-identity
// gate — the matrix is a provably fresh ALL-ZERO local (see
// ps5002FreshZeroMatrix), because the mirror is only exact when the matrix
// is symmetric at loop entry. Anything else returns nil and the diagnostic
// stays advisory.
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
	// The fresh-zero gate below reasons about a single local variable, so
	// the fix requires the matrix base to be a plain identifier (a
	// selector like s.m — a field, whose content is unknown — stays
	// advisory).
	base, ok := row.X.(*ast.Ident)
	if !ok || base.Name == i || base.Name == j {
		return nil
	}
	mText := base.Name
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
		outerHeader = "for " + i + " := 0; " + i + " < " + obText + "; " + i + "++ {"
		bound = obText
	default:
		return nil
	}

	// Bit-identity gate: the mirror m[i][j] = m[j][i] reproduces the full
	// accumulation only when m is symmetric at loop entry, so the fix
	// requires m provably ALL-ZERO there (all-zero ⇒ symmetric). A
	// parameter, field, global, or pre-populated matrix stays advisory.
	if !ps5002FreshZeroMatrix(pass, enclosingFuncBody(pass, outer), base, outer) {
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

// ps5002FreshZeroMatrix reports whether base — the accumulation target of
// the matched outer-product nest accumLoop — is provably an ALL-ZERO matrix
// every time the nest is entered. All-zero is trivially symmetric, which is
// the precondition that makes the triangle+mirror rewrite bit-identical:
// the original leaves init[i][j] + x[i]*x[j] in the upper cell while the
// mirror copies init[j][i] + x[j]*x[i], and the two agree exactly iff
// init[i][j] == init[j][i]. The tractable SOUND conditions, all required:
//
//  1. base resolves to a local *types.Var whose declaration is INSIDE
//     fnBody (not a parameter, receiver, field, or global) and reads
//     `m := make([][]T, …)` or `var m = make([][]T, …)` — nil rows and
//     made rows are all-zero — with a non-complex numeric element type
//     (scalar values cannot alias, and x[i]*x[j] is bitwise commutative);
//  2. every other write to m is a whole-row init `m[k] = make([]T, …)`
//     placed between the declaration and accumLoop, not inside a function
//     literal, and not inside a loop that encloses accumLoop without also
//     enclosing the declaration (re-zeroing a row between two runs of the
//     accumulation would break the symmetry the mirror relies on);
//  3. every read of m is harmless: a key-only `range m`, builtin
//     len(m)/cap(m), a pure element-value read m[a][b] (never assigned,
//     ++/--'d, or &-taken), or `return m` (the function instance ends, so
//     no later accumulation can observe the caller's mutations). Anything
//     that could alias m or a row — `m2 := m`, `row := m[k]`, `&m`,
//     passing m or m[k] to a call, append/copy/slicing, a value-binding
//     range, any use inside a func literal — disqualifies;
//  4. fnBody contains no goto (a backward jump could re-run a row init
//     after an accumulation without re-running the declaration).
//
// Writes lexically inside accumLoop itself are the accumulation being
// rewritten and are exempt. When in doubt, this returns false and the
// diagnostic stays advisory.
func ps5002FreshZeroMatrix(pass *analysis.Pass, fnBody *ast.BlockStmt, base *ast.Ident, accumLoop ast.Node) bool {
	if fnBody == nil {
		return false
	}
	info := pass.TypesInfo
	mObj, ok := info.Uses[base].(*types.Var)
	if !ok || mObj.IsField() {
		return false
	}
	outerSl, ok := mObj.Type().Underlying().(*types.Slice)
	if !ok {
		return false
	}
	rowSl, ok := outerSl.Elem().Underlying().(*types.Slice)
	if !ok {
		return false
	}
	elem, ok := rowSl.Elem().Underlying().(*types.Basic)
	if !ok || elem.Info()&(types.IsInteger|types.IsFloat) == 0 {
		return false
	}

	// Locate the declaration inside fnBody. Parameters, receivers, fields
	// and globals are all defined elsewhere and fail this search.
	var declNode ast.Node
	astutil.WithStack(fnBody, func(n ast.Node, stack []ast.Node) bool {
		id, isIdent := n.(*ast.Ident)
		if !isIdent || info.Defs[id] != types.Object(mObj) || len(stack) == 0 {
			return true
		}
		switch p := stack[len(stack)-1].(type) {
		case *ast.AssignStmt:
			if p.Tok == token.DEFINE && len(p.Lhs) == 1 && len(p.Rhs) == 1 &&
				p.Lhs[0] == ast.Expr(id) && ps5002IsMake(info, p.Rhs[0]) {
				declNode = p
			}
		case *ast.ValueSpec:
			if len(p.Names) == 1 && len(p.Values) == 1 && p.Names[0] == id &&
				ps5002IsMake(info, p.Values[0]) {
				declNode = p
			}
		}
		return true
	})
	if declNode == nil || declNode.End() > accumLoop.Pos() {
		return false
	}

	okZero := true
	astutil.WithStack(fnBody, func(n ast.Node, stack []ast.Node) bool {
		if !okZero {
			return false
		}
		if br, isBr := n.(*ast.BranchStmt); isBr && br.Tok == token.GOTO {
			okZero = false
			return false
		}
		id, isIdent := n.(*ast.Ident)
		if !isIdent || info.Uses[id] != types.Object(mObj) {
			return true
		}
		if ps5002Within(id, accumLoop) {
			return true // the accumulation being rewritten
		}
		if ps5002Within(id, declNode) {
			return true // defensive; the declared name is a Def, not a Use
		}
		if len(stack) == 0 {
			okZero = false
			return false
		}
		for _, anc := range stack {
			if _, isLit := anc.(*ast.FuncLit); isLit {
				okZero = false // unknowable execution schedule
				return false
			}
		}
		switch p := stack[len(stack)-1].(type) {
		case *ast.RangeStmt:
			if p.X == ast.Expr(id) && p.Value == nil {
				return true // key-only range: reads only the length
			}
		case *ast.ReturnStmt:
			return true // the function instance ends here
		case *ast.CallExpr:
			if len(p.Args) == 1 && p.Args[0] == ast.Expr(id) {
				if fun, isFun := p.Fun.(*ast.Ident); isFun && (fun.Name == "len" || fun.Name == "cap") {
					if _, isBuiltin := info.Uses[fun].(*types.Builtin); isBuiltin {
						return true
					}
				}
			}
		case *ast.IndexExpr:
			if p.X == ast.Expr(id) && ps5002MatrixIndexUse(info, p, stack, declNode, accumLoop) {
				return true
			}
		}
		okZero = false
		return false
	})
	return okZero
}

// ps5002MatrixIndexUse classifies a use of the matrix through one index
// level — ix1 is m[k] and stack holds the ancestors of the m ident (ix1
// last). Allowed: the row init m[k] = make([]T, …) subject to the placement
// rules of ps5002FreshZeroMatrix condition 2, and a pure element-value read
// m[a][b]. Everything else disqualifies.
func ps5002MatrixIndexUse(info *types.Info, ix1 *ast.IndexExpr, stack []ast.Node, declNode, accumLoop ast.Node) bool {
	if len(stack) < 2 {
		return false
	}
	switch p := stack[len(stack)-2].(type) {
	case *ast.AssignStmt:
		// Row init m[k] = make([]T, …): a fresh all-zero row.
		if p.Tok != token.ASSIGN || len(p.Lhs) != 1 || len(p.Rhs) != 1 ||
			p.Lhs[0] != ast.Expr(ix1) || !ps5002IsMake(info, p.Rhs[0]) {
			return false
		}
		// It must run between the declaration and the accumulation…
		if p.Pos() < declNode.End() || p.End() > accumLoop.Pos() {
			return false
		}
		// …and never BETWEEN two accumulations: a loop that encloses the
		// accumulation but not the declaration would re-run this row init
		// against an already-accumulated (non-zero) matrix.
		for _, anc := range stack {
			if astutil.IsLoop(anc) && ps5002Within(accumLoop, anc) && !ps5002Within(declNode, anc) {
				return false
			}
		}
		return true
	case *ast.IndexExpr:
		// m[a][b]: allowed only as a pure element VALUE read. The element
		// type is a scalar (enforced by the caller), so a value read
		// cannot alias the matrix.
		if p.X != ast.Expr(ix1) || len(stack) < 3 {
			return false
		}
		switch gp := stack[len(stack)-3].(type) {
		case *ast.AssignStmt:
			for _, l := range gp.Lhs {
				if l == ast.Expr(p) {
					return false // element write
				}
			}
			return true
		case *ast.IncDecStmt:
			return false // element write
		case *ast.UnaryExpr:
			return gp.Op != token.AND // &m[a][b] aliases an element
		}
		return true
	}
	return false
}

// ps5002IsMake reports whether e is a call of the builtin make.
func ps5002IsMake(info *types.Info, e ast.Expr) bool {
	call, ok := ast.Unparen(e).(*ast.CallExpr)
	if !ok {
		return false
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "make" {
		return false
	}
	_, ok = info.Uses[fun].(*types.Builtin)
	return ok
}

// ps5002Within reports whether node a lies lexically inside node b.
func ps5002Within(a, b ast.Node) bool {
	return a.Pos() >= b.Pos() && a.End() <= b.End()
}
