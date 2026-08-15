package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS1010 reports a nested loop over a slice of slices whose inner loop
// varies the ROW index — a column walk that jumps a whole row per step.
var PS1010 = register(&lint.Check{
	ID:       "PS1010",
	Category: "access",
	Slug:     "column-walk-slice-of-slices",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an inner loop reading one column of a [][]T (a row-jumping walk)",
		Text: `for j { for i { X[i][j] } } re-dereferences a row header and
touches a fresh cache line to read eight bytes, every step. The flat-array
strided checks cannot see this: they match index arithmetic, and X[i][j]
has none — a blind spot the reference paid for twice before adding this
check.

Reported ONLY when interchange is profitable: the inner loop must assign to
something that does not mention the inner variable — a scalar accumulator,
or a slot indexed only by the outer one. A transpose (out[j][i] = in[i][j])
strides whichever way it runs and is excluded by that clause. An inner body
that itself contains a loop is skipped (amortization filter): the strided
access then happens once against that loop's whole trip count.

Bit-identity is usually free — interchanging preserves each accumulator's
summation order when the accumulation is per-outer-index — but CONFIRM per
site. The check finds the SHAPE, not the payoff: the reference measured
-23.7% on one site and +3.7% (reverted) on another, so benchmark at the
site.`,
		Before: `for j := 0; j < d; j++ {
	s := 0.0
	for i := range rows {
		s += rows[i][j] // jumps a whole row per step
	}
	mean[j] = s / float64(len(rows))
}`,
		After: `sums := make([]float64, d)
for i := range rows { // row-major: one contiguous pass
	for j, v := range rows[i] {
		sums[j] += v
	}
}
for j := range mean {
	mean[j] = sums[j] / float64(len(rows))
}`,
		MeasuredWin: "reference corpus: GaussianNB.Fit -23.74% folding 2d column walks into two row-major passes; ballTree.build -18.71% on KNNFit; CholSolve's l[k][i] measured +3.74% and was reverted — confirm at the site",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1010",
		Doc:  "column walk over a slice of slices",
		Run:  runPS1010,
	},
})

func isSliceOfSlices(t types.Type) bool {
	s, ok := t.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	_, ok = s.Elem().Underlying().(*types.Slice)
	return ok
}

func runPS1010(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			iv, body := loopVarAndBody(n)
			if iv == "" || body == nil {
				return true
			}
			// Must sit inside an enclosing loop (the column index varies
			// somewhere above) and have no nested loop (amortization
			// filter).
			outerLoop, in := astutil.InLoop(stack)
			if !in {
				return true
			}
			if containsLoop(body) {
				return true
			}
			// Interchange must be profitable: every assignment target in
			// the body must not mention the inner variable.
			profitable := true
			ast.Inspect(body, func(m ast.Node) bool {
				as, ok := m.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for _, lhs := range as.Lhs {
					if exprMentions(lhs, iv) {
						profitable = false
						return false
					}
				}
				return true
			})
			if !profitable {
				return true
			}
			// A column read: X[iv][c] with c free of iv and X a [][]T.
			var hit *ast.IndexExpr
			var base string
			ast.Inspect(body, func(m ast.Node) bool {
				if hit != nil {
					return false
				}
				outer, ok := m.(*ast.IndexExpr)
				if !ok {
					return true
				}
				row, ok := outer.X.(*ast.IndexExpr)
				if !ok {
					return true
				}
				if !exprMentions(row.Index, iv) || exprMentions(outer.Index, iv) {
					return true
				}
				t := pass.TypesInfo.TypeOf(row.X)
				if t == nil || !isSliceOfSlices(t) {
					return true
				}
				hit = outer
				base = baseIdentName(row.X)
				return false
			})
			if hit == nil {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     hit.Pos(),
				End:     hit.End(),
				Message: "inner loop walks a COLUMN of " + base + " — a row header dereference and a fresh cache line per 8-byte read; interchange to a row-major pass (accumulators are per-outer-index here, so summation order per output is preserved — confirm per site)",
			}
			if fix := ps1010Fix(pass, f, outerLoop, n); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps1010CountedHeader matches the counted loop header `for v := 0; v < BOUND;
// v++` and returns the loop variable name and the bound expression.
func ps1010CountedHeader(l *ast.ForStmt) (string, ast.Expr, bool) {
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
	if x, ok := cond.X.(*ast.Ident); !ok || x.Name != v.Name {
		return "", nil, false
	}
	post, ok := l.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC {
		return "", nil, false
	}
	if x, ok := post.X.(*ast.Ident); !ok || x.Name != v.Name {
		return "", nil, false
	}
	return v.Name, cond.Y, true
}

// ps1010InvariantText renders a side-effect-free expression built only from
// identifiers, selector chains, int/float literals, len(...) over such an
// expression, and single-argument type conversions T(...) — verified against
// types.Info, not names. Anything else (index expressions, arithmetic,
// ordinary calls) returns "": it cannot be proven pure and loop-invariant, so
// it cannot be re-evaluated by the rewritten divide loop.
func ps1010InvariantText(pass *analysis.Pass, e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return simpleExprText(x)
	case *ast.BasicLit:
		if x.Kind == token.INT || x.Kind == token.FLOAT {
			return x.Value
		}
	case *ast.ParenExpr:
		if inner := ps1010InvariantText(pass, x.X); inner != "" {
			return "(" + inner + ")"
		}
	case *ast.CallExpr:
		id, ok := x.Fun.(*ast.Ident)
		if !ok || len(x.Args) != 1 || x.Ellipsis.IsValid() {
			return ""
		}
		arg := ps1010InvariantText(pass, x.Args[0])
		if arg == "" {
			return ""
		}
		switch obj := pass.TypesInfo.Uses[id].(type) {
		case *types.Builtin:
			if obj.Name() == "len" {
				return "len(" + arg + ")"
			}
		case *types.TypeName:
			return id.Name + "(" + arg + ")"
		}
	}
	return ""
}

// ps1010Fix builds the loop-interchange rewrite for the exact canonical
// column-mean shape and nothing wider:
//
//	for J := 0; J < D; J++ {
//		S := 0.0 // or 0
//		for I := range ROWS {
//			S += ROWS[I][J]
//		}
//		MEAN[J] = S / DIVISOR
//	}
//
// with ROWS and MEAN plain identifiers, ROWS a [][]T, D a side-effect-free
// simple expression, and DIVISOR pure and loop-invariant (free of J, I, S,
// and MEAN). The replacement accumulates into a column-sums slab while
// walking ROWS row-major, then divides in a second pass:
//
//	psColSumsN := make([]T, D)
//	for I := range ROWS {
//		for J := 0; J < D; J++ {
//			psColSumsN[J] += ROWS[I][J]
//		}
//	}
//	for J := 0; J < D; J++ {
//		MEAN[J] = psColSumsN[J] / DIVISOR
//	}
//
// Bit-identity: per output index J the slab accumulates ROWS[I][J] for I
// ascending — exactly the original inner loop's order — and the single
// divide sees the same completed sum and the same invariant divisor, so
// each MEAN[J] is bit-identical. The counted inner loop (not `range
// ROWS[I]`) reads exactly the element set the original read, so ragged
// rows still panic rather than silently changing the sums. Extra
// statements, non-zero seeds, non-counted outer loops, or a varying
// divisor keep the plain advisory report.
func ps1010Fix(pass *analysis.Pass, file *ast.File, outerNode, innerNode ast.Node) *analysis.SuggestedFix {
	outer, ok := outerNode.(*ast.ForStmt)
	if !ok {
		return nil
	}
	jVar, dExpr, ok := ps1010CountedHeader(outer)
	if !ok || len(outer.Body.List) != 3 {
		return nil
	}
	d := simpleExprText(dExpr)
	if d == "" {
		return nil
	}
	inner, ok := outer.Body.List[1].(*ast.RangeStmt)
	if !ok || ast.Node(inner) != innerNode {
		return nil
	}
	if inner.Tok != token.DEFINE || inner.Value != nil {
		return nil
	}
	iID, ok := inner.Key.(*ast.Ident)
	if !ok || iID.Name == "_" {
		return nil
	}
	iVar := iID.Name
	rowsID, ok := inner.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if t := pass.TypesInfo.TypeOf(rowsID); t == nil || !isSliceOfSlices(t) {
		return nil
	}
	// S := 0.0 (or S := 0): a zero seed, so make()'s zero value matches.
	seed, ok := outer.Body.List[0].(*ast.AssignStmt)
	if !ok || seed.Tok != token.DEFINE || len(seed.Lhs) != 1 || len(seed.Rhs) != 1 {
		return nil
	}
	sID, ok := seed.Lhs[0].(*ast.Ident)
	if !ok {
		return nil
	}
	zero, ok := seed.Rhs[0].(*ast.BasicLit)
	if !ok || (zero.Value != "0" && zero.Value != "0.0") {
		return nil
	}
	// Inner body: exactly { S += ROWS[I][J] }.
	if len(inner.Body.List) != 1 {
		return nil
	}
	acc, ok := inner.Body.List[0].(*ast.AssignStmt)
	if !ok || acc.Tok != token.ADD_ASSIGN || len(acc.Lhs) != 1 || len(acc.Rhs) != 1 {
		return nil
	}
	if lhs, ok := acc.Lhs[0].(*ast.Ident); !ok || lhs.Name != sID.Name {
		return nil
	}
	colIx, ok := acc.Rhs[0].(*ast.IndexExpr)
	if !ok {
		return nil
	}
	rowIx, ok := colIx.X.(*ast.IndexExpr)
	if !ok {
		return nil
	}
	if base, ok := rowIx.X.(*ast.Ident); !ok || base.Name != rowsID.Name {
		return nil
	}
	if ri, ok := rowIx.Index.(*ast.Ident); !ok || ri.Name != iVar {
		return nil
	}
	if ci, ok := colIx.Index.(*ast.Ident); !ok || ci.Name != jVar {
		return nil
	}
	// MEAN[J] = S / DIVISOR.
	fin, ok := outer.Body.List[2].(*ast.AssignStmt)
	if !ok || fin.Tok != token.ASSIGN || len(fin.Lhs) != 1 || len(fin.Rhs) != 1 {
		return nil
	}
	outIx, ok := fin.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return nil
	}
	outID, ok := outIx.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if ix, ok := outIx.Index.(*ast.Ident); !ok || ix.Name != jVar {
		return nil
	}
	quo, ok := fin.Rhs[0].(*ast.BinaryExpr)
	if !ok || quo.Op != token.QUO {
		return nil
	}
	if num, ok := quo.X.(*ast.Ident); !ok || num.Name != sID.Name {
		return nil
	}
	divisor := ps1010InvariantText(pass, quo.Y)
	if divisor == "" {
		return nil
	}
	// All five participating names must be pairwise distinct; the loop
	// writes only J, I, S, and MEAN[J], so the divisor and the bound must
	// be free of those names to stay invariant across the interchange
	// (mentioning ROWS — e.g. len(ROWS) — is fine: ROWS is never written).
	names := []string{jVar, iVar, sID.Name, rowsID.Name, outID.Name}
	for i := range names {
		for k := i + 1; k < len(names); k++ {
			if names[i] == names[k] {
				return nil
			}
		}
	}
	for _, written := range []string{jVar, iVar, sID.Name, outID.Name} {
		if exprMentions(quo.Y, written) {
			return nil
		}
	}
	dRoot := d
	if i := strings.IndexByte(dRoot, '.'); i >= 0 {
		dRoot = dRoot[:i]
	}
	for _, n := range names {
		if dRoot == n {
			return nil
		}
	}
	// Element type of the sums slab: the accumulator's own basic type.
	tname := ""
	if obj := pass.TypesInfo.Defs[sID]; obj != nil {
		if b, ok := obj.Type().(*types.Basic); ok && b.Info()&types.IsNumeric != 0 {
			tname = b.Name()
		}
	}
	if tname == "" {
		if zero.Value != "0.0" {
			return nil
		}
		tname = "float64"
	}
	// Fresh name: line-derived slab name. Bail on any user-written
	// identifier with that name anywhere in the file.
	pos := pass.Fset.Position(outer.Pos())
	sums := fmt.Sprintf("psColSums%d", pos.Line)
	collides := false
	ast.Inspect(file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == sums {
			collides = true
		}
		return !collides
	})
	if collides {
		return nil
	}
	// Assume gofmt indentation (tabs), matching the outer loop's column.
	ind := strings.Repeat("\t", pos.Column-1)
	var b strings.Builder
	wf := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	wf("%s := make([]%s, %s)\n", sums, tname, d)
	wf("%sfor %s := range %s {\n", ind, iVar, rowsID.Name)
	wf("%s\tfor %s := 0; %s < %s; %s++ {\n", ind, jVar, jVar, d, jVar)
	wf("%s\t\t%s[%s] += %s[%s][%s]\n", ind, sums, jVar, rowsID.Name, iVar, jVar)
	wf("%s\t}\n", ind)
	wf("%s}\n", ind)
	wf("%sfor %s := 0; %s < %s; %s++ {\n", ind, jVar, jVar, d, jVar)
	wf("%s\t%s[%s] = %s[%s] / %s\n", ind, outID.Name, jVar, sums, jVar, divisor)
	wf("%s}", ind)
	return &analysis.SuggestedFix{
		Message: "interchange to a row-major pass over " + rowsID.Name + " with a column-sums slab",
		TextEdits: []analysis.TextEdit{
			{Pos: outer.Pos(), End: outer.End(), NewText: []byte(b.String())},
		},
	}
}
