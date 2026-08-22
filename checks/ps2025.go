package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2025 reports utf8.Valid([]byte(s)) with s a plain string — a call
// that copies the whole string into a throwaway []byte just to validate
// it — and rewrites it to utf8.ValidString(s), the identical validation
// with no copy. The symmetric reverse, utf8.ValidString(string(b)) with
// b a plain []byte, rewrites to utf8.Valid(b) with the same win.
var PS2025 = register(&lint.Check{
	ID:       "PS2025",
	Category: "alloc",
	Slug:     "utf8-valid-throwaway-conversion",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "utf8.Valid([]byte(s)) copies the whole string just to validate it; utf8.ValidString(s) validates in place",
		Text: `utf8.Valid([]byte(s)) copies s into a fresh []byte only to
feed utf8.Valid a value whose bytes it merely reads. utf8.ValidString(s)
runs the identical validation directly over the string's bytes: no
conversion, no copy, no allocation. The validation work itself is the
same in both spellings; the saving is the conversion — a full-length
copy, heap-allocated past the 32-byte stack temporary. One honest
caveat: current gc proves this exact argument-position conversion
read-only and non-escaping and elides it entirely, so the forward
direction is near-parity there — the win is robustness (the elision is
shape- and toolchain-dependent), older Go, and non-gc toolchains. The
symmetric reverse is matched too, and is a REAL win on current gc:
utf8.ValidString(string(b)) with b a []byte must copy b into a fresh
string (b may be mutated after the conversion, so gc cannot alias it),
and rewrites to utf8.Valid(b), dropping the copy and its allocation.

The rewrite is BIT-IDENTICAL on every input. utf8.Valid and
utf8.ValidString implement one validation algorithm twice — the same
word-at-a-time ASCII fast path, the same first/acceptRanges tables, the
same per-sequence bounds checks (the bodies differ only in the operand
type) — and []byte(s) carries byte content identical to s (as does
string(b) to b), so the returned bool is equal for every input: the
empty string, nil (string(nil []byte) is "" and both functions accept
the empty input as valid), pure ASCII, every multi-byte rune, and every
invalid sequence — truncated sequences, bare continuation bytes,
overlong encodings, surrogates, out-of-range starters, 0xFE/0xFF. Both
functions only read their argument, return a plain bool, and evaluate
their argument exactly once, so side effects in the operand expression
run once in the same order in both spellings. The runtime differential
test pins the equivalence over exhaustive short inputs from an
adversarial alphabet, targeted invalid-UTF-8 seeds, and randomized long
inputs, in both directions.

The automatic fix applies only when type information proves the shape:
the callee is the standard library's package-level utf8.Valid (resp.
utf8.ValidString) — a shadowed utf8 identifier or a same-named method
never matches, and an aliased import keeps its qualifier verbatim — the
argument is a conversion to exactly the predeclared []byte (resp. the
predeclared string, the universe identifier itself), and the operand's
static type is a plain string, an untyped string constant, or a plain
[]byte. A NAMED string or byte-slice operand is deliberately not
matched: there the conversion also changes the static type, and the
semantics belong to the named type. A []byte(nil) argument is likewise
out of scope (utf8.ValidString(nil) would not compile). The operand
expression is kept byte-verbatim in place; only the conversion
punctuation around it is deleted and the callee identifier renamed, so
the replacement is a call wherever the original was legal and never
needs parentheses. The fix touches no imports (unicode/utf8 stays
imported). A comment inside the deleted conversion syntax withholds the
fix (the report stays advisory) rather than destroying the comment.`,
		Before: `ok := utf8.Valid([]byte(s))
rev := utf8.ValidString(string(b))`,
		After: `ok := utf8.ValidString(s)
rev := utf8.Valid(b)`,
		MeasuredWin: `BenchmarkPS2025 (64-byte mixed ASCII/multi-byte input,
Apple M2 Pro, go1.26): the reverse direction utf8.ValidString(string(b))
60.2 ns/op, 80 B/op, 1 alloc/op -> utf8.Valid(b) 36.8 ns/op, 0 B/op,
0 allocs/op (~1.6x faster, allocation-free) — the string conversion must
copy because b may be mutated afterwards, and the saving grows linearly
with len(b). The forward direction utf8.Valid([]byte(s)) 35.5 ns/op ->
utf8.ValidString(s) 32.9 ns/op, 0 B/op both sides: current gc proves
this exact non-escaping, read-only conversion unnecessary and elides
the copy, so the forward win there is ~7% plus robustness — on
toolchains and shapes without the elision the Before pays the
full-length copy and (past 32 bytes) its heap allocation. The
validation itself is identical machine code over the same bytes in
every case.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2025",
		Doc:  "utf8.Valid([]byte(s)) / utf8.ValidString(string(b)) copy the whole input through a throwaway conversion just to validate it; the sibling utf8.ValidString(s) / utf8.Valid(b) runs the identical validation in place",
		Run:  runPS2025,
	},
})

func runPS2025(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps2025Match(pass, call)
			if m == nil {
				return true
			}
			msg := "utf8." + m.from + "(" + m.argText + ") copies the whole " + m.copied +
				" through a throwaway conversion just to validate it; utf8." + m.to +
				"(" + m.operandText + ") runs the identical validation in place — same bool for every input, no copy, no allocation"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			if ps2025CommentInSpans(f, m.del) {
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

// ps2025Span is a source range of conversion scaffolding the fix deletes.
type ps2025Span struct {
	pos, end token.Pos
}

// ps2025Site is a matched utf8.Valid/ValidString call whose argument is a
// throwaway conversion: the selector to rewrite, the callee names before
// and after, the scaffolding spans to delete, and the source texts for
// the message.
type ps2025Site struct {
	sel         *ast.SelectorExpr
	from, to    string
	copied      string
	del         []ps2025Span
	argText     string
	operandText string
}

// ps2025Match matches a call to the standard library's package-level
// utf8.Valid whose argument is a predeclared-[]byte conversion of a
// plain-string operand, or utf8.ValidString whose argument is a
// predeclared-string conversion of a plain-[]byte operand. Type
// information pins the callee (a shadowed utf8 identifier, a same-named
// method, or a third-party package never matches), the conversion target
// (exactly []byte resp. the universe string — defined types do not
// match), and the operand's type (a plain or untyped-constant string,
// resp. an unnamed slice of the predeclared byte — NAMED operand types
// are deliberately out of scope, as is untyped nil, for which the
// rewritten call would not compile).
func ps2025Match(pass *analysis.Pass, call *ast.CallExpr) *ps2025Site {
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
	case "Valid":
		// utf8.Valid([]byte(s)) -> utf8.ValidString(s): the conversion
		// target must be exactly []byte and the operand a plain (possibly
		// untyped-constant) string. A named string operand or a defined
		// byte-slice conversion target is out of scope.
		if !ps2108IsByteSliceConv(pass, conv.Fun) {
			return nil
		}
		basic, isBasic := types.Unalias(tv.Type).(*types.Basic)
		if !isBasic || basic.Info()&types.IsString == 0 {
			return nil
		}
		to, copied = "ValidString", "string"
	case "ValidString":
		// utf8.ValidString(string(b)) -> utf8.Valid(b): the conversion
		// target must be the predeclared string itself and the operand an
		// unnamed slice of the predeclared byte. A named byte-slice
		// operand or a plain-string operand (a no-op string(s) conversion
		// — a different pattern) is out of scope.
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
		to, copied = "Valid", "[]byte"
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
	// The deleted spans run from the ARGUMENT's boundaries (not the
	// unparened conversion's), so redundant parentheses wrapped around
	// the conversion are deleted together with it.
	return &ps2025Site{
		sel: sel, from: fn.Name(), to: to, copied: copied,
		del: []ps2025Span{
			{pos: call.Args[0].Pos(), end: operand.Pos()}, // "[]byte(" resp. "string(", plus any wrapping "("s
			{pos: operand.End(), end: call.Args[0].End()}, // ")", plus any wrapping ")"s
		},
		argText: argText, operandText: operandText,
	}
}

// ps2025CommentInSpans reports whether a comment overlaps any of the
// conversion-scaffolding spans the fix would delete.
func ps2025CommentInSpans(f *ast.File, spans []ps2025Span) bool {
	for _, d := range spans {
		if ps2111CommentIn(f, d.pos, d.end) {
			return true
		}
	}
	return false
}
