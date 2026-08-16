package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5039 reports append(dst, string(r)...) — a single non-constant rune
// UTF-8-encoded into a throwaway string only to be appended — where
// utf8.AppendRune(dst, r) runs the identical encoder straight into dst's
// tail: no intermediate string, no second copy. The append analog of
// PS2139 (w.WriteString(string(r)) -> w.WriteRune(r)), built on the same
// operand gate (ps2139RuneConv).
var PS5039 = register(&lint.Check{
	ID:       "PS5039",
	Category: "alloc",
	Slug:     "append-string-of-rune",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "append(dst, string(r)...) encodes a rune into a throwaway string append then copies again; utf8.AppendRune(dst, r) encodes into dst directly",
		Text: `append(dst, string(r)...) for a non-constant rune does the work
twice: string(r) runs the rune-to-UTF-8 encoder into a temporary 1-4
byte string (building a string header, and heap-allocating when escape
analysis cannot stack the temporary), and append then copies those
bytes a SECOND time into dst's tail. utf8.AppendRune(dst, r) runs the
same encoder once, directly into dst's backing array. (A CONSTANT rune
is out of scope: string('a') is folded into a constant string at
compile time and allocates nothing.)

The appended BYTES are identical for EVERY int32 value: string(r) and
utf8.AppendRune dispatch to the same UTF-8 encoding, and both map every
invalid rune — negative, the surrogate range 0xD800-0xDFFF, and values
above 0x10FFFF — to U+FFFD (0xEF 0xBF 0xBD). This is the exact identity
PS2139 already proves for string(r) vs WriteRune, pinned here by the
runtime differential (exhaustive over the whole valid-plus-margin
window and a full strided int32 sweep). dst and r are each evaluated
exactly once, in the same order; when dst has spare capacity both
spellings write the same backing array in place and preserve its
capacity exactly. The one non-guarantee is the PS2112-accepted class
PS5017 also documents: when append must GROW, the fresh slice's
CAPACITY is an unspecified implementation detail (the runtime growth
paths request the same new length and agree, but an inlined
utf8.AppendRune can let the compiler stack-place or round the new
backing array differently). Length and bytes are identical always; no
correct program relies on the rounding of a freshly grown capacity.

Three gates are load-bearing:

  - INTEGER WIDTH (inherited from PS2139). The conversion operand's
    static type must fit losslessly in int32: rune/int32 itself or a
    named rune type (kept verbatim, or wrapped as rune(x) when not
    assignable to rune), or byte, int8, int16, uint16 — widened
    value-preservingly as rune(x). A WIDER type (int, int64, uint32,
    uintptr, ...) is never touched: string(x) converts the FULL value
    (out of range becomes U+FFFD) while rune(x) would truncate first
    and diverge — string(int64(0x100000041)) is "�" but
    string(rune(int64(0x100000041))) is "A".

  - CONVERSION. The argument must be exactly the spread of the
    predeclared string conversion — string(x)... with two call
    arguments in total. A named string type's conversion (whose value
    could flow anywhere) and a plain string variable never match; the
    callee append must resolve to the predeclared builtin (a shadowing
    local append does not).

  - DESTINATION TYPE. The automatic fix applies only when dst's static
    type is exactly the unnamed []byte, so the rewritten expression
    keeps its static type (utf8.AppendRune returns []byte). A NAMED
    byte-slice dst compiles with append — which returns dst's own type
    — but the rewrite would change the expression's static type (and
    break a method call or %T on the result), so it keeps the advisory
    report.

The automatic fix keeps dst and the operand byte-verbatim in place —
same expressions, same single evaluation, same order — and rewrites
only the scaffolding: append( becomes utf8.AppendRune(, the string(
conversion (and the trailing ...) vanish, and the operand is wrapped as
rune(x) for the narrower widths. The unicode/utf8 import is added when
missing, reusing an existing import's alias; a dot or blank utf8
import, a shadowed utf8 qualifier at the call site, a cgo file whose
import block must not be edited, or a comment inside the rewritten
scaffolding keeps the report advisory.`,
		Before: `dst = append(dst, string(r)...)`,
		After:  `dst = utf8.AppendRune(dst, r)`,
		MeasuredWin: `BenchmarkPS5039 (1024 runes appended into a reset
preallocated []byte, Apple M2 Pro, go1.26): mixed 1-4 byte runes
5311 ns/op -> 2186 ns/op (~2.4x); pure ASCII 4458 ns/op -> 654 ns/op
(~6.8x — AppendRune's ASCII path inlines to a plain one-byte append,
skipping the runtime.intstring round trip entirely). Both sides 0 B/op
here (the temporary string stack-spills when it does not escape); when
the conversion escapes, string(r) additionally heap-allocates on every
multi-byte rune, which AppendRune never does.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5039",
		Doc:  "append(dst, string(r)...) of a single rune instead of utf8.AppendRune(dst, r)",
		Run:  runPS5039,
	},
})

func runPS5039(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// All fixes of a run are applied together, so only the first fix
		// needing the unicode/utf8 import carries the import edit.
		importAdded := false
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || !call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the predeclared append builtin, not a
			// shadowing local.
			fn, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if b, ok := pass.TypesInfo.Uses[fn].(*types.Builtin); !ok || b.Name() != "append" {
				return true
			}
			// The spread argument must be the non-constant predeclared
			// string conversion of an int32-lossless integer — the same
			// operand gate as PS2139, and the whole width-safety story.
			conv, inner, needsWrap := ps2139RuneConv(pass, call.Args[1])
			if conv == nil {
				return true
			}
			dst := call.Args[0]
			dstType := pass.TypesInfo.TypeOf(dst)
			if dstType == nil {
				return true
			}
			msg := "append(dst, string(r)...) UTF-8-encodes r into a throwaway string that append copies a second time; utf8.AppendRune(dst, r) encodes straight into dst"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			if !types.Identical(dstType, types.NewSlice(types.Typ[types.Uint8])) {
				// append returns dst's own (named or generic) type;
				// utf8.AppendRune returns []byte — the rewrite would change
				// the expression's static type, so it stays advisory.
				diag.Message = msg + "; dst's static type is not the plain []byte and utf8.AppendRune returns []byte, so the rewrite would change the expression's static type — rewrite by hand if nothing relies on it"
			} else if fix := ps5039Fix(pass, f, call, dst, inner, needsWrap, &importAdded); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps5039Fix builds the utf8.AppendRune(dst, r) rewrite, or returns nil when
// any guard fails and the report must stay advisory. dst and the conversion
// operand are kept byte-verbatim in place — same expressions, same single
// evaluation, same order — and only the scaffolding around them is edited:
// "append(" becomes "<utf8>.AppendRune(", the ", string(" span becomes ", "
// (or ", rune(" for the narrower widths the match admits), and the closing
// ")...)" span becomes ")" (or "))").
func ps5039Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, dst, inner ast.Expr, needsWrap bool, importAdded *bool) *analysis.SuggestedFix {
	// A comment anywhere inside the rewritten scaffolding would be
	// silently destroyed — advisory then. (Comments inside dst or the
	// operand survive verbatim.)
	if ps2111CommentIn(f, call.Pos(), dst.Pos()) ||
		ps2111CommentIn(f, dst.End(), inner.Pos()) ||
		ps2111CommentIn(f, inner.End(), call.End()) {
		return nil
	}
	// Reuse the file's existing unicode/utf8 import name (possibly an
	// alias); dot and blank imports leave no usable qualifier.
	name, ok := ps2125Utf8Name(f)
	if !ok {
		return nil
	}
	// The name must actually reference unicode/utf8 at the call site — a
	// shadowing local would capture the qualifier.
	needImport, usable := ps2110PkgUsable(pass, call.Pos(), name, "unicode/utf8")
	if !usable {
		return nil
	}
	// Never edit a cgo file's import block, and never import unicode/utf8
	// into itself (were this check ever run over the standard library).
	if needImport && (ps2110ImportsC(f) || pass.Pkg.Path() == "unicode/utf8") {
		return nil
	}
	sep, closing := ", ", ")"
	if needsWrap {
		sep, closing = ", rune(", "))"
	}
	edits := []analysis.TextEdit{
		{Pos: call.Pos(), End: dst.Pos(), NewText: []byte(name + ".AppendRune(")},
		{Pos: dst.End(), End: inner.Pos(), NewText: []byte(sep)},
		{Pos: inner.End(), End: call.End(), NewText: []byte(closing)},
	}
	if needImport && !*importAdded {
		edits = append(edits, ps2110ImportEdit(f, "unicode/utf8"))
		*importAdded = true
	}
	return &analysis.SuggestedFix{
		Message:   "replace with " + name + ".AppendRune(...)",
		TextEdits: edits,
	}
}
