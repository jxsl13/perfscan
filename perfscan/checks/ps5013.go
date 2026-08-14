package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5013 reports bytes.Index(b, []byte{c}) / bytes.Index(b, []byte("z"))
// and the LastIndex siblings where the needle is statically one byte:
// both construct a throwaway one-element slice and route a single byte
// through the generic substring-search machinery, while bytes.IndexByte /
// bytes.LastIndexByte take the byte directly. The bytes twin of PS5007.
var PS5013 = register(&lint.Check{
	ID:       "PS5013",
	Category: "arith",
	Slug:     "bytes-index-single-byte-needle",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "bytes.Index/LastIndex of a one-byte needle builds a slice and runs the substring machinery; IndexByte/LastIndexByte take the byte directly",
		Text: `bytes.Index(b, []byte{'/'}) pays a per-call dispatch over the
needle length before it discovers len(sep) == 1 and delegates to
IndexByte(s, sep[0]); bytes.LastIndex likewise dispatches and delegates
to the backward byte scan for a one-byte needle. On top of that
dispatch, the CALLER constructs a throwaway one-element []byte for
every call — []byte{'/'} or []byte("z") — that the byte forms never
need. bytes.IndexByte(b, '/') and bytes.LastIndexByte(b, 'z') skip
straight to the SIMD forward scan / plain backward scan with no needle
construction and no dispatch. Escape analysis keeps the one-element
slice on the stack, so no side allocates — the win is pure instruction
count, in exactly the parsing and splitting loops where these calls
cluster. This is the bytes twin of PS5007 (the strings version).

Two needle shapes are matched, both statically one byte long:

  - a composite literal []byte{X} with EXACTLY one element and no key —
    X is byte-assignable by the composite-literal typing rule, so
    bytes.IndexByte(b, X) type-checks as-is, and X is evaluated exactly
    once in the same argument position, so side effects and evaluation
    order are preserved verbatim ([]byte{f()} -> IndexByte(b, f()));
  - a conversion []byte(C) where C is a compile-time constant string of
    byte-length EXACTLY 1. The length rule is bytes, not runes: "é"
    (two bytes of UTF-8) is out of scope, while a raw single-byte
    escape such as "\xff" or "\x00" — even invalid UTF-8 — is in scope,
    because a one-byte needle is matched byte-wise on both sides of the
    rewrite.

The empty needle ([]byte{}, []byte(""), or nil) is excluded:
bytes.Index(b, empty) is 0 and LastIndex(b, empty) is len(b) —
semantics IndexByte cannot express. Multi-element and keyed composite
literals and needles that are not statically one byte are out of scope.

The rewrite is BIT-IDENTICAL on every input. For len(sep) == 1,
bytes.Index(s, sep) is implemented as IndexByte(s, sep[0]) and
bytes.LastIndex(s, sep) as bytealg.LastIndexByte(s, sep[0]) — the exact
function bytes.LastIndexByte wraps — so both pairs perform the
identical raw byte-wise search: no rune decoding, no case folding, no
UTF-8 validation anywhere on either path. The returned index is equal
for every haystack: empty or nil, needle absent, needle at either end,
repeated needles, NUL bytes, and invalid UTF-8 (pinned by the
equivalence suite over all 256 needle bytes crossed with adversarial
and randomized haystacks). The haystack argument passes through
byte-verbatim, the result is a plain int, and both arguments are
evaluated exactly once in the original order — no named-type, Stringer,
nil, or overflow surface exists.

The automatic fix rewrites the callee name (Index -> IndexByte,
LastIndex -> LastIndexByte; an aliased bytes import keeps its qualifier
verbatim) and unwraps the needle IN PLACE: for []byte{X} it deletes the
slice scaffolding around the element, leaving X's source bytes
untouched (a named byte constant keeps its symbolic name); for
[]byte("z") it deletes the conversion scaffolding and appends [0],
reusing the string literal token byte-for-byte — no re-escaping is ever
performed, so the byte value is exact whatever the source spelling
(escape sequences and raw backquoted literals included), and the
compiler constant-folds "z"[0] to an immediate byte. The callee is
pinned by type information to the standard library's package-level
bytes.Index/bytes.LastIndex — a shadowed bytes identifier or a
same-named method never matches. When the conversion's operand is a
named string constant rather than a direct literal, the report is
advisory-only, because spelling out a copy of its value would discard
the symbolic name — index the constant itself (c[0]) when rewriting by
hand. A comment inside the deleted slice scaffolding likewise withholds
the fix (advisory) rather than destroying the comment. The fix touches
no imports (bytes stays imported).`,
		Before: `i := bytes.Index(b, []byte{'/'})
j := bytes.LastIndex(b, []byte("z"))`,
		After: `i := bytes.IndexByte(b, '/')
j := bytes.LastIndexByte(b, "z"[0])`,
		MeasuredWin: `BenchmarkPS5013 (61-byte haystack, needle in the last
field, Apple M2 Pro, go1.26): bytes.Index(b, []byte{'z'}) 4.33 ns/op ->
bytes.IndexByte(b, 'z') 3.30 ns/op (~1.3x), and
bytes.LastIndex(b, []byte("z")) 2.19 ns/op ->
bytes.LastIndexByte(b, "z"[0]) 0.63 ns/op (~3.4x). 0 B/op and
0 allocs/op on every side (escape analysis stack-allocates the
one-element needle): the win is pure instruction count — the
needle-length dispatch, the sep[0] load, and the slice construction
all disappear. The "z"[0] spelling is free — the compiler folds it to
a byte constant.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5013",
		Doc:  "bytes.Index/LastIndex of a statically one-byte needle instead of the direct IndexByte/LastIndexByte scan",
		Run:  runPS5013,
	},
})

func runPS5013(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5013Match(pass, call)
			if m == nil {
				return true
			}
			msg := "bytes." + m.fn + " of the one-byte needle " + m.argText +
				" builds a throwaway slice and runs the generic substring search; bytes." +
				m.fn + "Byte(b, " + m.afterText +
				") takes the same byte directly — identical index for every input"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			switch {
			case m.constOnly:
				// The conversion operand is a named constant or constant
				// expression: spelling out a copy of its value would discard
				// the symbolic name — advisory only, index the constant by
				// hand.
				diag.Message = msg + "; the needle is a constant expression, not a literal — rewrite to bytes." + m.fn + "Byte by hand"
			case ps5013CommentInSpans(f, m.del):
				// A comment sits inside the slice scaffolding the fix would
				// delete: withhold the fix rather than destroy the comment.
				diag.Message = msg + "; a comment inside the needle's slice syntax withholds the automatic fix — rewrite by hand"
			default:
				// Rename the callee identifier and unwrap the needle in
				// place: only scaffolding around the kept token is deleted,
				// so the element / literal source bytes carry over verbatim
				// and no re-escaping ever happens.
				edits := []analysis.TextEdit{
					{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte(m.fn + "Byte")},
				}
				for _, d := range m.del {
					edits = append(edits, analysis.TextEdit{Pos: d.pos, End: d.end, NewText: []byte(d.repl)})
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace " + m.fn + " with " + m.fn + "Byte",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps5013Span is a source range the fix rewrites (repl is empty for a pure
// deletion of needle scaffolding).
type ps5013Span struct {
	pos, end token.Pos
	repl     string
}

// ps5013Site is a matched bytes.Index/LastIndex call with a statically
// one-byte needle: the selector to rewrite, the function name, the
// scaffolding spans to delete/replace (empty when constOnly), the original
// needle's source text and the replacement argument's text for the
// message, and whether the site is advisory because the needle is a
// non-literal constant conversion.
type ps5013Site struct {
	sel       *ast.SelectorExpr
	fn        string
	del       []ps5013Span
	argText   string
	afterText string
	constOnly bool
}

// ps5013Match matches a call to the standard library's package-level
// bytes.Index or bytes.LastIndex whose second argument is statically a
// one-byte needle: a one-element, unkeyed byte-slice composite literal, or
// a byte-slice conversion of a constant string of decoded byte-length
// exactly 1. Type information pins the callee (a shadowed bytes
// identifier, a same-named method, or a third-party package never
// matches) and the needle's slice-of-byte type; the constant's DECODED
// byte length is measured — so "é" (two bytes) is out and "\xff" (one raw
// byte) is in, matching the byte-wise semantics of the rewrite exactly.
func ps5013Match(pass *analysis.Pass, call *ast.CallExpr) *ps5013Site {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
		return nil
	}
	switch fn.Name() {
	case "Index", "LastIndex":
	default:
		// IndexAny/LastIndexAny take a SET needle — a different pattern.
		return nil
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil
	}
	arg := ps2108Unparen(call.Args[1])

	// Shape A: a one-element, unkeyed composite literal []byte{X}. The
	// composite-literal typing rule makes X byte-assignable, so it carries
	// over verbatim as IndexByte's byte argument — a named byte constant
	// keeps its symbolic name, and side effects run once in the same order.
	if lit, isLit := arg.(*ast.CompositeLit); isLit {
		if !ps5013ByteSlice(pass.TypesInfo.TypeOf(lit)) || len(lit.Elts) != 1 {
			return nil
		}
		elt := lit.Elts[0]
		if _, keyed := elt.(*ast.KeyValueExpr); keyed {
			// []byte{0: 'x'} is length 1 too, but the keyed spelling is a
			// deliberate index statement — out of scope.
			return nil
		}
		argText, okText := ps5004ExprText(lit)
		if !okText {
			return nil
		}
		afterText, okText := ps5004ExprText(elt)
		if !okText {
			return nil
		}
		return &ps5013Site{
			sel: sel, fn: fn.Name(),
			del: []ps5013Span{
				{pos: lit.Pos(), end: elt.Pos()}, // "[]byte{"
				{pos: elt.End(), end: lit.End()}, // "}" (and any trailing comma)
			},
			argText: argText, afterText: afterText,
		}
	}

	// Shape B: a conversion []byte(C) of a constant string of decoded
	// byte-length exactly 1.
	conv, isConv := arg.(*ast.CallExpr)
	if !isConv || len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil
	}
	if tv, found := pass.TypesInfo.Types[conv.Fun]; !found || !tv.IsType() || !ps5013ByteSlice(tv.Type) {
		return nil
	}
	operand := ps2108Unparen(conv.Args[0])
	tv, found := pass.TypesInfo.Types[operand]
	if !found || tv.Value == nil || tv.Value.Kind() != constant.String {
		return nil
	}
	if len(constant.StringVal(tv.Value)) != 1 {
		return nil
	}
	argText, okText := ps5004ExprText(arg)
	if !okText {
		return nil
	}
	strLit, _ := operand.(*ast.BasicLit)
	if strLit != nil && strLit.Kind != token.STRING {
		strLit = nil
	}
	if strLit == nil {
		// A named constant or constant expression, not a direct literal:
		// advisory only — index the constant itself when rewriting by hand.
		operandText, okText := ps5004ExprText(operand)
		if !okText {
			return nil
		}
		return &ps5013Site{sel: sel, fn: fn.Name(), argText: argText, afterText: operandText + "[0]", constOnly: true}
	}
	return &ps5013Site{
		sel: sel, fn: fn.Name(),
		del: []ps5013Span{
			{pos: arg.Pos(), end: strLit.Pos()},              // "[]byte("
			{pos: strLit.End(), end: arg.End(), repl: "[0]"}, // ")" -> "[0]"
		},
		argText: argText, afterText: strLit.Value + "[0]",
	}
}

// ps5013ByteSlice reports whether t's underlying type is a slice of byte
// (an alias or defined type with underlying []byte is still assignable to
// bytes.Index's parameter, and its one-byte content drives the identical
// byte-wise search, so the rewrite is unaffected by the spelling).
func ps5013ByteSlice(t types.Type) bool {
	if t == nil {
		return false
	}
	s, ok := t.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	b, ok := s.Elem().Underlying().(*types.Basic)
	return ok && b.Kind() == types.Uint8
}

// ps5013CommentInSpans reports whether a comment overlaps any of the
// scaffolding spans the fix would delete.
func ps5013CommentInSpans(f *ast.File, spans []ps5013Span) bool {
	for _, cg := range f.Comments {
		for _, d := range spans {
			if cg.End() > d.pos && cg.Pos() < d.end {
				return true
			}
		}
	}
	return false
}
