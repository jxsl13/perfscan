package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2035 reports fmt.Appendf(buf, "%v", x) where x is an UNNAMED predeclared
// integer, bool or float — the %v twin of PS5015's bare scalar verbs (whose
// classifier deliberately has no %v case) carried to the Appendf destination,
// and the Appendf twin of PS2137 (Sprint/Sprintf("%v") of a plain scalar).
var PS2035 = register(&lint.Check{
	ID:       "PS2035",
	Category: "alloc",
	Slug:     "appendf-v-scalar",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Appendf(buf, \"%v\", x) of a plain integer, bool or float has a direct strconv.Append* equivalent",
		Text: `fmt.Appendf parses its format string, boxes the argument into an
interface (a heap allocation) and drives fmt's formatter state machine
through a pooled pp buffer — even when the format is exactly "%v" over one
plain scalar. For an integer, bool or float operand %v prints the same
bytes strconv.Append* writes straight into buf's existing capacity, with
no format parse and no boxing:

  x is a signed integer    -> strconv.AppendInt(buf, int64(x), 10)  (bare x for int64)
  x is an unsigned integer -> strconv.AppendUint(buf, uint64(x), 10) (bare x for uint64;
                              uintptr included)
  x is bool                -> strconv.AppendBool(buf, x)
  x is float64             -> strconv.AppendFloat(buf, x, 'g', -1, 64)
  x is float32             -> strconv.AppendFloat(buf, float64(x), 'g', -1, 32)

The match is deliberately narrow — it is the whole safety story, PS2137's
guard combined with PS5015's destination guard. The callee is pinned by
type information to the package-level fmt.Appendf (a shadowed fmt or a
method named Appendf never matches), the call must not spread its
arguments, and the format must be a string literal that is EXACTLY "%v" —
any literal text, flag, width or another verb disqualifies it (%d/%t/%g
and friends are PS5015's, the lone %s is PS2141's). The operand's static
type must be an UNNAMED predeclared type of integer, bool or float kind
(types.Default materializes an untyped constant as exactly what Appendf's
...any parameter sees; complex is out of scope). That guard is what makes
the rewrite bit-identical: unlike the bare %d, %v HONORS fmt.Stringer and
fmt.Formatter — a NAMED type with a String() method prints via String(),
not as the plain value — so named types are skipped entirely, not even
reported (PS2137's policy). An unnamed predeclared type cannot carry
methods, so it cannot dispatch through Stringer/Formatter, and %v of a
plain scalar prints exactly the strconv form: for integers the %d decimal
(FormatInt works in uint64 magnitude space, so MinInt64 needs no special
case; MaxUint64 prints at full width), for bool the literal true/false,
and for floats the shortest-'g' form — AppendFloat with precision -1 and
the operand's own bit size (the float32 operand is widened
value-preservingly to AppendFloat's float64 parameter while bitSize 32
keeps the float32 rounding), matching %v for NaN, the Infs, -0 and
full-precision extremes bit for bit. Every matched operand emits at least
one byte, so — as PS2136 observed — there is no nil-vs-empty corner: both
sides of Appendf(nil, ...) return a non-nil slice. Both calls return an
unnamed []byte, and buf and x are each evaluated exactly once, in the
original order.

The destination buf must be an unnamed []byte (PS2141's guard); a named
[]byte destination or an untyped nil keeps the report advisory. A string
or []byte operand is out of scope entirely: the lone-%s Appendf belongs
to PS2141, and %v over a []byte prints the "[104 105]" element list, not
the bytes.

The fix edits around the arguments: buf and x stay byte-verbatim in
place, only the scaffolding changes — fmt.Appendf becomes
strconv.Append*, the format literal (with its commas) becomes the
widening wrapper, and the base or 'g', -1, bitSize arguments are
appended. The strconv import is added when the file lacks it (reusing an
existing import's alias); the fix is suppressed when the strconv name is
shadowed at the call site or a cgo file would need the import added (a
cgo file's import block is never edited). The fix also applies when the
rewrite removes the file's last fmt reference — the fix pipeline prunes
the orphaned fmt import — except in cgo files, where the reports stay
advisory. A comment inside the rewritten scaffolding suppresses the fix
and keeps the report.`,
		Before: `buf = fmt.Appendf(buf, "%v", n)`,
		After:  `buf = strconv.AppendInt(buf, int64(n), 10)`,
		MeasuredWin: `BenchmarkPS2035 (6-digit int appended to a preallocated
buffer, Apple M2 Pro, go1.26): fmt.Appendf(buf, "%v", n) 45.6 ns/op,
8 B/op, 1 alloc/op vs strconv.AppendInt(buf, int64(n), 10) 9.5 ns/op,
0 B/op, 0 allocs/op (~4.8x, and the interface-boxing allocation
disappears); the float pair (BenchmarkPS2035Float): 104 ns/op, 8 B/op,
1 alloc/op vs 53 ns/op, 0 B/op, 0 allocs/op (~2x). Both sides write
into the buffer's existing capacity — the entire gap is fmt's format
parse, boxing and formatter state machine.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2035",
		Doc:  "fmt.Appendf(buf, \"%v\", x) of a plain integer, bool or float instead of the direct strconv.Append* call",
		Run:  runPS2035,
	},
})

func runPS2035(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last fmt
		// reference and orphan the import. The fix pipeline prunes such an
		// orphan afterwards — except in a cgo file (import "C"), whose
		// import block is never edited, so there the fixes are withheld and
		// the reports stay advisory (same policy as PS2107/PS5015).
		type site struct {
			call *ast.CallExpr
			msg  string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		// All fixes of a run are applied together, so only the first fix
		// needing the strconv import carries the import edit (same
		// convention as PS2107).
		importAdded := false
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
			}
			// Type info pins the callee to the package-level fmt.Appendf; a
			// shadowed fmt or a method named Appendf never matches.
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Appendf": true}); !ok {
				return true
			}
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if v, err := strconv.Unquote(lit.Value); err != nil || v != "%v" {
				return true
			}
			c := ps2035Classify(pass, call.Args[2])
			if c == nil {
				return true
			}
			var fix *analysis.SuggestedFix
			// PS2141's destination guard: an unnamed []byte, so the
			// rewrite's []byte result matches Appendf's exactly. (An
			// untyped nil destination fails this too and stays advisory.)
			if ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(call.Args[0])) &&
				// The edited scaffolding spans must not swallow a comment.
				!ps2111CommentIn(f, call.Args[0].End(), call.Args[2].Pos()) &&
				!ps2111CommentIn(f, call.Args[2].End(), call.End()) {
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "strconv", "strconv")
				if usable && !(needImport && ps2107ImportsC(f)) {
					// PkgFuncCall matched, so call.Fun is a SelectorExpr.
					sel := call.Fun.(*ast.SelectorExpr)
					edits := []analysis.TextEdit{
						{Pos: sel.Pos(), End: sel.End(), NewText: []byte(useName + "." + c.appendName)},
						{Pos: call.Args[0].End(), End: call.Args[2].Pos(), NewText: []byte(c.prefix)},
						{Pos: call.Args[2].End(), End: call.End(), NewText: []byte(c.suffix)},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "strconv"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "replace fmt.Appendf(buf, \"%v\", ...) with strconv." + c.appendName,
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, site{call, c.msg, fix})
			return true
		})
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "fmt") > fixable || !ps2110ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: st.msg,
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2035Case is one recognized Appendf-"%v" operand shape: the diagnostic
// message, the strconv.Append* callee and the scaffolding text spliced around
// the untouched buf and value arguments (prefix replaces `, "%v", `; suffix
// replaces the closing `)` span).
type ps2035Case struct {
	msg        string
	appendName string
	prefix     string
	suffix     string
}

// ps2035Classify matches the operand against the recognized scalar kinds. It
// returns nil for everything else — including every NAMED type: %v honors
// fmt.Stringer and fmt.Formatter, so a named type with a String() method
// prints via String(), not as the plain value, and even an advisory report
// would be wrong (PS2137's skip-named policy, unlike PS5015's advisory,
// because the bare %d verb ignores Stringer while %v does not). An unnamed
// predeclared type cannot carry methods, so for it the strconv form is
// provably bit-identical. types.Default materializes an untyped constant as
// its default type — exactly what Appendf's ...any parameter sees; a named
// type survives Default as *types.Named, never *types.Basic, and is skipped.
// Complex kinds (IsComplex, disjoint from IsFloat), strings and []byte stay
// out (the lone %s belongs to PS2141; %v of a []byte prints the element
// list).
func ps2035Classify(pass *analysis.Pass, arg ast.Expr) *ps2035Case {
	t := pass.TypesInfo.TypeOf(arg)
	if t == nil {
		return nil
	}
	basic, ok := types.Default(t).(*types.Basic)
	if !ok {
		return nil
	}
	const boxes = " boxes the argument and walks fmt's formatter state machine; "
	switch {
	case basic.Info()&types.IsInteger != 0:
		// %v of an integer prints the %d decimal — exactly the base-10
		// digits AppendInt/AppendUint produce, leading '-' included
		// (MinInt64 needs no special case: FormatInt works in uint64
		// magnitude space).
		c := &ps2035Case{msg: "fmt.Appendf of a single %v integer value" + boxes + "strconv.AppendInt/AppendUint appends the decimal digits directly"}
		if basic.Info()&types.IsUnsigned != 0 {
			c.appendName = "AppendUint"
			if basic.Kind() == types.Uint64 {
				c.prefix, c.suffix = ", ", ", 10)"
			} else {
				// The uint64(u) widening is value-preserving for every
				// narrower unsigned width (uintptr included).
				c.prefix, c.suffix = ", uint64(", "), 10)"
			}
		} else {
			c.appendName = "AppendInt"
			if basic.Kind() == types.Int64 {
				c.prefix, c.suffix = ", ", ", 10)"
			} else {
				// The int64(i) widening is value-preserving for every
				// narrower signed width.
				c.prefix, c.suffix = ", int64(", "), 10)"
			}
		}
		return c
	case basic.Info()&types.IsBoolean != 0:
		return &ps2035Case{
			msg:        "fmt.Appendf of a single %v bool value" + boxes + "strconv.AppendBool appends \"true\"/\"false\" directly",
			appendName: "AppendBool",
			prefix:     ", ",
			suffix:     ")",
		}
	case basic.Info()&types.IsFloat != 0:
		// %v of a float prints the %g shortest form — AppendFloat with
		// precision -1 and the operand's own bit size (including -0, NaN
		// and the Infs).
		c := &ps2035Case{msg: "fmt.Appendf of a single %v float value" + boxes + "strconv.AppendFloat appends the shortest-'g' form directly"}
		switch basic.Kind() {
		case types.Float64:
			c.appendName, c.prefix, c.suffix = "AppendFloat", ", ", ", 'g', -1, 64)"
		case types.Float32:
			// AppendFloat takes a float64; the widening float64(f) is
			// value-preserving and bitSize 32 keeps the float32 rounding.
			c.appendName, c.prefix, c.suffix = "AppendFloat", ", float64(", "), 'g', -1, 32)"
		default:
			return nil
		}
		return c
	}
	return nil
}
