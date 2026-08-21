package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS5016 reports strings.Contains(s, "z") where the needle is statically
// one byte: Contains is Index >= 0, and Index pays its per-call
// needle-length dispatch before delegating a one-byte needle to
// IndexByte, while strings.IndexByte(s, 'z') >= 0 answers the identical
// membership question with the direct byte scan. The strings twin of
// PS5014 (bytes.Contains) and the membership (boolean) sibling of PS5007
// (strings.Index/LastIndex).
var PS5016 = register(&lint.Check{
	ID:       "PS5016",
	Category: "arith",
	Slug:     "strings-contains-single-byte-needle",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Contains of a one-byte needle runs the substring machinery; IndexByte >= 0 answers the same membership question directly",
		Text: `strings.Contains(s, substr) is defined as
strings.Index(s, substr) >= 0. For a one-byte needle that Index call
still pays two wrapper frames plus its per-call dispatch over the
needle length (empty? one byte? short? bytealg cutover?) before it
discovers len(substr) == 1 and delegates to IndexByte(s, substr[0]).
strings.IndexByte(s, "z"[0]) >= 0 skips straight to the SIMD
single-byte scan: no Contains frame, no Index frame, no length-class
branch. Neither form allocates (the needle is a string constant, not a
constructed slice) — the win is pure instruction count, in exactly the
parsing, filtering and splitting loops where membership probes
cluster. This is the strings twin of PS5014 (bytes.Contains of a
one-byte needle) and the membership (boolean) sibling of PS5007
(strings.Index/LastIndex of a one-byte literal).

Only a needle that is a compile-time constant string of byte-length
EXACTLY 1 is matched. The length rule is bytes, not runes: a
multi-byte literal such as "é" (two bytes of UTF-8) is out of scope,
while a raw single-byte escape such as "\xff" or "\x00" — even when it
is not valid UTF-8 — is in scope, because a one-byte needle is matched
byte-wise on both sides of the rewrite. The empty needle is excluded
by the same rule: strings.Contains(s, "") is always true (Index
returns 0) — a constant IndexByte cannot express.

The rewrite is BIT-IDENTICAL on every input. For len(substr) == 1,
strings.Contains(s, substr) == (strings.Index(s, substr) >= 0) ==
(strings.IndexByte(s, substr[0]) >= 0): both paths perform the
identical raw byte-wise search — no rune decoding, no case folding, no
UTF-8 validation — so the returned bool is equal for every haystack:
empty, needle absent, needle at either end, repeated needles, NUL
bytes, and invalid UTF-8 (pinned by the equivalence suite over all 256
needle bytes crossed with adversarial and randomized haystacks). The
haystack is already the plain predeclared string IndexByte takes —
no conversion, no named-type involvement, no method dispatch — and
both arguments are evaluated exactly once, in the original order. The
result is a plain untyped bool comparison, assignable everywhere the
original bool was, and neither form can panic.

The automatic fix renames the callee (Contains -> IndexByte; an
aliased strings import keeps its qualifier verbatim), wraps the
ORIGINAL needle literal as "z"[0] exactly like PS5007 — reusing the
literal token byte-for-byte, so no re-escaping is ever performed and
the byte value is exact whatever the source spelling (escape sequences
and raw backquoted literals included; the compiler folds the literal
index to an immediate byte, so codegen matches a hand-written byte
literal) — and appends the comparison. The introduced >= 0 binds
tighter than && / || and than a leading !, so plain boolean positions
need no extra syntax; two contexts get dedicated handling:

  - !strings.Contains(...) absorbs the negation into the comparison:
    strings.IndexByte(...) < 0 — !(x >= 0) == (x < 0) for every int;
  - when the replaced expression is itself an operand of a comparison
    (x == strings.Contains(...)) or of another !, the replacement is
    parenthesized — (strings.IndexByte(...) >= 0) — because == and >=
    share a precedence level and would otherwise chain illegally.

A go or defer statement requires a call expression, so a comparison
cannot be spliced there — such a site is reported advisory-only, and so
is a bare call STATEMENT (legal Go that discards the bool), because a
comparison cannot stand alone as a statement. As in
PS5007, a named constant or constant expression of byte-length 1 is
advisory-only, because wrapping a spelled-out copy of its value would
discard the symbolic name — index the constant itself (c[0]) when
rewriting by hand. A comment between a leading ! and the call the
absorption would fold away withholds the fix rather than destroying
the comment. The fix touches no imports (strings stays imported).`,
		Before: `if strings.Contains(s, "/") { ... }
found := !strings.Contains(line, "=")`,
		After: `if strings.IndexByte(s, "/"[0]) >= 0 { ... }
found := strings.IndexByte(line, "="[0]) < 0`,
		MeasuredWin: `BenchmarkPS5016 (61-byte haystack, needle in the last
field, Apple M2 Pro, go1.26): strings.Contains(s, "z") 3.80 ns/op ->
strings.IndexByte(s, "z"[0]) >= 0 3.04 ns/op (~1.25x), and
!strings.Contains(s, "=") 2.70 ns/op ->
strings.IndexByte(s, "="[0]) < 0 2.02 ns/op (~1.3x). 0 B/op and
0 allocs/op on every side: the win is pure instruction count — the
wrapper frames and the needle-length dispatch disappear. The "z"[0]
spelling is free — the compiler folds it to a byte constant.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5016",
		Doc:  "strings.Contains of a single-byte constant needle instead of the direct strings.IndexByte comparison",
		Run:  runPS5016,
	},
})

func runPS5016(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5016Match(pass, call)
			if m == nil {
				return true
			}

			// Context bookkeeping for the introduced comparison operator,
			// exactly as in PS5014 (the bytes twin). A direct leading ! is
			// absorbed into the comparison (!Contains -> IndexByte < 0);
			// the resulting expression then needs parentheses when ITS
			// parent is a comparison operand or another ! (>= and == share
			// a precedence level and would chain illegally; ! binds
			// tighter than >=), while plain boolean positions — if/for
			// conditions, && / || operands, assignments, returns, call
			// arguments — are safe bare. A go/defer statement requires a
			// call expression, so no comparison can be spliced there.
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
			// A bare call STATEMENT compiles (the bool is discarded), but a
			// comparison cannot stand alone as a statement — the rewrite
			// would not compile. Parens around the call keep it a legal
			// statement, so walk through them; a !call statement does not
			// compile at all, so flip is necessarily nil here.
			stmtOnly := false
			if flip == nil {
				stmtIdx := parentIdx
				for stmtIdx >= 0 {
					if _, isParen := stack[stmtIdx].(*ast.ParenExpr); isParen {
						stmtIdx--
						continue
					}
					break
				}
				if stmtIdx >= 0 {
					_, stmtOnly = stack[stmtIdx].(*ast.ExprStmt)
				}
			}

			repl := "strings.IndexByte(s, " + m.argText + "[0]) " + op
			if needParens {
				repl = "(" + repl + ")"
			}
			msg := "strings.Contains of the single-byte string " + m.argText +
				" runs the generic substring search; " + repl +
				" answers the same membership question directly — identical boolean for every input"

			diagPos, diagEnd := call.Pos(), call.End()
			var commentSpans []ps5013Span
			if flip != nil {
				// The only source the fix deletes is the ! and any
				// trivia between it and the call — the literal token
				// itself is reused in place, so no other span can hold a
				// comment.
				diagPos = flip.Pos()
				commentSpans = []ps5013Span{{pos: flip.OpPos, end: call.Pos()}}
			}
			diag := analysis.Diagnostic{
				Pos:     diagPos,
				End:     diagEnd,
				Message: msg,
			}
			switch {
			case m.lit == nil:
				// A named constant or constant expression: wrapping a
				// spelled-out copy of its value would discard the symbolic
				// name — advisory only, index the constant by hand.
				diag.Message = msg + "; the needle is a constant expression, not a string literal — rewrite to strings.IndexByte by hand"
			case goDefer:
				// go/defer require a call expression; a comparison cannot
				// be spliced into that position.
				diag.Message = msg + "; a go/defer statement requires a call expression, so the comparison cannot be spliced here — rewrite by hand"
			case stmtOnly:
				// The bool is discarded by a bare call statement; a
				// comparison cannot stand alone as a statement.
				diag.Message = msg + "; the call is a statement discarding its result — a comparison cannot stand alone as a statement, so no fix is offered — rewrite by hand"
			case ps5013CommentInSpans(f, commentSpans):
				// A comment sits between the ! and the call the absorption
				// would fold away: withhold the fix rather than destroy
				// the comment.
				diag.Message = msg + "; a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand"
			default:
				// Rename the callee identifier, wrap the ORIGINAL literal
				// token as lit[0] (its source bytes carry over verbatim,
				// so no re-escaping ever happens), fold a leading ! into
				// the comparison direction, and append the comparison —
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
				edits = append(edits,
					analysis.TextEdit{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte("IndexByte")},
					analysis.TextEdit{Pos: m.lit.Pos(), End: m.lit.End(), NewText: []byte(m.lit.Value + "[0]")},
					analysis.TextEdit{Pos: call.End(), End: call.End(), NewText: []byte(" " + op + closing)},
				)
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

// ps5016Site is a matched strings.Contains call with a one-byte constant
// needle: the selector to rewrite, the needle's literal token (nil when
// the constant is not a direct string literal — advisory only), and the
// needle's source text for the message.
type ps5016Site struct {
	sel     *ast.SelectorExpr
	lit     *ast.BasicLit
	argText string
}

// ps5016Match matches a call to the standard library's package-level
// strings.Contains whose second argument is a compile-time constant
// string of byte-length exactly 1. Type information pins the callee (a
// shadowed strings identifier, a same-named method, or a third-party
// package never matches), and the constant's DECODED byte length is
// measured — so "é" (two bytes) is out and "\xff" (one raw byte) is in,
// matching the byte-wise semantics of the rewrite exactly. The needle
// handling mirrors PS5007's (the index form of the same rewrite);
// ContainsAny takes a SET needle and ContainsRune/ContainsFunc a rune
// predicate — different patterns with different semantics.
func ps5016Match(pass *analysis.Pass, call *ast.CallExpr) *ps5016Site {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "strings" || fn.Name() != "Contains" {
		return nil
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil
	}
	arg := ps2108Unparen(call.Args[1])
	tv, found := pass.TypesInfo.Types[arg]
	if !found || tv.Value == nil || tv.Value.Kind() != constant.String {
		return nil
	}
	if len(constant.StringVal(tv.Value)) != 1 {
		return nil
	}
	argText, ok := ps5004ExprText(arg)
	if !ok {
		return nil
	}
	lit, _ := arg.(*ast.BasicLit)
	if lit != nil && lit.Kind != token.STRING {
		lit = nil
	}
	return &ps5016Site{sel: sel, lit: lit, argText: argText}
}
