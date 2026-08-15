package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5014 reports bytes.Contains(b, []byte{c}) / bytes.Contains(b, []byte("z"))
// where the needle is statically one byte: the call constructs a throwaway
// one-element slice and routes a single byte through the generic
// substring-search dispatch, while bytes.IndexByte(b, c) >= 0 answers the
// identical membership question with the direct byte scan. The membership
// (boolean) sibling of PS5013.
var PS5014 = register(&lint.Check{
	ID:       "PS5014",
	Category: "arith",
	Slug:     "bytes-contains-single-byte-needle",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "bytes.Contains of a one-byte needle builds a slice and runs the substring machinery; IndexByte >= 0 answers the same membership question directly",
		Text: `bytes.Contains(b, sep) is defined as bytes.Index(b, sep) >= 0.
For a one-byte needle that Index call still pays its per-call dispatch
over the needle length (empty? one byte? short? bytealg cutover?)
before it discovers len(sep) == 1 and delegates to IndexByte(s, sep[0])
— and on top of that dispatch, the CALLER constructs a throwaway
one-element []byte for every call ([]byte{'\n'} or []byte("=")) that
the byte form never needs. bytes.IndexByte(b, '\n') >= 0 skips straight
to the SIMD byte scan with no needle construction and no dispatch.
Escape analysis keeps the one-element slice on the stack, so no side
allocates — the win is pure instruction count, in exactly the parsing
and filtering loops where membership probes cluster. This is the
membership (boolean) sibling of PS5013 (the index form).

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
bytes.Contains(b, empty) is always true (Index returns 0) — a constant
IndexByte cannot express. Multi-element and keyed composite literals
and needles that are not statically one byte are out of scope, exactly
as in PS5013.

The rewrite is BIT-IDENTICAL on every input. For len(sep) == 1,
bytes.Contains(b, sep) == (bytes.Index(b, sep) >= 0) ==
(bytes.IndexByte(b, sep[0]) >= 0): both paths perform the identical
raw byte-wise search — no rune decoding, no case folding, no UTF-8
validation — so the returned bool is equal for every haystack: empty
or nil, needle absent, needle at either end, repeated needles, NUL
bytes, and invalid UTF-8 (pinned by the equivalence suite over all 256
needle bytes crossed with adversarial and randomized haystacks). The
haystack passes through byte-verbatim, both arguments are evaluated
exactly once in the original order, and the result is a plain untyped
bool comparison — assignable everywhere the original bool was.

The automatic fix renames the callee (Contains -> IndexByte; an
aliased bytes import keeps its qualifier verbatim), unwraps the needle
IN PLACE exactly like PS5013 (for []byte{X} the slice scaffolding
around the element is deleted, X's source bytes untouched; for
[]byte("z") the conversion scaffolding is deleted and [0] appended,
reusing the literal token byte-for-byte with no re-escaping), and
appends the comparison. The introduced >= 0 binds tighter than && / ||
and than a leading !, so plain boolean positions need no extra
syntax; two contexts get dedicated handling:

  - !bytes.Contains(...) absorbs the negation into the comparison:
    bytes.IndexByte(...) < 0 — !(x >= 0) == (x < 0) for every int;
  - when the replaced expression is itself an operand of a comparison
    (x == bytes.Contains(...)) or of another !, the replacement is
    parenthesized — (bytes.IndexByte(...) >= 0) — because == and >=
    share a precedence level and would otherwise chain illegally.

A go or defer statement requires a call expression, so a comparison
cannot be spliced there — such a site is reported advisory-only. As in
PS5013, a conversion of a named string constant is advisory-only
(spelling out a copy of its value would discard the symbolic name —
index the constant itself, c[0], by hand), and a comment inside the
slice scaffolding the fix would delete withholds the fix rather than
destroying the comment. The fix touches no imports (bytes stays
imported).`,
		Before: `found := bytes.Contains(buf, []byte{'\n'})
if !bytes.Contains(line, []byte("=")) {`,
		After: `found := bytes.IndexByte(buf, '\n') >= 0
if bytes.IndexByte(line, "="[0]) < 0 {`,
		MeasuredWin: `BenchmarkPS5014 (61-byte haystack, needle in the last
field, Apple M2 Pro, go1.26): bytes.Contains(b, []byte{'z'}) 4.03 ns/op
-> bytes.IndexByte(b, 'z') >= 0 3.35 ns/op (~1.2x), and
!bytes.Contains(b, []byte("=")) 3.35 ns/op ->
bytes.IndexByte(b, "="[0]) < 0 2.15 ns/op (~1.6x). 0 B/op and
0 allocs/op on every side (escape analysis stack-allocates the
one-element needle): the win is pure instruction count — the needle
construction, the needle-length dispatch, and the sep[0] load all
disappear. The "="[0] spelling is free — the compiler folds it to a
byte constant.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5014",
		Doc:  "bytes.Contains of a statically one-byte needle instead of the direct bytes.IndexByte comparison",
		Run:  runPS5014,
	},
})

func runPS5014(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5014Match(pass, call)
			if m == nil {
				return true
			}

			// Context bookkeeping for the introduced comparison operator.
			// A direct leading ! is absorbed into the comparison
			// (!Contains -> IndexByte < 0); the resulting expression then
			// needs parentheses when ITS parent is a comparison operand or
			// another ! (>= and == share a precedence level and would chain
			// illegally; ! binds tighter than >=), while plain boolean
			// positions — if/for conditions, && / || operands, assignments,
			// returns, call arguments — are safe bare. A go/defer statement
			// requires a call expression, so no comparison can be spliced
			// there at all.
			op := ">= 0"
			var flip *ast.UnaryExpr
			parentIdx := len(stack) - 1
			if parentIdx >= 0 {
				if u, isUnary := stack[parentIdx].(*ast.UnaryExpr); isUnary && u.Op == token.NOT {
					flip = u
					op = "< 0"
					parentIdx--
				}
			}
			var parent ast.Node
			if parentIdx >= 0 {
				parent = stack[parentIdx]
			}
			goDefer := false
			needParens := false
			switch p := parent.(type) {
			case *ast.GoStmt:
				goDefer = flip == nil && p.Call == call
			case *ast.DeferStmt:
				goDefer = flip == nil && p.Call == call
			case *ast.BinaryExpr:
				needParens = p.Op != token.LAND && p.Op != token.LOR
			case *ast.UnaryExpr:
				// Only reachable as the OUTER ! of a !!Contains chain (any
				// other unary op on a bool call does not compile): the
				// absorbed comparison must be parenthesized under it.
				needParens = true
			}

			repl := "bytes.IndexByte(b, " + m.afterText + ") " + op
			if needParens {
				repl = "(" + repl + ")"
			}
			msg := "bytes.Contains of the one-byte needle " + m.argText +
				" builds a throwaway slice and runs the generic substring search; " +
				repl + " answers the same membership question directly — identical boolean for every input"

			diagPos, diagEnd := call.Pos(), call.End()
			commentSpans := m.del
			if flip != nil {
				diagPos = flip.Pos()
				commentSpans = append([]ps5013Span{{pos: flip.OpPos, end: call.Pos()}}, m.del...)
			}
			diag := analysis.Diagnostic{
				Pos:     diagPos,
				End:     diagEnd,
				Message: msg,
			}
			switch {
			case m.constOnly:
				// The conversion operand is a named constant or constant
				// expression: spelling out a copy of its value would discard
				// the symbolic name — advisory only, index the constant by
				// hand.
				diag.Message = msg + "; the needle is a constant expression, not a literal — rewrite to bytes.IndexByte by hand"
			case goDefer:
				// go/defer require a call expression; a comparison cannot be
				// spliced into that position.
				diag.Message = msg + "; a go/defer statement requires a call expression, so the comparison cannot be spliced here — rewrite by hand"
			case ps5013CommentInSpans(f, commentSpans):
				// A comment sits inside syntax the fix would delete (the
				// slice scaffolding, or between ! and the call): withhold
				// the fix rather than destroy the comment.
				diag.Message = msg + "; a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand"
			default:
				// Rename the callee identifier, unwrap the needle in place
				// (only scaffolding around the kept token is deleted, so the
				// element / literal source bytes carry over verbatim and no
				// re-escaping ever happens), fold a leading ! into the
				// comparison direction, and append the comparison —
				// parenthesized when the surrounding operator requires it.
				open, closing := "", ""
				if needParens {
					open, closing = "(", ")"
				}
				var edits []analysis.TextEdit
				if flip != nil {
					edits = append(edits, analysis.TextEdit{Pos: flip.OpPos, End: call.Pos(), NewText: []byte(open)})
				} else if open != "" {
					edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: call.Pos(), NewText: []byte(open)})
				}
				edits = append(edits, analysis.TextEdit{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte("IndexByte")})
				for _, d := range m.del {
					edits = append(edits, analysis.TextEdit{Pos: d.pos, End: d.end, NewText: []byte(d.repl)})
				}
				edits = append(edits, analysis.TextEdit{Pos: call.End(), End: call.End(), NewText: []byte(" " + op + closing)})
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace Contains with IndexByte " + op,
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps5014Site is a matched bytes.Contains call with a statically one-byte
// needle: the selector to rewrite, the scaffolding spans to delete/replace
// (empty when constOnly), the original needle's source text and the
// replacement byte argument's text for the message, and whether the site
// is advisory because the needle is a non-literal constant conversion.
type ps5014Site struct {
	sel       *ast.SelectorExpr
	del       []ps5013Span
	argText   string
	afterText string
	constOnly bool
}

// ps5014Match matches a call to the standard library's package-level
// bytes.Contains whose second argument is statically a one-byte needle: a
// one-element, unkeyed byte-slice composite literal, or a byte-slice
// conversion of a constant string of decoded byte-length exactly 1. Type
// information pins the callee (a shadowed bytes identifier, a same-named
// method, or a third-party package never matches) and the needle's
// slice-of-byte type; the constant's DECODED byte length is measured — so
// "é" (two bytes) is out and "\xff" (one raw byte) is in, matching the
// byte-wise semantics of the rewrite exactly. The needle shapes and spans
// mirror PS5013's (the index form of the same rewrite).
func ps5014Match(pass *analysis.Pass, call *ast.CallExpr) *ps5014Site {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" || fn.Name() != "Contains" {
		// ContainsAny takes a SET needle and ContainsRune/ContainsFunc a
		// rune predicate — different patterns with different semantics.
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
		return &ps5014Site{
			sel: sel,
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
		return &ps5014Site{sel: sel, argText: argText, afterText: operandText + "[0]", constOnly: true}
	}
	return &ps5014Site{
		sel: sel,
		del: []ps5013Span{
			{pos: arg.Pos(), end: strLit.Pos()},              // "[]byte("
			{pos: strLit.End(), end: arg.End(), repl: "[0]"}, // ")" -> "[0]"
		},
		argText: argText, afterText: strLit.Value + "[0]",
	}
}
