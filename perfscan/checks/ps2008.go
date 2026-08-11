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

// PS2008 reports loops that allocate one slice per iteration into
// ARR[loopvar] with a loop-invariant length — a single slab plus disjoint
// capped views would do.
var PS2008 = register(&lint.Check{
	ID:       "PS2008",
	Category: "alloc",
	Slug:     "per-row-make-slab",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a loop that allocates one uniform slice per iteration into ARR[i]",
		Text: `A loop filling ARR[i] = make([]T, L) with a loop-invariant L
performs one allocation per row where a single slab would do: allocate
make([]T, n*L) once and cut three-index views
ARR[i] = slab[i*L : (i+1)*L : (i+1)*L]. Bit-identical by construction —
make() zeroes and so does a fresh slab.

Preconditions the check cannot verify: the rows must die together (a slab
pins every row in it), must not be appended to (the 3-index cap makes an
append copy instead of corrupting the neighbor, but the benefit is then
lost), and must not be individually replaced later.

The automatic fix rewrites only the exact single-statement range form
'for i := range rows { rows[i] = make([]T, L) }' where L is an int-typed
product of identifiers, selectors, and integer literals; counted-for
variants and multi-statement bodies keep the advisory report.

Expect an allocation-count win always and a wall-clock win only when the
loop body is small relative to the allocator work; measure the enclosing
operation. The payoff scales with the iteration count, which is a runtime
fact — prefer sites whose loop bound is a data size over ones bounded by a
small constant.`,
		Before: `rows := make([][]float64, n)
for i := range rows {
	rows[i] = make([]float64, d)
}`,
		After: `rows := make([][]float64, n)
slab := make([]float64, n*d)
for i := range rows {
	rows[i] = slab[i*d : (i+1)*d : (i+1)*d]
}`,
		MeasuredWin: "reference corpus: classic DBSCAN block-allocator variant -66.18%/-74.13% allocs/op, -5.90%/-7.48% time (p=0.000, n=12); GMM PredictProba rows -11.17% at k=4 alongside allocs 605→94",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2008",
		Doc:  "per-row make with invariant length into ARR[loopvar]",
		Run:  runPS2008,
	},
})

func runPS2008(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			loopVar, body := loopVarAndBody(n)
			if loopVar == "" || body == nil {
				return true
			}
			for _, stmt := range body.List {
				as, ok := stmt.(*ast.AssignStmt)
				if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
					continue
				}
				idx, ok := as.Lhs[0].(*ast.IndexExpr)
				if !ok {
					continue
				}
				arr, ok := idx.X.(*ast.Ident)
				if !ok {
					continue
				}
				iv, ok := idx.Index.(*ast.Ident)
				if !ok || iv.Name != loopVar {
					continue
				}
				mk, ok := as.Rhs[0].(*ast.CallExpr)
				if !ok || astutil.CalleeName(mk.Fun) != "make" || len(mk.Args) < 2 {
					continue
				}
				if _, isSlice := mk.Args[0].(*ast.ArrayType); !isSlice {
					continue
				}
				if mentionsIdent(mk.Args[1], loopVar) {
					continue
				}
				diag := analysis.Diagnostic{
					Pos:     as.Pos(),
					End:     as.End(),
					Message: arr.Name + "[" + loopVar + "] gets its own make per iteration with a loop-invariant length; one slab plus 3-index views cuts the allocations to one (rows must die together and not be appended to)",
				}
				if rng, isRange := n.(*ast.RangeStmt); isRange {
					if fix := ps2008SlabFix(pass, rng, as, arr, loopVar, mk); fix != nil {
						diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
					}
				}
				pass.Report(diag)
			}
			return true
		})
	}
	return nil, nil
}

// ps2008SlabFix rewrites the exact shape
//
//	for i := range rows { rows[i] = make([]T, L) }
//
// into a single slab allocation plus disjoint 3-index views:
//
//	psSlabN := make([]T, len(rows)*(L))
//	for i := range rows {
//		rows[i] = psSlabN[i*(L) : (i+1)*(L) : (i+1)*(L)]
//	}
//
// Only the narrowest provably-safe shape is rewritten: a := range over a
// plain slice/array identifier with no value variable, a single-statement
// body, a two-argument make, and a loop-invariant length of type int built
// from identifiers, selectors, and integer literals under multiplication.
// Everything else keeps the advisory report without a fix.
func ps2008SlabFix(pass *analysis.Pass, rng *ast.RangeStmt, as *ast.AssignStmt, arr *ast.Ident, loopVar string, mk *ast.CallExpr) *analysis.SuggestedFix {
	if rng.Tok != token.DEFINE || rng.Value != nil || len(rng.Body.List) != 1 {
		return nil
	}
	rx, ok := rng.X.(*ast.Ident)
	if !ok || rx.Name != arr.Name {
		return nil
	}
	if as.Tok != token.ASSIGN || len(mk.Args) != 2 {
		return nil
	}
	if !ps2008SimpleLen(mk.Args[1]) {
		return nil
	}
	// len(ARR) and the range index are int; the rendered products only
	// type-check when L itself is int (or an untyped integer constant).
	bt, ok := pass.TypesInfo.TypeOf(mk.Args[1]).(*types.Basic)
	if !ok || (bt.Kind() != types.Int && bt.Kind() != types.UntypedInt) {
		return nil
	}
	// The rewrite indexes the slab with the loop variable, which must
	// enumerate exactly 0..len(ARR)-1: slices and arrays only (a map's
	// keys would not).
	ct := pass.TypesInfo.TypeOf(rng.X)
	if ct == nil {
		return nil
	}
	switch ct.Underlying().(type) {
	case *types.Slice, *types.Array:
	default:
		return nil
	}
	pos := pass.Fset.Position(rng.Pos())
	// Assume gofmt indentation (tabs): the loop starts at column pos.Column,
	// i.e. pos.Column-1 tabs of indentation.
	indent := strings.Repeat("\t", pos.Column-1)
	name := fmt.Sprintf("psSlab%d", pos.Line)
	elem := exprTextRendered(mk.Args[0])
	l := "(" + exprTextRendered(mk.Args[1]) + ")"
	var b strings.Builder
	fmt.Fprintf(&b, "%s := make(%s, len(%s)*%s)\n", name, elem, arr.Name, l)
	fmt.Fprintf(&b, "%sfor %s := range %s {\n", indent, loopVar, arr.Name)
	fmt.Fprintf(&b, "%s\t%s[%s] = %s[%s*%s : (%s+1)*%s : (%s+1)*%s]\n",
		indent, arr.Name, loopVar, name, loopVar, l, loopVar, l, loopVar, l)
	fmt.Fprintf(&b, "%s}", indent)
	return &analysis.SuggestedFix{
		Message: "allocate one slab and cut disjoint capped views",
		TextEdits: []analysis.TextEdit{
			{Pos: rng.Pos(), End: rng.End(), NewText: []byte(b.String())},
		},
	}
}

// ps2008SimpleLen reports whether e is a loop-invariant simple length
// expression: an identifier, a selector chain, an integer literal, or a
// product of those (parentheses allowed).
func ps2008SimpleLen(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return true
	case *ast.BasicLit:
		return x.Kind == token.INT
	case *ast.SelectorExpr:
		return ps2008SimpleLen(x.X)
	case *ast.ParenExpr:
		return ps2008SimpleLen(x.X)
	case *ast.BinaryExpr:
		return x.Op == token.MUL && ps2008SimpleLen(x.X) && ps2008SimpleLen(x.Y)
	}
	return false
}

// loopVarAndBody extracts the integer loop variable and body of
// `for i := ...` / `for i := range xs`.
func loopVarAndBody(n ast.Node) (string, *ast.BlockStmt) {
	switch l := n.(type) {
	case *ast.ForStmt:
		if as, ok := l.Init.(*ast.AssignStmt); ok && len(as.Lhs) == 1 {
			if id, ok := as.Lhs[0].(*ast.Ident); ok {
				return id.Name, l.Body
			}
		}
	case *ast.RangeStmt:
		if id, ok := l.Key.(*ast.Ident); ok && id.Name != "_" {
			return id.Name, l.Body
		}
	}
	return "", nil
}

func mentionsIdent(e ast.Expr, name string) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}
