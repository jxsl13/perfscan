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

// PS3005 reports sorting an index slice with a comparator that dereferences
// the sorted element into a 2-D structure.
var PS3005 = register(&lint.Check{
	ID:       "PS3005",
	Category: "indirect",
	Slug:     "indirect-key-comparator",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an index-slice sort whose comparator dereferences into a 2-D structure",
		Text: `sort.Slice(idx, func(a, b int) bool { return m[idx[a]][f] <
m[idx[b]][f] }) pays a row-pointer load plus an index per comparison,
O(n log n) times, for a value that depends only on the element. Fill a flat
id-indexed key column once (O(n)) and compare THAT.

The rewrite keeps the SAME PREDICATE, so the sort returns the same
permutation, ties included — this is not an argument about acceptable tie
orders. A comparator already reading a flat key (key[idx[a]]) is the fixed
form and is deliberately silent, so applying the advice clears the finding.
The flat key column is also what makes a radix pass practical afterwards.

The automatic fix applies only to the exact shape above — a
sort.Slice/SliceStable statement whose comparator body is a single '<'
return over m[idx[a]][f] and m[idx[b]][f] (either orientation), with m and
idx plain identifiers, m a slice or array, f an invariant plain
identifier/selector/literal, and an ordered basic element type. It inserts
the flat key prefill immediately before the sort statement and swaps the
comparator's key reads; the predicate is unchanged, so the permutation is
identical. Anything looser stays advisory.`,
		Before: `sort.Slice(idx, func(a, b int) bool {
	return m[idx[a]][f] < m[idx[b]][f]
})`,
		After: `key := make([]float64, len(m))
for i := range m {
	key[i] = m[i][f]
}
sort.Slice(idx, func(a, b int) bool { return key[idx[a]] < key[idx[b]] })`,
		MeasuredWin: "reference corpus: GBM presort 1.05x (1.10x cumulative once the flat key enabled a radix pass), ball-tree median split 1.088x on KNN fit",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3005",
		Doc:  "index sort comparator dereferencing a 2-D structure",
		Run:  runPS3005,
	},
})

var sortWithComparator = map[string]bool{
	"Slice":          true,
	"SliceStable":    true,
	"SortFunc":       true,
	"SortStableFunc": true,
}

func runPS3005(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			if !sortWithComparator[callName(call)] {
				return true
			}
			sorted := baseIdentName(call.Args[0])
			if sorted == "" {
				return true
			}
			lit, ok := call.Args[1].(*ast.FuncLit)
			if !ok || lit.Type.Params == nil {
				return true
			}
			var params []string
			for _, fl := range lit.Type.Params.List {
				for _, nm := range fl.Names {
					params = append(params, nm.Name)
				}
			}
			if len(params) != 2 {
				return true
			}
			base, found := doubleIndexThrough(lit.Body, sorted, params)
			if !found {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("the comparator sorting %s dereferences %s[%s[…]][…] on every comparison — a row-pointer load plus an index, O(n log n) times, for a value that depends only on the element; fill a flat id-indexed key column once and compare that (same predicate, identical permutation)", sorted, base, sorted),
			}
			if fix := flatKeyFix(pass, stack, call, lit, params); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// doubleIndexThrough reports whether body contains m[sorted[p]][…] for one
// of the comparator's parameters p. The OUTER IndexExpr is m[sorted[p]]
// indexed by f, so the sorted slice sits in the outer's own X's Index —
// getting that nesting backwards made the reference's first cut silent on
// all three sites it was written from.
func doubleIndexThrough(body *ast.BlockStmt, sorted string, params []string) (string, bool) {
	var base string
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		outer, ok := n.(*ast.IndexExpr)
		if !ok {
			return true
		}
		lookup, ok := outer.X.(*ast.IndexExpr)
		if !ok {
			return true
		}
		sel, ok := lookup.Index.(*ast.IndexExpr)
		if !ok || baseIdentName(sel.X) != sorted {
			return true
		}
		for _, p := range params {
			if exprMentions(sel.Index, p) {
				base, found = baseIdentName(lookup.X), true
				return false
			}
		}
		return true
	})
	return base, found
}

// ps3005Side is one side of the comparator's '<': m[idx[p]][f].
type ps3005Side struct {
	m     *ast.Ident     // the 2-D structure, a plain identifier
	p     *ast.Ident     // the comparator parameter used on this side
	f     ast.Expr       // the invariant key index
	fText string         // f rendered
	outer *ast.IndexExpr // the whole m[idx[p]][f], for element typing
}

// ps3005KeySide matches e against m[idxName[p]][f] with m and p plain
// identifiers and f a plain identifier, a selector chain over one, or a
// basic literal — the only key indexes the prefill can safely re-evaluate.
func ps3005KeySide(e ast.Expr, idxName string) (ps3005Side, bool) {
	var s ps3005Side
	outer, ok := e.(*ast.IndexExpr)
	if !ok {
		return s, false
	}
	lookup, ok := outer.X.(*ast.IndexExpr)
	if !ok {
		return s, false
	}
	m, ok := lookup.X.(*ast.Ident)
	if !ok {
		return s, false
	}
	sel, ok := lookup.Index.(*ast.IndexExpr)
	if !ok {
		return s, false
	}
	sx, ok := sel.X.(*ast.Ident)
	if !ok || sx.Name != idxName {
		return s, false
	}
	p, ok := sel.Index.(*ast.Ident)
	if !ok {
		return s, false
	}
	fText := ""
	if lit, isLit := outer.Index.(*ast.BasicLit); isLit {
		fText = lit.Value
	} else {
		fText = simpleExprText(outer.Index)
	}
	if fText == "" {
		return s, false
	}
	return ps3005Side{m: m, p: p, f: outer.Index, fText: fText, outer: outer}, true
}

// flatKeyFix builds the two-edit rewrite for the EXACT shape
//
//	sort.Slice(idx, func(a, b int) bool { return m[idx[a]][f] < m[idx[b]][f] })
//
// (sort.SliceStable too, and either orientation of a/b): insert a flat key
// prefill immediately before the sort statement and compare the flat column
// in the comparator. The predicate is unchanged, so the sort returns the
// identical permutation. Eligibility is deliberately narrow: m and idx are
// plain identifiers, m is a slice or array (a map cannot be prefilled
// row-for-row into make([]T, len(m))), f is an invariant plain
// identifier/selector/literal, the comparator body is exactly the one
// return, and the element type is an ordered basic type (its universe name
// needs no new import). Everything else stays advisory.
func flatKeyFix(pass *analysis.Pass, stack []ast.Node, call *ast.CallExpr, lit *ast.FuncLit, params []string) *analysis.SuggestedFix {
	fn, ok := astutil.PkgFuncCall(call.Fun, "sort", map[string]bool{"Slice": true, "SliceStable": true})
	if !ok || fn == "" {
		return nil
	}
	idx, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return nil
	}
	a, b := params[0], params[1]
	if a == b || a == "_" || b == "_" {
		return nil
	}
	// Body: exactly one statement, `return <lhs> < <rhs>`. Anything more
	// could run side effects per comparison that the rewrite would drop.
	if len(lit.Body.List) != 1 {
		return nil
	}
	ret, ok := lit.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil
	}
	bin, ok := ret.Results[0].(*ast.BinaryExpr)
	if !ok || bin.Op != token.LSS {
		return nil
	}
	lhs, ok := ps3005KeySide(bin.X, idx.Name)
	if !ok {
		return nil
	}
	rhs, ok := ps3005KeySide(bin.Y, idx.Name)
	if !ok {
		return nil
	}
	// Both sides identical except the parameter: same m, same f, and the
	// two comparator parameters used once each (either orientation).
	if lhs.m.Name != rhs.m.Name || lhs.fText != rhs.fText {
		return nil
	}
	if !((lhs.p.Name == a && rhs.p.Name == b) || (lhs.p.Name == b && rhs.p.Name == a)) {
		return nil
	}
	// f must be invariant: it may not read the comparator parameters, and
	// no involved name may collide with the prefill's loop variable.
	if exprMentions(lhs.f, a) || exprMentions(lhs.f, b) || exprMentions(lhs.f, "psI") {
		return nil
	}
	if lhs.m.Name == "psI" || idx.Name == "psI" {
		return nil
	}
	// m must be a slice or array: the prefill iterates its rows into a
	// column of len(m) indexed by row id.
	mType := pass.TypesInfo.TypeOf(lhs.m)
	if mType == nil {
		return nil
	}
	switch mType.Underlying().(type) {
	case *types.Slice, *types.Array:
	default:
		return nil
	}
	// The compared element must be an ordered basic type; its universe
	// name needs no new import in the fixed file.
	elem := pass.TypesInfo.TypeOf(lhs.outer)
	basic, ok := elem.(*types.Basic)
	if !ok || basic.Info()&types.IsOrdered == 0 {
		return nil
	}
	// The sort call must be a plain statement in a block so the prefill
	// can be inserted directly before it.
	if len(stack) < 2 {
		return nil
	}
	exprStmt, ok := stack[len(stack)-1].(*ast.ExprStmt)
	if !ok {
		return nil
	}
	switch stack[len(stack)-2].(type) {
	case *ast.BlockStmt, *ast.CaseClause, *ast.CommClause:
	default:
		return nil
	}
	pos := pass.Fset.Position(exprStmt.Pos())
	// Assume gofmt indentation (tabs): the statement starts at column
	// pos.Column, i.e. pos.Column-1 tabs of indentation.
	indent := strings.Repeat("\t", pos.Column-1)
	keyName := fmt.Sprintf("psKey%d", pos.Line)
	elemName := types.TypeString(elem, nil)
	prefill := fmt.Sprintf("%s := make([]%s, len(%s))\n%sfor psI := range %s {\n%s\t%s[psI] = %s[psI][%s]\n%s}\n%s",
		keyName, elemName, lhs.m.Name,
		indent, lhs.m.Name,
		indent, keyName, lhs.m.Name, lhs.fText,
		indent, indent)
	newCmp := fmt.Sprintf("%s[%s[%s]] < %s[%s[%s]]",
		keyName, idx.Name, lhs.p.Name, keyName, idx.Name, rhs.p.Name)
	return &analysis.SuggestedFix{
		Message: fmt.Sprintf("fill a flat key column once and compare that — the predicate is unchanged, so sort.%s returns the identical permutation", fn),
		TextEdits: []analysis.TextEdit{
			{Pos: exprStmt.Pos(), End: exprStmt.Pos(), NewText: []byte(prefill)},
			{Pos: bin.Pos(), End: bin.End(), NewText: []byte(newCmp)},
		},
	}
}
