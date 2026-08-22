package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5063 reports slices.Compare(a, b) compared against the literal 0 with == or
// != — an equality test spelled through a three-way ordering scan — where
// slices.Equal answers exactly that question and stops at the first difference
// (or a length mismatch) without computing any ordering. The slices companion of
// PS5101 (bytes.Compare == 0). Byte slices are PS5055's / PS5101's domain and
// float elements (NaN) are excluded.
var PS5063 = register(&lint.Check{
	ID:       "PS5063",
	Category: "arith",
	Slug:     "slices-compare-equality",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.Compare used only for equality, where slices.Equal is faster",
		Text: `slices.Compare computes a full three-way ordering; testing its result
against 0 throws the ordering away and keeps only equality. slices.Equal answers
exactly that: it short-circuits on a length mismatch without touching an element
(Compare must scan the whole common prefix first) and compares with == rather
than cmp.Compare's three-way branch per element.

The rewrite is BIT-IDENTICAL for the element types matched: for integers and
strings, slices.Equal(a, b) == (slices.Compare(a, b) == 0) for every pair (both
treat nil and empty identically and read the same elements). FLOAT element types
are deliberately EXCLUDED: cmp.Compare orders two NaNs as equal (returns 0) while
== reports them unequal, so slices.Compare([NaN], [NaN]) == 0 is true but
slices.Equal is false. Byte-slice elements are also excluded — that shape is
PS5055's (to bytes.Compare) and PS5101's (to bytes.Equal).

The automatic fix rewrites slices.Compare(a, b) == 0 to slices.Equal(a, b) and
slices.Compare(a, b) != 0 to !slices.Equal(a, b), with the 0 on either side. The
arguments are left untouched in place (same evaluation, same order); only the
selected name changes (Compare -> Equal, keeping an aliased slices qualifier), so
the slices import is never orphaned. Only a comparison against the literal
constant 0 is matched — a variable or named constant holding 0, and every
ordering operator (<, <=, >, >=), are left alone.`,
		Before: `if slices.Compare(a, b) == 0 {`,
		After:  `if slices.Equal(a, b) {`,
		MeasuredWin: `On a 256-int pair differing only in the last element (Apple M2 Pro, go1.26): ` +
			`slices.Compare(a, b) == 0 ~250 ns/op vs slices.Equal(a, b) ~95 ns/op (~2.6x, 0 ` +
			`allocs/op either way); a length mismatch makes slices.Equal O(1) while Compare still ` +
			`scans the common prefix.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5063",
		Doc:  "slices.Compare compared against 0 instead of slices.Equal",
		Run:  runPS5063,
	},
})

func runPS5063(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
				return true
			}
			var call *ast.CallExpr
			var zeroOnLeft bool
			if c := ps5063CompareCall(pass, bin.X); c != nil && ps5101IsZeroLit(pass, bin.Y) {
				call, zeroOnLeft = c, false
			} else if c := ps5063CompareCall(pass, bin.Y); c != nil && ps5101IsZeroLit(pass, bin.X) {
				call, zeroOnLeft = c, true
			} else {
				return true
			}
			sel := call.Fun.(*ast.SelectorExpr)
			op, repl := "==", "slices.Equal(a, b)"
			if bin.Op == token.NEQ {
				op, repl = "!=", "!slices.Equal(a, b)"
			}

			var edits []analysis.TextEdit
			if zeroOnLeft {
				lead := ""
				if bin.Op == token.NEQ {
					lead = "!"
				}
				edits = append(edits, analysis.TextEdit{Pos: bin.Pos(), End: call.Pos(), NewText: []byte(lead)})
			} else if bin.Op == token.NEQ {
				edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: call.Pos(), NewText: []byte("!")})
			}
			edits = append(edits, analysis.TextEdit{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Equal")})
			if !zeroOnLeft {
				edits = append(edits, analysis.TextEdit{Pos: call.End(), End: bin.End()})
			}
			pass.Report(analysis.Diagnostic{
				Pos:     bin.Pos(),
				End:     bin.End(),
				Message: "slices.Compare(...) " + op + " 0 tests equality only; " + repl + " is faster and computes no ordering",
				SuggestedFixes: []analysis.SuggestedFix{{
					Message:   "replace with " + repl,
					TextEdits: edits,
				}},
			})
			return true
		})
	}
	return nil, nil
}

// ps5063CompareCall returns e as a call of the package-level slices.Compare
// whose element type is an integer (other than byte) or string — the types for
// which Compare == 0 is exactly Equal. Float elements are rejected (cmp.Compare
// orders two NaNs as equal while == does not), and byte elements are rejected
// (PS5055/PS5101's domain). A shadowed slices or a same-named method never
// matches.
func ps5063CompareCall(pass *analysis.Pass, e ast.Expr) *ast.CallExpr {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || call.Ellipsis != token.NoPos {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "Compare" || fn.Pkg() == nil || fn.Pkg().Path() != "slices" {
		return nil
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return nil
	}
	st, ok := pass.TypesInfo.TypeOf(call.Args[0]).Underlying().(*types.Slice)
	if !ok {
		return nil
	}
	basic, ok := st.Elem().Underlying().(*types.Basic)
	if !ok {
		return nil
	}
	info := basic.Info()
	switch {
	case info&types.IsString != 0:
		return call
	case info&types.IsInteger != 0 && basic.Kind() != types.Uint8:
		return call
	}
	return nil
}
