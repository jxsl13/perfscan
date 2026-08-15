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

// PS5015 reports fmt.Appendf(buf, verb, x) where the format is exactly one
// bare scalar verb (%d, %t, %g, %b, %o, %x over an integer, or %q over a
// string/rune) with a direct strconv.Append* equivalent that writes the
// identical bytes straight into buf. The Appendf-destination twin of
// PS2107 (Sprintf single verb -> strconv) and the scalar sibling of
// PS2141 (Appendf lone %s -> append).
var PS5015 = register(&lint.Check{
	ID:       "PS5015",
	Category: "alloc",
	Slug:     "appendf-single-scalar-verb",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Appendf of a single scalar verb runs fmt's formatter; strconv.Append* writes the bytes directly",
		Text: `fmt.Appendf parses its format string, boxes the argument into an
interface (a heap allocation) and drives fmt's formatter state machine
through a pooled pp buffer — even when the format is a single bare verb
over one scalar. Every such shape has a strconv.Append* twin that formats
the value straight into buf's existing capacity, with no format parse and
no boxing:

  fmt.Appendf(buf, "%d", i)  -> strconv.AppendInt(buf, int64(i), 10)   (signed widths)
                             -> strconv.AppendUint(buf, uint64(u), 10) (unsigned widths)
  fmt.Appendf(buf, "%t", b)  -> strconv.AppendBool(buf, b)
  fmt.Appendf(buf, "%g", f)  -> strconv.AppendFloat(buf, f, 'g', -1, 64)          (float64)
                             -> strconv.AppendFloat(buf, float64(f), 'g', -1, 32) (float32)
  fmt.Appendf(buf, "%b", i)  -> strconv.AppendInt/AppendUint(buf, ..., 2)
  fmt.Appendf(buf, "%o", i)  -> strconv.AppendInt/AppendUint(buf, ..., 8)
  fmt.Appendf(buf, "%x", i)  -> strconv.AppendInt/AppendUint(buf, ..., 16) (i an integer)
  fmt.Appendf(buf, "%q", s)  -> strconv.AppendQuote(buf, s)     (s a string)
  fmt.Appendf(buf, "%q", r)  -> strconv.AppendQuoteRune(buf, r) (r a rune)

The match is deliberately narrow — it is the whole safety story, the same
guards as PS2107 plus PS2141's destination guard. The callee is pinned by
type information to the package-level fmt.Appendf (a shadowed fmt or a
method named Appendf never matches), the call must not spread its
arguments, and the format must be a string literal that is EXACTLY one of
the verbs above: any literal text, flag, width or precision disqualifies
it (%#o, %+d, %2x and %e/%f — whose default precision AppendFloat(-1)
does not reproduce — are all out). The operand must be an UNNAMED
predeclared type of the verb's kind: unnamed predeclared types cannot
carry a String() or Format() method, so the value cannot dispatch through
fmt.Stringer/fmt.Formatter — closing the only divergence path. A NAMED
operand type keeps the advisory report. The destination buf must be an
unnamed []byte (PS2141's guard); a named []byte destination or a nil
literal keeps the advisory report too.

Within that shape the rewrite is bit-identical for every input, the
identities PS2107 established: %d/%b/%o/%x print exactly the base-10/2/8/16
digits AppendInt/AppendUint produce (leading '-' for negatives included,
MinInt64/MaxUint64 verified), %t is "true"/"false" on both sides, %g maps
onto AppendFloat's shortest 'g' form with the operand's own bit size
(including -0, NaN and the Infs; the float64(f) widening of a float32 is
value-preserving and bitSize 32 keeps the float32 rounding), and fmt
implements a bare %q via exactly strconv's AppendQuote/AppendQuoteRune
(invalid UTF-8, surrogate and out-of-range runes included). Every matched
verb emits at least one byte, so — as PS2136 observed — there is no
nil-vs-empty corner: both sides of Appendf(nil, ...) return a non-nil
slice. buf and x are each evaluated exactly once, in the original order,
and the int64/uint64/float64 widening conversions are value-preserving.
%s belongs to PS2141 and %x over a []byte (hex bytes, not an integer) is
out of scope here.

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
		Before: `buf = fmt.Appendf(buf, "%d", id)`,
		After:  `buf = strconv.AppendInt(buf, int64(id), 10)`,
		MeasuredWin: `BenchmarkPS5015 (6-digit int appended to a preallocated
buffer, Apple M2 Pro, go1.26): fmt.Appendf(buf, "%d", i) ~45 ns/op,
8 B/op, 1 alloc/op vs strconv.AppendInt(buf, int64(i), 10) ~8.2 ns/op,
0 B/op, 0 allocs/op (~5.5x, and the interface-boxing allocation
disappears); the %g pair (BenchmarkPS5015Float): ~95 ns/op, 8 B/op,
1 alloc/op vs ~48 ns/op, 0 B/op, 0 allocs/op (~2x). Both sides write
into the buffer's existing capacity — the entire gap is fmt's format
parse, boxing and formatter state machine.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5015",
		Doc:  "fmt.Appendf of a single bare scalar verb instead of the direct strconv.Append* call",
		Run:  runPS5015,
	},
})

func runPS5015(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last fmt
		// reference and orphan the import. The fix pipeline prunes such an
		// orphan afterwards — except in a cgo file (import "C"), whose
		// import block is never edited, so there the fixes are withheld and
		// the reports stay advisory (same policy as PS2107/PS2137).
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
			verb, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			c := ps5015Classify(pass, verb, call.Args[2])
			if c == nil {
				return true
			}
			var fix *analysis.SuggestedFix
			if c.appendName != "" &&
				// PS2141's destination guard: an unnamed []byte, so the
				// rewrite's []byte result matches Appendf's exactly. (An
				// untyped nil destination fails this too and stays advisory.)
				ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(call.Args[0])) &&
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
						Message:   "replace fmt.Appendf(buf, " + lit.Value + ", ...) with strconv." + c.appendName,
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

// ps5015Case is one recognized single-verb Appendf shape: the diagnostic
// message and, when the rewrite is provably bit-identical, the
// strconv.Append* callee plus the scaffolding text spliced around the
// untouched buf and value arguments (prefix replaces `, "<verb>", `;
// suffix replaces the closing `)` span).
type ps5015Case struct {
	msg        string
	appendName string // "" keeps the report advisory
	prefix     string
	suffix     string
}

// ps5015Classify matches verb+operand against the recognized shapes. It
// returns nil when the call is not one of them (wrong verb, wrong operand
// type). A shape match with a NAMED operand type yields an advisory-only
// case (appendName == ""): a named type may implement fmt.Stringer or
// fmt.Formatter, which fmt would honor and strconv would not. Unnamed
// predeclared types cannot carry methods, so for them the rewrite is
// bit-identical (see the Doc for the per-verb identities).
func ps5015Classify(pass *analysis.Pass, verb string, arg ast.Expr) *ps5015Case {
	t := pass.TypesInfo.TypeOf(arg)
	if t == nil {
		return nil
	}
	// An untyped constant materializes as its default type when passed to
	// Appendf's ...any parameter.
	t = types.Default(t)
	basic, tIsBasic := t.(*types.Basic)
	under, underBasic := t.Underlying().(*types.Basic)
	const boxes = " boxes the argument and walks fmt's formatter state machine; "
	switch verb {
	case "%d", "%b", "%o", "%x":
		// Integer bases: %d/%b/%o/%x print exactly the base-10/2/8/16
		// digits AppendInt/AppendUint produce, leading '-' included. Only
		// the bare lowercase verb reaches here (%X, %#x, %+d never match),
		// and only integers do — %b over a float (its binary-exponent form)
		// and %x over a []byte/string (hex bytes) or float (hex float) are
		// different formatting entirely.
		if !underBasic || under.Info()&types.IsInteger == 0 {
			return nil
		}
		base, baseName := "10", "base 10"
		switch verb {
		case "%b":
			base, baseName = "2", "base 2"
		case "%o":
			base, baseName = "8", "base 8"
		case "%x":
			base, baseName = "16", "base 16 (hexadecimal)"
		}
		c := &ps5015Case{msg: "fmt.Appendf of a single " + verb + " integer value" + boxes + "strconv.AppendInt/AppendUint with " + baseName + " appends the digits directly"}
		if !tIsBasic {
			return c
		}
		if basic.Info()&types.IsUnsigned != 0 {
			c.appendName = "AppendUint"
			if basic.Kind() == types.Uint64 {
				c.prefix, c.suffix = ", ", ", "+base+")"
			} else {
				// The uint64(u) widening is value-preserving for every
				// narrower unsigned width (uintptr included).
				c.prefix, c.suffix = ", uint64(", "), "+base+")"
			}
		} else {
			c.appendName = "AppendInt"
			if basic.Kind() == types.Int64 {
				c.prefix, c.suffix = ", ", ", "+base+")"
			} else {
				// The int64(i) widening is value-preserving for every
				// narrower signed width.
				c.prefix, c.suffix = ", int64(", "), "+base+")"
			}
		}
		return c
	case "%t":
		if !underBasic || under.Info()&types.IsBoolean == 0 {
			return nil
		}
		c := &ps5015Case{msg: "fmt.Appendf of a single %t value" + boxes + "strconv.AppendBool appends \"true\"/\"false\" directly"}
		if !tIsBasic {
			return c
		}
		c.appendName, c.prefix, c.suffix = "AppendBool", ", ", ")"
		return c
	case "%g":
		// Only %g maps onto AppendFloat's shortest form ('g' with
		// precision -1): %e/%f default to 6 digits, which AppendFloat(-1)
		// does not reproduce, so they are deliberately NOT recognized. The
		// bitSize matches the operand's float width so the shortest
		// representation rounds back to the ORIGINAL float32/float64 value
		// — exactly what %g prints (including -0, NaN and the Infs).
		if !underBasic || under.Info()&types.IsFloat == 0 {
			return nil
		}
		c := &ps5015Case{msg: "fmt.Appendf of a single %g float value" + boxes + "strconv.AppendFloat appends it directly"}
		if !tIsBasic {
			return c
		}
		switch basic.Kind() {
		case types.Float64:
			c.appendName, c.prefix, c.suffix = "AppendFloat", ", ", ", 'g', -1, 64)"
		case types.Float32:
			// AppendFloat takes a float64; the widening float64(f) is
			// value-preserving and bitSize 32 keeps the float32 rounding.
			c.appendName, c.prefix, c.suffix = "AppendFloat", ", float64(", "), 'g', -1, 32)"
		}
		return c
	case "%q":
		// %q over a string is strconv.AppendQuote; over a rune (int32) it
		// is strconv.AppendQuoteRune. Both are bit-identical to fmt's bare
		// %q (fmt implements it via exactly these; out-of-range and
		// surrogate runes collapse to U+FFFD on both sides). A []byte or
		// any wider integer under %q is out of scope — a non-int32 integer
		// above utf8.MaxRune renders U+FFFD in fmt but would need a range
		// guard here, so only int32/rune is provably identical.
		if !underBasic {
			return nil
		}
		isString := under.Info()&types.IsString != 0
		isRune := under.Kind() == types.Int32
		if !isString && !isRune {
			return nil
		}
		msg := "fmt.Appendf(buf, \"%q\", r) on a rune" + boxes + "strconv.AppendQuoteRune appends the quoted rune directly"
		if isString {
			msg = "fmt.Appendf(buf, \"%q\", s) on a string" + boxes + "strconv.AppendQuote appends the quoted string directly"
		}
		c := &ps5015Case{msg: msg}
		if !tIsBasic {
			return c
		}
		if isString {
			c.appendName = "AppendQuote"
		} else {
			c.appendName = "AppendQuoteRune"
		}
		c.prefix, c.suffix = ", ", ")"
		return c
	}
	return nil
}
