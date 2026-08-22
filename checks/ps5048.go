package checks

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS5048 reports strconv.Itoa(a) == strconv.Itoa(b) (and !=) — formatting two
// ints to decimal strings only to compare them — where a == b compares the ints
// directly, with no allocation and no formatting. Two throwaway string
// allocations and two base-10 conversions collapse to one integer comparison.
var PS5048 = register(&lint.Check{
	ID:       "PS5048",
	Category: "alloc",
	Slug:     "itoa-compare-to-int-compare",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strconv.Itoa(a) == strconv.Itoa(b) formats both ints to throwaway strings just to compare them; a == b compares the ints directly",
		Text: `strconv.Itoa(a) == strconv.Itoa(b) converts each int to its decimal
string — two heap allocations and two base-10 formatting passes — and then
compares the strings. Because Itoa is injective (distinct ints have distinct
decimal strings, equal ints identical ones), the string comparison answers
exactly a == b, which the machine does in one instruction with no allocation and
no formatting. The == and != forms both collapse: Itoa(a) == Itoa(b) is a == b,
Itoa(a) != Itoa(b) is a != b.

The rewrite is BIT-IDENTICAL for equality and inequality only. It is deliberately
restricted to == and != — the ORDERING comparisons do NOT carry over, because
decimal strings compare lexicographically ("10" < "9") while ints compare
numerically (10 > 9), so those are left alone. Both operands are evaluated
exactly once in both forms.

The match is deliberately narrow: an == or != comparison whose BOTH operands are
a single-argument call of the package-level strconv.Itoa (a shadowed strconv
never matches). Each Itoa argument is a plain int expression, and every Go
operator that can appear in one (arithmetic, bitwise, unary) binds tighter than
== / !=, so unwrapping it needs no parentheses. The fix drops both Itoa(...)
wrappers, keeping the two arguments and the operator byte-verbatim; it is
withheld (advisory report only) when a comment sits inside a removed wrapper, or
when removing the two strconv references would orphan the strconv import (the
runner never prunes imports).`,
		Before: `if strconv.Itoa(a) == strconv.Itoa(b) {`,
		After:  `if a == b {`,
		MeasuredWin: `On two 7-digit ints (Apple M2 Pro, go1.26): strconv.Itoa(a) == strconv.Itoa(b) ` +
			`~33 ns/op, 16 B/op, 2 allocs/op vs a == b ~0.3 ns/op, 0 B/op, 0 allocs/op (~110x, ` +
			`both string allocations and both formatting passes replaced by one integer compare).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5048",
		Doc:  "strconv.Itoa(a) == / != strconv.Itoa(b) instead of a == / != b",
		Run:  runPS5048,
	},
})

func runPS5048(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			bin *ast.BinaryExpr
			fix *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
				return true
			}
			left, lok := ps5048ItoaCall(pass, bin.X)
			right, rok := ps5048ItoaCall(pass, bin.Y)
			if !lok || !rok {
				return true
			}
			var fix *analysis.SuggestedFix
			// Drop both Itoa(...) wrappers; a comment inside either removed span
			// withholds the fix.
			if !ps2109CommentBetween(f, left.Pos(), left.Args[0].Pos()) &&
				!ps2109CommentBetween(f, left.Args[0].End(), left.End()) &&
				!ps2109CommentBetween(f, right.Pos(), right.Args[0].Pos()) &&
				!ps2109CommentBetween(f, right.Args[0].End(), right.End()) {
				fix = &analysis.SuggestedFix{
					Message: "compare the ints directly",
					TextEdits: []analysis.TextEdit{
						{Pos: left.Pos(), End: left.Args[0].Pos()},
						{Pos: left.Args[0].End(), End: left.End()},
						{Pos: right.Pos(), End: right.Args[0].Pos()},
						{Pos: right.Args[0].End(), End: right.End()},
					},
				}
				fixable++
			}
			sites = append(sites, site{bin, fix})
			return true
		})
		// Each fixable comparison removes TWO strconv references (both Itoa
		// selectors); withhold all fixes if that would orphan the import.
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "strconv") > 2*fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.bin.Pos(),
				End:     st.bin.End(),
				Message: "strconv.Itoa(a) " + st.bin.Op.String() + " strconv.Itoa(b) formats two ints to throwaway strings just to compare them; a " + st.bin.Op.String() + " b compares the ints directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5048ItoaCall reports whether e is a single-argument call of the
// package-level strconv.Itoa, returning the call.
func ps5048ItoaCall(pass *analysis.Pass, e ast.Expr) (*ast.CallExpr, bool) {
	call, ok := ps2109Unparen(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil, false
	}
	if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "strconv", map[string]bool{"Itoa": true}); !ok {
		return nil, false
	}
	return call, true
}
