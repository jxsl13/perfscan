package checks

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2136 reports []byte(strconv.Itoa(...)) and its Format*/Quote* siblings
// — converting a freshly formatted string to bytes — when the matching
// strconv.Append* function writes the bytes into a slice directly. The
// strconv analog of PS2109.
var PS2136 = register(&lint.Check{
	ID:       "PS2136",
	Category: "alloc",
	Slug:     "strconv-append-bytes",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "[]byte(strconv.Itoa(...)) allocates a string then copies it; strconv.AppendInt writes the bytes directly",
		Text: `strconv.Itoa and every strconv.Format*/Quote* function build their
result in a byte buffer and convert it to a string (one allocation +
copy); the surrounding []byte conversion immediately copies that string
back into a fresh byte slice (a second allocation + copy). Each of these
formatters has an Append* twin — in the standard library the string form
IS string(Append*(nil, ...)) — that appends the identical bytes straight
into a destination slice: same bytes, half the allocations, half the
copying.

  []byte(strconv.Itoa(n))              -> strconv.AppendInt(nil, int64(n), 10)
  []byte(strconv.FormatInt(x, b))      -> strconv.AppendInt(nil, x, b)
  []byte(strconv.FormatUint(x, b))     -> strconv.AppendUint(nil, x, b)
  []byte(strconv.FormatFloat(f, ...))  -> strconv.AppendFloat(nil, f, ...)
  []byte(strconv.FormatBool(v))        -> strconv.AppendBool(nil, v)
  []byte(strconv.Quote(s))             -> strconv.AppendQuote(nil, s)       (and the
                                          QuoteToASCII/QuoteToGraphic/QuoteRune*
                                          variants likewise)

Unlike PS2109's fmt arm there is NO empty-output corner to guard: every
matched formatter renders at least one byte (a digit, "true"/"false", or
the opening quote), so strconv.Append*(nil, ...) can never return the nil
slice that would differ observably from []byte("")'s non-nil empty slice.
The rewrite is bit-identical for every input when the formatter accepts
its arguments. Since Go 1.27, FormatFloat and AppendFloat intentionally use
different panic text for an invalid bitSize. FormatFloat is therefore fixed
only when its bitSize is a compile-time 32 or 64; a dynamic or invalid
bitSize remains an advisory finding so recover-observable behavior is kept.

The fix edits around the call: every original argument is preserved
verbatim, in place, with unchanged evaluation order. Only the selected
name changes (FormatInt -> AppendInt) and nil is prepended as the
destination, so an aliased strconv import keeps its qualifier and the
import can never be orphaned (Append* lives in strconv too). Itoa is the
one special case: it has no direct Append twin, so the argument is
wrapped in a value-preserving int64(...) conversion and the explicit
base 10 is appended — strconv.Itoa is defined as FormatInt(int64(i), 10).

Only the exact predeclared []byte(...) conversion of a call type-pinned
to the standard library's strconv is matched. A NAMED byte-slice
conversion (type Bytes []byte; Bytes(strconv.Itoa(n))) is out of scope:
strconv.Append* returns []byte, not the named type, so the rewrite would
not type-check. A comment inside the deleted conversion shell suppresses
the fix and keeps the advisory report.`,
		Before: `payload := []byte(strconv.Itoa(id))`,
		After:  `payload := strconv.AppendInt(nil, int64(id), 10)`,
		MeasuredWin: `BenchmarkPS2136 (6-digit int, Apple M2 Pro): 26.1 ns/op,
16 B/op, 2 allocs/op -> 19.2 ns/op, 8 B/op, 1 alloc/op (~1.4x) — the
intermediate string allocation and its copy removed; the gap widens with
output length since the saved copy is O(len).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2136",
		Doc:  "[]byte(strconv.Itoa/Format*/Quote*(...)) instead of strconv.Append*(nil, ...)",
		Run:  runPS2136,
	},
})

// ps2136Verb maps a matched strconv formatter to its Append* twin and its
// (fixed, non-variadic) parameter count. Itoa is special-cased in the fix
// construction: AppendInt needs the int64(...) wrap and an explicit base 10.
type ps2136Verb struct {
	appendName string
	argc       int
}

var ps2136Append = map[string]ps2136Verb{
	"Itoa":               {"AppendInt", 1},
	"FormatInt":          {"AppendInt", 2},
	"FormatUint":         {"AppendUint", 2},
	"FormatFloat":        {"AppendFloat", 4},
	"FormatBool":         {"AppendBool", 1},
	"Quote":              {"AppendQuote", 1},
	"QuoteToASCII":       {"AppendQuoteToASCII", 1},
	"QuoteToGraphic":     {"AppendQuoteToGraphic", 1},
	"QuoteRune":          {"AppendQuoteRune", 1},
	"QuoteRuneToASCII":   {"AppendQuoteRuneToASCII", 1},
	"QuoteRuneToGraphic": {"AppendQuoteRuneToGraphic", 1},
}

func runPS2136(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			outer, ok := n.(*ast.CallExpr)
			if !ok || len(outer.Args) != 1 || outer.Ellipsis.IsValid() {
				return true
			}
			if !ps2136IsByteSliceConv(pass, outer) {
				return true
			}
			call, ok := ps2109Unparen(outer.Args[0]).(*ast.CallExpr)
			if !ok || call.Ellipsis.IsValid() {
				return true
			}
			name, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "strconv", nil)
			if !ok {
				return true
			}
			verb, known := ps2136Append[name]
			if !known || len(call.Args) != verb.argc {
				return true
			}
			diag := analysis.Diagnostic{
				Pos: outer.Pos(),
				End: outer.End(),
				Message: "[]byte(strconv." + name + "(...)) allocates a string then copies it; strconv." +
					verb.appendName + "(nil, ...) writes the bytes directly",
			}
			// Every matched formatter renders at least one byte, so there is
			// no nil-vs-empty guard (contrast PS2109). Only the deleted
			// conversion shell must not swallow a comment. Since Go 1.27,
			// FormatFloat and AppendFloat use different recover-observable
			// panic strings for invalid bitSize values, so only a proven-valid
			// constant bitSize can be fixed.
			if !ps2136SafeFloatBitSize(pass, name, call.Args) {
				diag.Message += "; bitSize is not a compile-time 32 or 64, and FormatFloat/AppendFloat have different panic values for invalid bitSize — kept advisory to preserve recover-observable behavior"
			} else if !ps2109CommentBetween(f, outer.Pos(), call.Pos()) &&
				!ps2109CommentBetween(f, call.End(), outer.End()) {
				// PkgFuncCall matched, so call.Fun is a SelectorExpr whose
				// qualifier resolves to strconv; the rename keeps that
				// (possibly aliased) qualifier from the source untouched.
				sel := call.Fun.(*ast.SelectorExpr)
				edits := []analysis.TextEdit{
					// Drop "[]byte(" (and any wrapping parens) ...
					{Pos: outer.Pos(), End: call.Pos()},
					// ... rename FormatX -> AppendX, keeping the (possibly
					// aliased) qualifier and every argument untouched ...
					{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte(verb.appendName)},
				}
				if name == "Itoa" {
					// Itoa(n) -> AppendInt(nil, int64(n), 10): wrap the
					// original argument text in int64(...) and add the base.
					edits = append(edits,
						analysis.TextEdit{Pos: call.Lparen + 1, End: call.Lparen + 1, NewText: []byte("nil, int64(")},
						analysis.TextEdit{Pos: call.Rparen, End: call.Rparen, NewText: []byte("), 10")},
					)
				} else {
					// ... prepend the nil destination slice ...
					edits = append(edits,
						analysis.TextEdit{Pos: call.Lparen + 1, End: call.Lparen + 1, NewText: []byte("nil, ")},
					)
				}
				// ... and drop the conversion's closing ")".
				edits = append(edits, analysis.TextEdit{Pos: call.End(), End: outer.End()})
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace with strconv." + verb.appendName + "(nil, ...)",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

func ps2136SafeFloatBitSize(pass *analysis.Pass, name string, args []ast.Expr) bool {
	if name != "FormatFloat" {
		return true
	}
	tv, ok := pass.TypesInfo.Types[ps2109Unparen(args[3])]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return false
	}
	bitSize, exact := constant.Int64Val(tv.Value)
	return exact && (bitSize == 32 || bitSize == 64)
}

// ps2136IsByteSliceConv reports whether call is a conversion to an (alias
// of an) UNNAMED slice of the predeclared byte. Type information
// distinguishes a conversion from a function call; a named byte-slice
// target is rejected because strconv.Append* returns the plain []byte, so
// the rewrite would change the expression's type and not type-check.
func ps2136IsByteSliceConv(pass *analysis.Pass, call *ast.CallExpr) bool {
	tv, ok := pass.TypesInfo.Types[call.Fun]
	if !ok || !tv.IsType() {
		return false
	}
	sl, ok := types.Unalias(tv.Type).(*types.Slice)
	if !ok {
		return false
	}
	eb, ok := types.Unalias(sl.Elem()).(*types.Basic)
	return ok && eb.Kind() == types.Byte
}
