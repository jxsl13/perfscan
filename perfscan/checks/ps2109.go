package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2109 reports []byte(fmt.Sprintf(...)) — formatting into a string only
// to convert it, when fmt.Appendf writes the bytes directly. Likewise
// []byte(fmt.Sprint(...)) -> fmt.Append and []byte(fmt.Sprintln(...)) ->
// fmt.Appendln.
var PS2109 = register(&lint.Check{
	ID:       "PS2109",
	Category: "alloc",
	Slug:     "byte-sprintf-to-appendf",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "[]byte(fmt.Sprintf(...)) allocates a string then copies it; fmt.Appendf writes bytes directly",
		Text: `fmt.Sprintf builds its result in a byte buffer, converts that
buffer to a string (one allocation + copy), and the surrounding []byte
conversion immediately copies the string back into a fresh byte slice
(a second allocation + copy). fmt.Appendf(nil, ...) runs the identical
formatting logic but appends the buffer straight into a byte slice: same
bytes, half the allocations, half the copying. The same pairing holds for
fmt.Sprint -> fmt.Append and fmt.Sprintln -> fmt.Appendln.

The rewrite edits around the call: the format string and every argument
are preserved verbatim, in order, with unchanged evaluation. Only the
selected name changes (Sprintf -> Appendf), so an aliased fmt import
keeps working, and since Appendf lives in fmt the import can never be
orphaned. Only the exact predeclared []byte(...) conversion of a call
type-pinned to the standard library's fmt is matched — a named byte-slice
conversion changes the expression's type and is out of scope.

One observable corner is guarded: when the formatted output is EMPTY,
[]byte("") is a non-nil empty slice while fmt.Appendf(nil, ...) returns
nil — and nil-ness is observable. The fix is therefore only offered when
the output is provably non-empty: a Sprintf format literal containing at
least one literal (non-directive) byte, any Sprintln call (it always
appends a newline), or a Sprint call with an unnamed basic numeric or
boolean operand (those always render at least one byte and cannot carry a
Stringer). Every other match is reported without a fix.`,
		Before: `payload := []byte(fmt.Sprintf("user=%d", id))`,
		After:  `payload := fmt.Appendf(nil, "user=%d", id)`,
		MeasuredWin: `BenchmarkPS2109 ("user=%d flags=%s" over an int and a
short string, Apple M2 Pro): 78 ns/op, 48 B, 2 allocs -> 71 ns/op, 24 B,
1 alloc — one full allocation+copy of the result removed; the gap widens
with output size since the saved copy is O(len).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2109",
		Doc:  "[]byte(fmt.Sprintf(...)) instead of fmt.Appendf(nil, ...)",
		Run:  runPS2109,
	},
})

// ps2109Append maps each matched fmt formatter to its byte-slice twin.
var ps2109Append = map[string]string{
	"Sprintf":  "Appendf",
	"Sprint":   "Append",
	"Sprintln": "Appendln",
}

func runPS2109(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			outer, ok := n.(*ast.CallExpr)
			if !ok || len(outer.Args) != 1 || outer.Ellipsis.IsValid() {
				return true
			}
			if !ps2109IsByteSliceConv(pass, outer) {
				return true
			}
			call, ok := ps2109Unparen(outer.Args[0]).(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ps2109FmtSprintName(pass, call)
			if name == "" {
				return true
			}
			appendName := ps2109Append[name]
			diag := analysis.Diagnostic{
				Pos: outer.Pos(),
				End: outer.End(),
				Message: "[]byte(fmt." + name + "(...)) allocates an intermediate string and copies it; fmt." +
					appendName + "(nil, ...) formats the bytes directly",
			}
			// Nil-ness guard: for empty output []byte("") is non-nil while
			// fmt.Appendf(nil, ...) returns nil. Fix only when the output is
			// provably non-empty; otherwise the report stays advisory. The
			// deleted ranges (the conversion shell) must not swallow comments.
			if ps2109ProvenNonEmpty(pass, name, call) &&
				!ps2109CommentBetween(f, outer.Pos(), call.Pos()) &&
				!ps2109CommentBetween(f, call.End(), outer.End()) {
				sel := call.Fun.(*ast.SelectorExpr)
				insert := "nil"
				if len(call.Args) > 0 {
					insert = "nil, "
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace with fmt." + appendName + "(nil, ...)",
					TextEdits: []analysis.TextEdit{
						// Drop "[]byte(" (and any wrapping parens) ...
						{Pos: outer.Pos(), End: call.Pos()},
						// ... rename Sprintf -> Appendf, keeping the (possibly
						// aliased) qualifier and every argument untouched ...
						{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte(appendName)},
						// ... prepend the nil destination slice ...
						{Pos: call.Lparen + 1, End: call.Lparen + 1, NewText: []byte(insert)},
						// ... and drop the conversion's closing ")".
						{Pos: call.End(), End: outer.End()},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2109IsByteSliceConv reports whether call is a conversion to the exact
// predeclared []byte type. Type information distinguishes a conversion
// from a function call and rejects named slice types (whose identity the
// rewrite would change) and shadowed identifiers.
func ps2109IsByteSliceConv(pass *analysis.Pass, call *ast.CallExpr) bool {
	tv, ok := pass.TypesInfo.Types[call.Fun]
	if !ok || !tv.IsType() {
		return false
	}
	sl, ok := tv.Type.(*types.Slice)
	if !ok {
		return false
	}
	eb, ok := sl.Elem().(*types.Basic)
	return ok && eb.Kind() == types.Byte
}

// ps2109FmtSprintName returns "Sprintf", "Sprint" or "Sprintln" when call
// invokes that package-level function of the standard library's fmt, and
// "" otherwise. Type information pins the callee: a local method or value
// spelled the same, or a shadowed fmt identifier, does not resolve to a
// receiver-less *types.Func with package path "fmt".
func ps2109FmtSprintName(pass *analysis.Pass, call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" {
		return ""
	}
	if _, known := ps2109Append[fn.Name()]; !known {
		return ""
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return ""
	}
	return fn.Name()
}

// ps2109ProvenNonEmpty reports whether the call's formatted output is
// provably at least one byte long for every possible argument value —
// the condition under which replacing the non-nil []byte("") result
// shape with fmt.Appendf(nil, ...) is bit-identical.
func ps2109ProvenNonEmpty(pass *analysis.Pass, name string, call *ast.CallExpr) bool {
	switch name {
	case "Sprintln":
		// Sprintln always appends a trailing newline.
		return true
	case "Sprintf":
		if len(call.Args) == 0 {
			return false
		}
		lit, ok := ps2109Unparen(call.Args[0]).(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return false
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			return false
		}
		return ps2109FormatHasLiteralByte(s)
	case "Sprint":
		return ps2109SprintNonEmpty(pass, call)
	}
	return false
}

// ps2109FormatHasLiteralByte reports whether the Sprintf format string
// emits at least one literal byte regardless of the arguments: any byte
// outside a % directive, or an escaped %%. A format made purely of verbs
// can render empty (e.g. "%s" of "") and proves nothing.
func ps2109FormatHasLiteralByte(format string) bool {
	i := 0
	for i < len(format) {
		if format[i] != '%' {
			return true
		}
		i++
		if i < len(format) && format[i] == '%' {
			return true
		}
		// Skip the directive: flags, width, precision, arg indexes.
		for i < len(format) && strings.IndexByte("+-# 0123456789.*[]", format[i]) >= 0 {
			i++
		}
		if i >= len(format) {
			return false // malformed trailing directive: unproven
		}
		_, size := utf8.DecodeRuneInString(format[i:]) // the verb rune
		i += size
	}
	return false
}

// ps2109SprintNonEmpty reports whether a Sprint call provably renders at
// least one byte: an operand of unnamed basic numeric or boolean type
// (always at least one character, and an unnamed basic type cannot carry
// a Stringer/Formatter), or a non-empty string constant. A spread call
// (args...) exposes no operands to inspect.
func ps2109SprintNonEmpty(pass *analysis.Pass, call *ast.CallExpr) bool {
	if call.Ellipsis.IsValid() {
		return false
	}
	for _, a := range call.Args {
		tv, ok := pass.TypesInfo.Types[a]
		if !ok || tv.Type == nil {
			continue
		}
		b, ok := types.Default(tv.Type).(*types.Basic)
		if !ok {
			continue
		}
		if b.Info()&(types.IsNumeric|types.IsBoolean) != 0 {
			return true
		}
		if b.Info()&types.IsString != 0 && tv.Value != nil &&
			tv.Value.Kind() == constant.String && constant.StringVal(tv.Value) != "" {
			return true
		}
	}
	return false
}

// ps2109Unparen strips any parentheses around e.
func ps2109Unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// ps2109CommentBetween reports whether a comment overlaps [from, to) —
// a deletion covering it would silently drop the comment.
func ps2109CommentBetween(f *ast.File, from, to token.Pos) bool {
	for _, cg := range f.Comments {
		if cg.End() > from && cg.Pos() < to {
			return true
		}
	}
	return false
}
