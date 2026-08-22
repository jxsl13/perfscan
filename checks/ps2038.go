package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2038 reports utf8.DecodeRuneInString(string(b)) — and its
// DecodeLastRuneInString twin — with b a plain []byte: the conversion
// copies the WHOLE slice into a throwaway string just to decode one
// rune at its edge, and rewrites to utf8.DecodeRune(b) (resp.
// utf8.DecodeLastRune(b)), which decodes the same rune in place. The
// symmetric forward shape utf8.DecodeRune([]byte(s)) with s a plain
// string rewrites to utf8.DecodeRuneInString(s) with the same
// bit-identical result. This is the decode sibling of PS2024
// (RuneCount) and PS2025 (Valid) — same throwaway-conversion pattern,
// same shared decode core, and here the Before's copy is even more
// disproportionate: the whole input is copied to read at most 4 bytes.
var PS2038 = register(&lint.Check{
	ID:       "PS2038",
	Category: "alloc",
	Slug:     "utf8-decoderune-throwaway-conversion",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "utf8.DecodeRuneInString(string(b)) copies the whole []byte to decode one rune; utf8.DecodeRune(b) decodes it in place",
		Text: `utf8.DecodeRuneInString(string(b)) with b a []byte copies the
ENTIRE slice into a fresh string — gc cannot alias a string to mutable
[]byte memory, so the conversion is a real full-length copy,
heap-allocated past the 32-byte stack temporary — only to decode the
single rune at its start (at most 4 bytes of it). utf8.DecodeRune(b)
reads b's bytes directly: no conversion, no copy, no allocation, and
the cost stops depending on len(b) altogether. The same holds for the
DecodeLastRuneInString(string(b)) -> DecodeLastRune(b) pair. The
forward direction — utf8.DecodeRune([]byte(s)) with s a plain string ->
utf8.DecodeRuneInString(s) — is matched too; on current gc that
argument-position conversion is proved read-only and non-escaping and
is elided, so the forward win there is robustness (the elision is
shape- and toolchain-dependent), older Go, and non-gc toolchains; it is
never a regression.

The rewrite is BIT-IDENTICAL on every input. unicode/utf8 implements
DecodeRune and DecodeRuneInString as the same algorithm twice — the
same inlineable ASCII fast path, one shared pair of slow paths built on
the same first/acceptRanges tables, bodies differing only in whether
the operand is indexed as []byte or string — and DecodeLastRune /
DecodeLastRuneInString share the identical backward scan (bounded by
UTFMax) before delegating to that same decode core. For identical
bytes each pair returns identical (r, size) on every input: empty
(both (RuneError, 0) — string(nil []byte) is ""), a lone continuation
or otherwise invalid byte (both (RuneError, 1)), truncated multibyte
sequences at either edge, overlong encodings, surrogate halves,
out-of-range leads, and every valid rune of every width. string(b) and
[]byte(s) are builtin conversions that copy bytes verbatim and never
dispatch a method, so no String()/Format() on a named operand type can
intercept. Both callees are pure and evaluate their single operand
exactly once, so side effects in the operand expression run once, in
the same order, in both spellings.

The automatic fix applies only when type information proves the shape:
the callee is the standard library's package-level
utf8.DecodeRuneInString / DecodeLastRuneInString (resp. DecodeRune /
DecodeLastRune) — a shadowed utf8 identifier or a same-named local
function never matches, and an aliased import keeps its qualifier
verbatim — the argument is DIRECTLY a conversion to exactly the
predeclared string (the universe identifier itself; resp. to exactly
[]byte, an unnamed slice of the predeclared byte), and the operand's
static type is a plain []byte (resp. a plain string or an untyped
string constant). A NAMED string or byte-slice operand is deliberately
not matched: there the conversion also changes the static type, and
the semantics belong to the named type. A []rune (or []int32) operand
never matches: string([]rune{...}) UTF-8-ENCODES its runes — a
different operation, not a byte-verbatim copy. An untyped nil operand
is likewise out of scope (string(nil) does not compile). A conversion
result stored in a variable first may have other consumers and is out
of scope. The fix keeps the operand byte-verbatim in place — same
text, same single evaluation — renames the callee identifier to its
sibling, and deletes only the conversion punctuation around the
operand; the replacement is a call wherever the original was legal and
never needs parentheses. No import is added or orphaned (unicode/utf8
stays referenced by the sibling). A comment inside the deleted
conversion syntax withholds the fix (the report stays advisory) rather
than destroying the comment.`,
		Before: `r, size := utf8.DecodeRuneInString(string(b))
last, lsz := utf8.DecodeLastRuneInString(string(b))
fr, fsz := utf8.DecodeRune([]byte(s))`,
		After: `r, size := utf8.DecodeRune(b)
last, lsz := utf8.DecodeLastRune(b)
fr, fsz := utf8.DecodeRuneInString(s)`,
		MeasuredWin: `BenchmarkPS2038 (85-byte mixed-width operand, Apple M2
Pro, gc 1.26; the slice is mutated each iteration so the conversion's
copy cannot be elided): reverse direction
utf8.DecodeRuneInString(string(b)) 23.33 ns/op, 96 B/op, 1 alloc/op ->
utf8.DecodeRune(b) 1.08 ns/op, 0 B/op, 0 allocs/op (~22x — the whole
string allocation removed, and the remaining cost no longer scales
with len(b)). DecodeLastRuneInString(string(b)) 22.75 ns/op, 96 B/op,
1 alloc/op -> DecodeLastRune(b) 2.02 ns/op, 0 B/op, 0 allocs/op
(~11x). Forward direction utf8.DecodeRune([]byte(s)) ->
utf8.DecodeRuneInString(s): ~parity on current gc (0.82 vs 0.77
ns/op, 0 allocs both — the compiler elides this read-only,
non-escaping conversion), never a regression; on toolchains and shapes
without the elision the Before pays the full-length copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2038",
		Doc:  "utf8.DecodeRuneInString(string(b)) / utf8.DecodeRune([]byte(s)) (and the DecodeLastRune pair) copy the whole input through a throwaway conversion to decode one rune; the sibling decodes the identical (r, size) in place",
		Run:  runPS2038,
	},
})

func runPS2038(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps2038Match(pass, call)
			if m == nil {
				return true
			}
			msg := "utf8." + m.from + "(" + m.argText + ") copies the whole " + m.copied +
				" through a throwaway conversion just to decode its " + m.which + " rune; utf8." + m.to +
				"(" + m.operandText + ") decodes the identical (r, size) in place — no copy, no allocation"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			if ps2038CommentInSpans(f, m.del) {
				// A comment sits inside the conversion syntax the fix would
				// delete: withhold the fix rather than destroy the comment.
				diag.Message = msg + "; a comment inside the conversion syntax withholds the automatic fix — rewrite by hand"
			} else {
				// Rename the callee identifier and unwrap the operand in
				// place: only the conversion punctuation around the kept
				// operand is deleted, so its source bytes carry over
				// verbatim and evaluation order is untouched.
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

// ps2038Span is a source range of conversion scaffolding the fix deletes.
type ps2038Span struct {
	pos, end token.Pos
}

// ps2038Site is a matched decode call whose argument is a throwaway
// conversion: the selector to rewrite, the callee names before and
// after, the scaffolding spans to delete, and the source texts for the
// message.
type ps2038Site struct {
	sel         *ast.SelectorExpr
	from, to    string
	copied      string
	which       string
	del         []ps2038Span
	argText     string
	operandText string
}

// ps2038Match matches a call to the standard library's package-level
// utf8.DecodeRuneInString / DecodeLastRuneInString whose argument is a
// predeclared-string conversion of a plain-[]byte operand, or
// utf8.DecodeRune / DecodeLastRune whose argument is a
// predeclared-[]byte conversion of a plain-string operand. Type
// information pins the callee (a shadowed utf8 identifier, a same-named
// local function, or a third-party package never matches), the
// conversion target (exactly the universe string resp. exactly []byte —
// defined types do not match), and the operand's type (an unnamed slice
// of the predeclared byte — so []rune, whose conversion ENCODES rather
// than copies, can never match — resp. a plain or untyped-constant
// string; NAMED operand types and untyped nil are out of scope).
func ps2038Match(pass *analysis.Pass, call *ast.CallExpr) *ps2038Site {
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

	var to, copied, which string
	switch name := fn.Name(); name {
	case "DecodeRuneInString", "DecodeLastRuneInString":
		// utf8.Decode(Last)RuneInString(string(b)) -> utf8.Decode(Last)Rune(b):
		// the conversion target must be the predeclared string itself and
		// the operand an unnamed slice of the predeclared byte. A named
		// byte-slice operand, a []rune operand (string([]rune) encodes —
		// a different operation), or a plain-string operand (a no-op
		// string(s) conversion — a different pattern) is out of scope.
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
		to, copied = strings.Replace(name, "InString", "", 1), "[]byte"
	case "DecodeRune", "DecodeLastRune":
		// utf8.Decode(Last)Rune([]byte(s)) -> utf8.Decode(Last)RuneInString(s):
		// the conversion target must be exactly []byte and the operand a
		// plain (possibly untyped-constant) string. A named string operand
		// or a defined byte-slice conversion target is out of scope.
		if !ps2108IsByteSliceConv(pass, conv.Fun) {
			return nil
		}
		basic, isBasic := types.Unalias(tv.Type).(*types.Basic)
		if !isBasic || basic.Info()&types.IsString == 0 {
			return nil
		}
		to, copied = name+"InString", "string"
	default:
		return nil
	}
	if strings.HasPrefix(fn.Name(), "DecodeLast") {
		which = "last"
	} else {
		which = "first"
	}
	argText, okText := ps5004ExprText(arg)
	if !okText {
		return nil
	}
	operandText, okText := ps5004ExprText(operand)
	if !okText {
		return nil
	}
	// The deleted spans run from the ARGUMENT's boundaries (not the
	// unparened conversion's), so redundant parentheses wrapped around
	// the conversion are deleted together with it.
	return &ps2038Site{
		sel: sel, from: fn.Name(), to: to, copied: copied, which: which,
		del: []ps2038Span{
			{pos: call.Args[0].Pos(), end: operand.Pos()}, // "string(" resp. "[]byte(", plus any wrapping "("s
			{pos: operand.End(), end: call.Args[0].End()}, // ")", plus any wrapping ")"s
		},
		argText: argText, operandText: operandText,
	}
}

// ps2038CommentInSpans reports whether a comment overlaps any of the
// conversion-scaffolding spans the fix would delete.
func ps2038CommentInSpans(f *ast.File, spans []ps2038Span) bool {
	for _, d := range spans {
		if ps2111CommentIn(f, d.pos, d.end) {
			return true
		}
	}
	return false
}
