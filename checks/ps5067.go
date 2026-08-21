package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5067 reports utf8.FullRuneInString(string(b)) with b a plain []byte — the
// conversion copies the WHOLE slice into a throwaway string just to test whether
// it begins with a full rune — and rewrites to utf8.FullRune(b), which tests the
// same bytes in place. The symmetric forward shape utf8.FullRune([]byte(s)) with
// s a plain string rewrites to utf8.FullRuneInString(s) with the same result.
// The FullRune sibling of PS2038 (DecodeRune), PS2024 (RuneCount) and PS2025
// (Valid) — same throwaway-conversion pattern over the same shared UTF-8 core.
var PS5067 = register(&lint.Check{
	ID:       "PS5067",
	Category: "alloc",
	Slug:     "utf8-fullrune-throwaway-conversion",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "utf8.FullRuneInString(string(b)) copies the whole []byte to test one rune; utf8.FullRune(b) tests it in place",
		Text: `utf8.FullRuneInString(string(b)) with b a []byte copies the ENTIRE
slice into a fresh string — gc cannot alias a string to mutable []byte memory,
so the conversion is a real full-length copy, heap-allocated past the small stack
temporary — only to test whether it begins with a full UTF-8 rune (reading at
most 4 bytes). utf8.FullRune(b) reads b's bytes directly: no conversion, no copy,
no allocation, and the cost stops depending on len(b). The forward direction —
utf8.FullRune([]byte(s)) with s a plain string -> utf8.FullRuneInString(s) — is
matched too; on current gc that read-only, non-escaping argument conversion is
elided (so it is parity there, never a regression), while older or non-gc
toolchains pay the full-length copy.

The rewrite is BIT-IDENTICAL. unicode/utf8 implements FullRune and
FullRuneInString as the same test twice — the same ASCII fast path and the same
first/acceptRanges table logic, differing only in whether the operand is indexed
as []byte or string — so for identical bytes they return the identical bool on
every input: empty (both true — an empty operand needs no more bytes),
a truncated multibyte sequence (false), a lone continuation or otherwise invalid
byte (true — it is a complete, if invalid, encoding), and every valid rune of
every width. string(b) and []byte(s) are builtin conversions that copy bytes
verbatim and never dispatch a method, so no String()/Format() on a named operand
type can intercept. Both callees are pure and evaluate their operand once.

The fix applies only when type information proves the shape: the callee is the
package-level utf8.FullRuneInString (resp. FullRune) — a shadowed utf8 or a
same-named local never matches, an aliased import keeps its qualifier — the
argument is DIRECTLY a conversion to exactly the predeclared string (resp. to
exactly []byte, an unnamed slice of the predeclared byte), and the operand's
static type is a plain []byte (resp. a plain or untyped-constant string). A NAMED
string or byte-slice operand, a []rune operand (string([]rune) ENCODES — a
different operation), and an untyped nil are all out of scope. The fix renames
the callee to its sibling and deletes only the conversion punctuation around the
byte-verbatim operand; no import is added or orphaned (unicode/utf8 stays
referenced). A comment inside the deleted conversion syntax withholds the fix.`,
		Before: `ok := utf8.FullRuneInString(string(b))
fwd := utf8.FullRune([]byte(s))`,
		After: `ok := utf8.FullRune(b)
fwd := utf8.FullRuneInString(s)`,
		MeasuredWin: `On an ~85-byte []byte operand (Apple M2 Pro, gc 1.26): reverse ` +
			`direction utf8.FullRuneInString(string(b)) ~15 ns/op, 64 B/op, 1 alloc/op vs ` +
			`utf8.FullRune(b) ~0.8 ns/op, 0 B/op, 0 allocs/op (~18x — the whole string allocation ` +
			`removed, and the cost no longer scales with len(b)). Forward direction is parity on ` +
			`current gc (the read-only conversion is elided), never a regression.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5067",
		Doc:  "utf8.FullRuneInString(string(b)) / utf8.FullRune([]byte(s)) copy the whole input through a throwaway conversion; the sibling tests the identical bool in place",
		Run:  runPS5067,
	},
})

func runPS5067(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5067Match(pass, call)
			if m == nil {
				return true
			}
			msg := "utf8." + m.from + "(" + m.argText + ") copies the whole " + m.copied +
				" through a throwaway conversion just to test one rune; utf8." + m.to +
				"(" + m.operandText + ") tests the identical bool in place — no copy, no allocation"
			diag := analysis.Diagnostic{Pos: call.Pos(), End: call.End(), Message: msg}
			if ps2038CommentInSpans(f, m.del) {
				diag.Message = msg + "; a comment inside the conversion syntax withholds the automatic fix — rewrite by hand"
			} else {
				edits := []analysis.TextEdit{
					{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte(m.to)},
				}
				for _, d := range m.del {
					edits = append(edits, analysis.TextEdit{Pos: d.pos, End: d.end})
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace " + m.from + " with " + m.to + " and drop the conversion",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

type ps5067Site struct {
	sel         *ast.SelectorExpr
	from, to    string
	copied      string
	del         []ps2038Span
	argText     string
	operandText string
}

// ps5067Match matches a call to the package-level utf8.FullRuneInString whose
// argument is a predeclared-string conversion of a plain-[]byte operand, or
// utf8.FullRune whose argument is a predeclared-[]byte conversion of a
// plain-string operand.
func ps5067Match(pass *analysis.Pass, call *ast.CallExpr) *ps5067Site {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "unicode/utf8" {
		return nil
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil
	}
	arg := ps2108Unparen(call.Args[0])
	conv, isConv := arg.(*ast.CallExpr)
	if !isConv || len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil
	}
	operand := conv.Args[0]
	tv, found := pass.TypesInfo.Types[operand]
	if !found || tv.Type == nil {
		return nil
	}

	var to, copied string
	switch fn.Name() {
	case "FullRuneInString":
		// utf8.FullRuneInString(string(b)) -> utf8.FullRune(b): the conversion
		// target must be the predeclared string and the operand an unnamed slice
		// of the predeclared byte.
		if !ps2108IsUniverseString(pass, conv.Fun) {
			return nil
		}
		sl, isSlice := types.Unalias(tv.Type).(*types.Slice)
		if !isSlice {
			return nil
		}
		eb, isBasic := types.Unalias(sl.Elem()).(*types.Basic)
		if !isBasic || eb.Kind() != types.Byte {
			return nil
		}
		to, copied = "FullRune", "[]byte"
	case "FullRune":
		// utf8.FullRune([]byte(s)) -> utf8.FullRuneInString(s): the conversion
		// target must be exactly []byte and the operand a plain (or
		// untyped-constant) string.
		if !ps2108IsByteSliceConv(pass, conv.Fun) {
			return nil
		}
		basic, isBasic := types.Unalias(tv.Type).(*types.Basic)
		if !isBasic || basic.Info()&types.IsString == 0 {
			return nil
		}
		to, copied = "FullRuneInString", "string"
	default:
		return nil
	}
	argText, okText := ps5004ExprText(arg)
	if !okText {
		return nil
	}
	operandText, okText := ps5004ExprText(operand)
	if !okText {
		return nil
	}
	return &ps5067Site{
		sel: sel, from: fn.Name(), to: to, copied: copied,
		del: []ps2038Span{
			{pos: call.Args[0].Pos(), end: operand.Pos()},
			{pos: operand.End(), end: call.Args[0].End()},
		},
		argText: argText, operandText: operandText,
	}
}
