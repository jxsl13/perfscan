package checks

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2105 reports `for … := range []rune(s)` where s is a string: ranging
// a string already decodes runes, so the []rune conversion allocates and
// fills a slice only to be iterated once and thrown away.
//
// PS2105 is a perfscan-original check (the x1xx block per category is
// reserved for checks that did not originate in the upstream reference
// registry).
var PS2105 = register(&lint.Check{
	ID:       "PS2105",
	Category: "alloc",
	Slug:     "range-over-rune-conversion",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "ranging over []rune(s) allocates a rune slice; range the string directly",
		Text: `[]rune(s) decodes the whole string into a freshly allocated
rune slice. When that slice exists only to feed a range loop, the
allocation buys nothing: ranging a string directly decodes the SAME rune
sequence (including one U+FFFD per invalid UTF-8 byte, exactly as the
conversion produces) with no allocation at all.

The one observable difference is the range KEY: over []rune(s) it is the
rune index (0, 1, 2, …), over the string it is the BYTE OFFSET of each
rune. The automatic fix therefore only fires when the key is absent or
the blank identifier — then dropping the conversion is bit-identical: the
value variable holds the same runes in the same order over the same
number of iterations. A used key keeps the advisory report; rework the
key (or use it as a byte offset) to unlock the rewrite.

The value side needs no guard: both forms yield values of type rune, and
the discarded slice cannot alias anything.`,
		Before: `for _, r := range []rune(s) {
	process(r)
}`,
		After: `for _, r := range s {
	process(r)
}`,
		MeasuredWin: `Dropping the conversion removes one full []rune
allocation + decode pass per loop entry: BenchmarkPS2105 (128 passes over
a ~127-char line per op) measures ~26µs, 61440 B and 128 allocs/op before
vs ~6.3µs and 0 allocs/op after — ~4x faster, allocation-free. Strings
short enough for the compiler's 32-rune stack buffer still pay the decode
copy, just not the heap.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2105",
		Doc:  "range over []rune(s) allocates; ranging the string yields the same runes",
		Run:  runPS2105,
	},
})

func runPS2105(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			rng, ok := n.(*ast.RangeStmt)
			if !ok {
				return true
			}
			call, ok := rng.X.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			if !ps2105IsRuneConvOfString(pass, call) {
				return true
			}
			callText := ps2105ExprText(call)
			argText := ps2105ExprText(call.Args[0])
			diag := analysis.Diagnostic{
				Pos: call.Pos(),
				End: call.End(),
			}
			// CRITICAL GUARD — key semantics differ between the two forms:
			// over []rune(s) the key is the rune index, over the string it
			// is the byte offset. Only an absent or blank key makes the
			// rewrite bit-identical. The rewritten range operand must also
			// be re-printable without a bare composite literal, which a
			// range header cannot carry unparenthesized.
			if ps2105KeyIsBlank(rng.Key) && !ps2105ContainsCompositeLit(call.Args[0]) {
				diag.Message = fmt.Sprintf("ranging over %s allocates a rune slice per loop entry; range %s directly — a string range yields the same runes", callText, argText)
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: fmt.Sprintf("range %s directly", argText),
					TextEdits: []analysis.TextEdit{
						{Pos: call.Pos(), End: call.End(), NewText: []byte(argText)},
					},
				}}
			} else {
				diag.Message = fmt.Sprintf("ranging over %s allocates a rune slice per loop entry; a direct range over %s yields the same runes, but its key is the byte offset, not the rune index — the key is used here, so the rewrite is manual", callText, argText)
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2105IsRuneConvOfString reports whether call is a conversion of a
// string-typed operand to a type whose underlying type is []rune (rune and
// int32 are the identical type, so both spellings qualify). Both the
// conversion and a direct string range then decode the identical rune
// sequence — invalid UTF-8 yields one U+FFFD per invalid byte in either
// form — over the identical iteration count.
func ps2105IsRuneConvOfString(pass *analysis.Pass, call *ast.CallExpr) bool {
	// The callee must denote a type, not a function.
	tv, ok := pass.TypesInfo.Types[call.Fun]
	if !ok || !tv.IsType() {
		return false
	}
	sl, ok := tv.Type.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	elem, ok := sl.Elem().Underlying().(*types.Basic)
	if !ok || elem.Kind() != types.Int32 {
		return false
	}
	argT := pass.TypesInfo.TypeOf(call.Args[0])
	if argT == nil {
		return false
	}
	b, ok := argT.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}

// ps2105KeyIsBlank reports whether the range key is absent or the blank
// identifier — the only forms under which the byte-offset/rune-index key
// difference is unobservable.
func ps2105KeyIsBlank(key ast.Expr) bool {
	if key == nil {
		return true
	}
	id, ok := key.(*ast.Ident)
	return ok && id.Name == "_"
}

// ps2105ContainsCompositeLit reports whether e contains a composite
// literal anywhere: a range header cannot carry a bare composite literal,
// so unwrapping the conversion around one could yield unparsable code.
func ps2105ContainsCompositeLit(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if _, ok := n.(*ast.CompositeLit); ok {
			found = true
		}
		return !found
	})
	return found
}

// ps2105ExprText renders an expression for messages and the fix text.
func ps2105ExprText(e ast.Expr) string {
	var b strings.Builder
	_ = printer.Fprint(&b, token.NewFileSet(), e)
	return b.String()
}
