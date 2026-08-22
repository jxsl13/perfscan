package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"math"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5071 reports strconv.Itoa(x) == "123" (and !=) — formatting an int to a
// decimal string only to compare it against a compile-time string constant —
// where x == 123 compares the int directly, with no allocation and no
// formatting. The string-constant sibling of PS5048 (strconv.Itoa(a) ==
// strconv.Itoa(b)).
var PS5071 = register(&lint.Check{
	ID:       "PS5071",
	Category: "alloc",
	Slug:     "itoa-const-compare-to-int-compare",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `strconv.Itoa(x) == "123" formats an int to a throwaway string just to compare it to a constant; x == 123 compares the int directly`,
		Text: `strconv.Itoa(x) == "123" converts x to its decimal string — a heap
allocation and a base-10 formatting pass — and then compares that string
against the constant. Because Itoa is injective and produces exactly one
canonical decimal spelling of each int, the string equals "123" for exactly
one int value, so the comparison is the integer test x == 123, which the
machine does in one instruction with no allocation and no formatting. The ==
and != forms both collapse.

The rewrite is BIT-IDENTICAL only when the constant is the CANONICAL decimal
spelling of an int — the exact text strconv.Itoa would emit. A non-canonical
constant is one Itoa can never produce ("0123" with a leading zero, "+5",
" 5" with spaces, "", "0x10", "1_000", a value that overflows int, "-0"): for
those strconv.Itoa(x) == c is ALWAYS false (and != always true) for every x,
which x == v would NOT reproduce, so they are deliberately left alone. The
value is additionally required to fit a signed 32-bit int, so the rewritten
constant never overflows int on a 32-bit target (int is at least 32 bits on
every Go platform); a larger canonical constant keeps the advisory report.

The rewrite is restricted to == and != — the ORDERING comparisons do not
carry over, because decimal strings compare lexicographically ("10" < "9")
while ints compare numerically. Only one operand is the Itoa call; the other
is a string constant (a string literal or a named string constant, matched by
constant value). Both operand orders are handled. Itoa's argument is a plain
int expression, and every operator that can appear in one binds tighter than
== / !=, so unwrapping it needs no parentheses; the constant becomes a bare
int literal (also a primary). The fix drops the Itoa(...) wrapper and rewrites
the constant to the integer literal, and is withheld (advisory only) when a
comment sits inside the removed wrapper, or when dropping the strconv
reference would orphan the strconv import (the runner never prunes imports).`,
		Before: `if strconv.Itoa(status) == "200" {`,
		After:  `if status == 200 {`,
		MeasuredWin: `On a 3-digit int (Apple M2 Pro, go1.26): strconv.Itoa(x) == "200" ` +
			`~14 ns/op, 3 B/op, 1 alloc/op vs x == 200 ~0.4 ns/op, 0 B/op, 0 allocs/op ` +
			`(~36x, the string allocation and the formatting pass replaced by one integer compare).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5071",
		Doc:  `strconv.Itoa(x) == / != a decimal string constant instead of x == / != the int`,
		Run:  runPS5071,
	},
})

func runPS5071(pass *analysis.Pass) (any, error) {
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
			// One side is strconv.Itoa(x), the other a canonical decimal
			// string constant. Try both orders.
			call, cst, intText, ok := ps5071Match(pass, bin.X, bin.Y)
			if !ok {
				call, cst, intText, ok = ps5071Match(pass, bin.Y, bin.X)
			}
			if !ok {
				return true
			}
			var fix *analysis.SuggestedFix
			// Drop the Itoa(...) wrapper and rewrite the constant to the int
			// literal; a comment inside the removed wrapper withholds the fix.
			if !ps2109CommentBetween(f, call.Pos(), call.Args[0].Pos()) &&
				!ps2109CommentBetween(f, call.Args[0].End(), call.End()) {
				fix = &analysis.SuggestedFix{
					Message: "compare the int directly",
					TextEdits: []analysis.TextEdit{
						{Pos: call.Pos(), End: call.Args[0].Pos()},
						{Pos: call.Args[0].End(), End: call.End()},
						{Pos: cst.Pos(), End: cst.End(), NewText: []byte(intText)},
					},
				}
				fixable++
			}
			sites = append(sites, site{bin, fix})
			return true
		})
		// Each fixable comparison removes ONE strconv reference (the Itoa
		// selector); withhold all fixes if that would orphan the import.
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "strconv") > fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.bin.Pos(),
				End:     st.bin.End(),
				Message: "strconv.Itoa(x) " + st.bin.Op.String() + ` a decimal string constant formats the int to a throwaway string just to compare it; x ` + st.bin.Op.String() + " the int constant compares directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5071Match tries side as strconv.Itoa(x) and far as a canonical decimal
// string constant. It returns the Itoa call, the constant operand node, and
// the integer-literal text to substitute for the constant.
func ps5071Match(pass *analysis.Pass, side, far ast.Expr) (call *ast.CallExpr, cst ast.Expr, intText string, ok bool) {
	c, isItoa := ps5048ItoaCall(pass, side)
	if !isItoa {
		return nil, nil, "", false
	}
	tv, found := pass.TypesInfo.Types[far]
	if !found || tv.Value == nil || tv.Value.Kind() != constant.String {
		return nil, nil, "", false
	}
	s := constant.StringVal(tv.Value)
	text, canon := ps5071CanonicalInt32(s)
	if !canon {
		return nil, nil, "", false
	}
	return c, far, text, true
}

// ps5071CanonicalInt32 reports whether s is the canonical decimal spelling of
// an int (exactly what strconv.Itoa would emit) whose value fits a signed
// 32-bit int, and returns that spelling. The int32 bound keeps the rewritten
// literal from overflowing int on a 32-bit target. Non-canonical spellings
// (leading zeros, a sign on zero, spaces, base prefixes, underscores,
// overflow) are rejected — strconv.Itoa can never produce them, so the
// comparison is a constant false/true that x == v would not reproduce.
func ps5071CanonicalInt32(s string) (string, bool) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return "", false
	}
	if v < math.MinInt32 || v > math.MaxInt32 {
		return "", false
	}
	// Canonical iff re-formatting the parsed value reproduces s exactly
	// (rejects "0123", "+5", "-0", " 5", ...).
	if strconv.Itoa(v) != s {
		return "", false
	}
	return s, true
}
