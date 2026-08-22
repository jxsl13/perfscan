package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2036 reports fmt.Append(buf, x) with a SINGLE operand of an UNNAMED
// predeclared integer, bool or float type — where strconv.AppendInt/
// AppendUint/AppendBool/AppendFloat writes the identical bytes straight
// into buf without fmt's interface boxing and reflection printer. The
// no-format fmt.Append twin of PS2137 (fmt.Sprint(x) -> strconv) and the
// scalar sibling of PS5033 (fmt.Append single string -> append).
var PS2036 = register(&lint.Check{
	ID:       "PS2036",
	Category: "alloc",
	Slug:     "append-single-scalar",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Append of a single plain integer, bool or float has a direct strconv.Append* equivalent",
		Text: `fmt.Append(b, a...) appends fmt.Sprint(a...): it boxes every operand
into an interface (a heap allocation), acquires a pp printer from fmt's
sync.Pool, formats each operand as %v into the pooled buffer, and copies
that buffer onto b. When the call has exactly ONE operand of a plain
integer, bool or float type, all of that machinery reproduces what a
single strconv.Append* call writes into buf's existing capacity directly,
with no boxing and no reflection:

  x is int/int8/int16/int32/int64      -> strconv.AppendInt(buf, int64(x), 10)
  x is uint/uint8/.../uint64/uintptr   -> strconv.AppendUint(buf, uint64(x), 10)
  x is bool                            -> strconv.AppendBool(buf, x)
  x is float64                         -> strconv.AppendFloat(buf, x, 'g', -1, 64)
  x is float32                         -> strconv.AppendFloat(buf, float64(x), 'g', -1, 32)

(the int64/uint64 widening wrapper is dropped when the operand already has
that exact width). The rewrite is restricted to operands whose static type
is an UNNAMED predeclared type of one of these kinds — the safety guard
that makes it bit-identical, the same one PS2137 established for
fmt.Sprint: %v (and therefore Append/Sprint) HONORS fmt.Stringer and
fmt.Formatter, so a NAMED type with a String() method prints via
String(), not as the plain value. An unnamed predeclared type cannot
carry methods, so it cannot be a Stringer or Formatter, and %v of a plain
scalar prints exactly the strconv form: for integers the decimal digits
with a leading '-' for negatives, MinInt64 and MaxUint64 included
(AppendInt works in uint64 magnitude space); for bool the literal
true/false; for floats the %v shortest-'g' form — AppendFloat with
precision -1 and the operand's own bit size (the float32 operand is
widened value-preservingly to AppendFloat's float64 parameter while
bitSize 32 keeps the float32 rounding), matching fmt for NaN, the Infs,
-0 and the fixed/exponent switchover bit for bit. With one operand
Sprint's between-operand spacing rule never applies. Both forms grow buf
with the same builtin append, evaluate buf and x exactly once in the
original order, and take and return the unnamed []byte — so a named
[]byte destination (or a nil literal) type-checks identically on both
sides and needs no extra guard. types.Default materializes an untyped
constant as what Append's ...any parameter sees (an untyped int becomes
int, an untyped rune becomes rune/int32, untyped bool becomes bool,
untyped float becomes float64); a named type survives Default as
*types.Named, never *types.Basic, and stays SILENT — not even advisory,
because the strconv suggestion would be wrong for a Stringer. Complex
kinds are out of scope (no single strconv call for fmt's (re+imi) form),
and the single-STRING operand is PS5033's (append(buf, s...)); a []byte
operand under %v prints the decimal slice representation "[104 105]" —
not this pattern at all. Two or more operands engage Sprint's spacing
rule and a spread (fmt.Append(buf, xs...)) passes an unknown number of
operands; both are excluded.

The fix keeps buf and the operand byte-verbatim in place and edits only
the scaffolding: fmt.Append becomes strconv.Append* (reusing an existing
strconv import's alias) and the widening wrapper plus the base or
'g', -1, bitSize arguments are spliced around the operand. The strconv
import is added when the file lacks it; the fix is withheld (advisory
report) when strconv's name is shadowed at the call site, when a cgo
file would need its import block edited, or when a comment sits inside
the rewritten scaffolding. Each rewrite removes the file's fmt.Append
selector, so — like PS2137/PS5015 — when applying all fixes would
rewrite the file's last fmt reference the fix pipeline prunes the
orphaned import afterwards, except in cgo files, where the reports stay
advisory.`,
		Before: `buf = fmt.Append(buf, n)`,
		After:  `buf = strconv.AppendInt(buf, int64(n), 10)`,
		MeasuredWin: `BenchmarkPS2036 (6-digit int appended to a preallocated buffer,
Apple M2 Pro, go1.26): fmt.Append(buf, n) ~45 ns/op, 8 B/op, 1 alloc/op
vs strconv.AppendInt(buf, int64(n), 10) ~8.7 ns/op, 0 B/op, 0 allocs/op
(~5x, and the interface-boxing allocation disappears); the float64 pair
(BenchmarkPS2036Float): ~76 ns/op, 8 B/op, 1 alloc/op vs ~36 ns/op,
0 B/op, 0 allocs/op (~2x). Both sides write into the buffer's existing
capacity — the entire gap is fmt's boxing, pooled printer and reflection
walk.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2036",
		Doc:  "fmt.Append of a single plain integer, bool or float instead of the direct strconv.Append* call",
		Run:  runPS2036,
	},
})

func runPS2036(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last fmt
		// reference and orphan the import. The fix pipeline prunes such an
		// orphan afterwards — except in a cgo file (import "C"), whose
		// import block is never edited, so there the fixes are withheld and
		// the reports stay advisory (same policy as PS2107/PS2137/PS5015).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		// All fixes of a run are applied together, so only the first fix
		// needing the strconv import carries the import edit (same
		// convention as PS2107).
		importAdded := false
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			// Type info pins the callee to the package-level fmt.Append; a
			// shadowed fmt or a method named Append never matches.
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Append": true}); !ok {
				return true
			}
			// THE safety guard (PS2137's): the operand's static type must be
			// an UNNAMED predeclared integer, bool or float. %v honors
			// fmt.Stringer and fmt.Formatter, so a NAMED type with a String()
			// method prints via String(), not the plain value — named types
			// stay SILENT (the strconv message would be wrong for them).
			// types.Default materializes an untyped constant as what Append's
			// ...any sees; complex kinds (IsComplex, disjoint from IsFloat),
			// strings (PS5033's), []byte and nil all fail this guard.
			basic, isBasic := types.Default(pass.TypesInfo.TypeOf(call.Args[1])).(*types.Basic)
			if !isBasic || basic.Info()&(types.IsInteger|types.IsBoolean|types.IsFloat) == 0 {
				return true
			}
			appendName, prefix, suffix := ps2036Repl(basic)
			// PkgFuncCall matched, so call.Fun is a SelectorExpr.
			sel := call.Fun.(*ast.SelectorExpr)
			var fix *analysis.SuggestedFix
			// The edited scaffolding spans must not swallow a comment: the
			// fmt.Append selector, the span between buf and x (the comma is
			// replaced by the widening wrapper), and the closing span after x
			// (where the base / 'g', -1, bitSize arguments land).
			if !ps2111CommentIn(f, sel.Pos(), sel.End()) &&
				!ps2111CommentIn(f, call.Args[0].End(), call.Args[1].Pos()) &&
				!ps2111CommentIn(f, call.Args[1].End(), call.End()) {
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "strconv", "strconv")
				if usable && !(needImport && ps2107ImportsC(f)) {
					edits := []analysis.TextEdit{
						{Pos: sel.Pos(), End: sel.End(), NewText: []byte(useName + "." + appendName)},
						{Pos: call.Args[0].End(), End: call.Args[1].Pos(), NewText: []byte(prefix)},
						{Pos: call.Args[1].End(), End: call.End(), NewText: []byte(suffix)},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "strconv"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "replace fmt.Append with strconv." + appendName,
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, site{call, fix})
			return true
		})
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "fmt") > fixable || !ps2110ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "fmt.Append with a single int/uint/bool/float operand boxes it into an interface and walks fmt's printer through a pooled buffer; strconv.Append* writes the identical bytes into buf directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2036Repl picks the strconv.Append* replacement for an operand of an
// unnamed predeclared integer, bool or float type: the callee name plus the
// scaffolding spliced around the untouched buf and operand (prefix replaces
// the `, ` between them, suffix replaces the closing `)` span). The
// int64/uint64 widenings are value-preserving for every narrower width
// (uintptr included) and dropped when the operand already has the exact
// width; the float forms are the shortest-'g' AppendFloat spellings that
// reproduce %v bit for bit — precision -1 with the operand's own bit size,
// the float32 operand widened value-preservingly to AppendFloat's float64
// parameter while bitSize 32 keeps the float32 rounding.
func ps2036Repl(basic *types.Basic) (appendName, prefix, suffix string) {
	switch {
	case basic.Info()&types.IsBoolean != 0:
		return "AppendBool", ", ", ")"
	case basic.Kind() == types.Float64:
		return "AppendFloat", ", ", ", 'g', -1, 64)"
	case basic.Kind() == types.Float32:
		return "AppendFloat", ", float64(", "), 'g', -1, 32)"
	case basic.Info()&types.IsUnsigned != 0:
		if basic.Kind() == types.Uint64 {
			return "AppendUint", ", ", ", 10)"
		}
		return "AppendUint", ", uint64(", "), 10)"
	default:
		// The caller's guard admits only integer, bool and float kinds, so
		// what remains here is exactly the signed-integer arm.
		if basic.Kind() == types.Int64 {
			return "AppendInt", ", ", ", 10)"
		}
		return "AppendInt", ", int64(", "), 10)"
	}
}
