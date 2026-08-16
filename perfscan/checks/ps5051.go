package checks

import (
	"go/ast"
	"go/constant"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5051 reports strconv.FormatInt(a, B) == strconv.FormatInt(b, B) (and !=),
// and the FormatUint twin — formatting two integers to base-B strings only to
// compare them — where a == b compares the integers directly, with no
// allocation and no formatting. The non-Itoa arm of PS5048 (which covers the
// base-10 strconv.Itoa form): two throwaway string allocations and two base-B
// conversions collapse to one integer comparison.
var PS5051 = register(&lint.Check{
	ID:       "PS5051",
	Category: "alloc",
	Slug:     "formatint-compare-to-int-compare",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strconv.FormatInt(a, B) == strconv.FormatInt(b, B) formats both integers to throwaway strings just to compare them; a == b compares the integers directly",
		Text: `strconv.FormatInt(a, B) == strconv.FormatInt(b, B) converts each integer
to its base-B string — two heap allocations and two formatting passes — and
then compares the strings. For any FIXED base B in [2, 36], FormatInt (over
int64) and FormatUint (over uint64) are injective: standard positional
notation, no leading zeros, a sign only on negatives, so equal strings appear
exactly when the values are equal. The string comparison therefore answers
exactly a == b, which the machine does in one instruction with no allocation
and no formatting. Both the == and != forms collapse.

The rewrite is BIT-IDENTICAL for equality and inequality only, and only when
BOTH sides share the SAME base. It is deliberately restricted to == and != —
ORDERING does NOT carry over (decimal strings compare lexicographically,
"10" < "9", while integers compare numerically). Different bases are NOT
equivalent either: FormatInt(21, 16) and FormatInt(15, 10) are both "15" though
21 != 15, so the base literals must match. Both operands are evaluated once in
both forms.

The match is deliberately narrow — it is the whole safety story:
  - an == or != comparison whose BOTH operands are a call of the package-level
    strconv.FormatInt, or both of strconv.FormatUint (a shadowed strconv, or a
    FormatInt-vs-FormatUint mix — whose int64 and uint64 operands would not even
    compare — never matches);
  - both base arguments are constant and EQUAL, with a value in [2, 36] — the
    valid-base range, so neither side can panic (an out-of-range base panics at
    runtime, which a == b would not reproduce);
  - the first argument of each call is a plain integer expression, and every Go
    operator that can appear in one (arithmetic, bitwise, shift, unary) binds
    tighter than == / !=, so unwrapping it needs no parentheses.
The fix drops both Format* wrappers and the base arguments, keeping the two
value expressions and the operator byte-verbatim; it is withheld (advisory
report only) when a comment sits inside a removed span, or when removing the two
strconv references would orphan the strconv import (the runner never prunes
imports).`,
		Before: `if strconv.FormatInt(a, 16) == strconv.FormatInt(b, 16) {`,
		After:  `if a == b {`,
		MeasuredWin: `On two int64 values in base 16 (Apple M2 Pro, go1.26): ` +
			`strconv.FormatInt(a, 16) == strconv.FormatInt(b, 16) ~33 ns/op, 16 B/op, 2 allocs/op ` +
			`vs a == b ~0.3 ns/op, 0 B/op, 0 allocs/op (~110x, both string allocations and both ` +
			`formatting passes replaced by one integer compare).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5051",
		Doc:  "strconv.FormatInt/FormatUint(a, B) == / != of the same base instead of a == / != b",
		Run:  runPS5051,
	},
})

func runPS5051(pass *analysis.Pass) (any, error) {
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
			left, lname, lbase, lok := ps5051FormatCall(pass, bin.X)
			right, rname, rbase, rok := ps5051FormatCall(pass, bin.Y)
			// Same function (so the unwrapped int64/uint64 operands compare) and
			// the same in-range base (so the strings are jointly injective).
			if !lok || !rok || lname != rname || lbase != rbase {
				return true
			}
			var fix *analysis.SuggestedFix
			if !ps2109CommentBetween(f, left.Pos(), left.Args[0].Pos()) &&
				!ps2109CommentBetween(f, left.Args[0].End(), left.End()) &&
				!ps2109CommentBetween(f, right.Pos(), right.Args[0].Pos()) &&
				!ps2109CommentBetween(f, right.Args[0].End(), right.End()) {
				fix = &analysis.SuggestedFix{
					Message: "compare the integers directly",
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
		// Each fixable comparison removes TWO strconv references; withhold all
		// fixes if that would orphan the import.
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "strconv") > 2*fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.bin.Pos(),
				End:     st.bin.End(),
				Message: "strconv.FormatInt/FormatUint(a, B) " + st.bin.Op.String() + " ...(b, B) formats two integers to throwaway strings just to compare them; a " + st.bin.Op.String() + " b compares the integers directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5051FormatCall reports whether e is a call of the package-level
// strconv.FormatInt or strconv.FormatUint whose base argument is a constant in
// [2, 36], returning the call, the function name, and the base value.
func ps5051FormatCall(pass *analysis.Pass, e ast.Expr) (call *ast.CallExpr, name string, base int64, ok bool) {
	c, isCall := ps2109Unparen(e).(*ast.CallExpr)
	if !isCall || len(c.Args) != 2 || c.Ellipsis.IsValid() {
		return nil, "", 0, false
	}
	fname, matched := astutil.PkgFuncCall(pass.TypesInfo, c.Fun, "strconv", map[string]bool{"FormatInt": true, "FormatUint": true})
	if !matched {
		return nil, "", 0, false
	}
	tv, found := pass.TypesInfo.Types[c.Args[1]]
	if !found || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return nil, "", 0, false
	}
	b, exact := constant.Int64Val(tv.Value)
	if !exact || b < 2 || b > 36 {
		return nil, "", 0, false
	}
	return c, fname, b, true
}
