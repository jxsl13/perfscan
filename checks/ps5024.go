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

// PS5024 reports strings.ContainsRune(s, 'x') where the rune is a
// compile-time constant in the ASCII range [0, 0x80): ContainsRune is
// IndexRune(s, r) >= 0, and for an ASCII r IndexRune immediately
// delegates to IndexByte(s, byte(r)) — two non-inlined wrapper frames
// before the byte scan. strings.IndexByte(s, 'x') >= 0 answers the
// identical membership question directly. The RUNE-needle sibling of
// PS5016 (Contains of a one-byte needle) and PS5022 (ContainsAny of a
// one-ASCII-byte cutset).
var PS5024 = register(&lint.Check{
	ID:       "PS5024",
	Category: "arith",
	Slug:     "containsrune-const-ascii-rune",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.ContainsRune of a constant ASCII rune chains two wrapper frames before the byte scan; IndexByte >= 0 answers the same membership question directly",
		Text: `strings.ContainsRune(s, r) is defined as
strings.IndexRune(s, r) >= 0, and IndexRune, for 0 <= r <
utf8.RuneSelf, immediately delegates to IndexByte(s, byte(r)). Neither
ContainsRune nor IndexRune is inlined, so an ASCII membership probe
pays two wrapper call frames plus IndexRune's rune-class dispatch
(ASCII? RuneError? invalid? multi-byte?) on every call before reaching
the SIMD single-byte scan. strings.IndexByte(s, 'x') >= 0 skips
straight to that scan: no ContainsRune frame, no IndexRune frame, no
rune-class branch. Neither form allocates — the win is pure
instruction count, in exactly the parsing, filtering and splitting
loops where membership probes cluster. This is the RUNE-needle sibling
of PS5016 (strings.Contains of a one-byte needle) and PS5022
(ContainsAny of a one-ASCII-byte cutset), which deliberately leave
ContainsRune out.

Only a needle that is a compile-time constant rune with value in
[0, 0x80) — i.e. below utf8.RuneSelf — is matched. The bound is
load-bearing on both sides:

  - a constant rune >= 0x80 takes a DIFFERENT IndexRune branch:
    utf8.RuneError (U+FFFD) searches for the first INVALID-UTF-8
    position, an invalid rune (surrogates, > MaxRune) is a constant
    false, and any other multi-byte rune is a genuine substring search
    over its UTF-8 encoding — none of which IndexByte(s, byte(r)) with
    its truncated byte can express. Those runes are excluded entirely
    (not even advisory: no byte-search rewrite exists for them);
  - a negative constant rune is a constant false via the same
    !ValidRune branch and is excluded for the same reason. The
    equivalence suite pins concrete divergence witnesses for all three
    shapes.

Under that constraint the identity is exact for ALL inputs:
ContainsRune(s, r) == (IndexRune(s, r) >= 0) ==
(IndexByte(s, byte(r)) >= 0), a pure byte scan with no rune decoding,
no UTF-8 validation, and no case folding — so the returned bool is
equal for every haystack: empty, needle absent, needle at either end,
repeated needles, NUL bytes, and invalid UTF-8 (pinned by the
equivalence suite over all 128 ASCII needles crossed with adversarial
and randomized haystacks). The haystack expression passes through
untouched, evaluated exactly once in the same position, and the callee
is pinned by type information — a shadowed strings identifier or a
same-named method never matches.

The automatic fix renames the callee (ContainsRune -> IndexByte; an
aliased strings import keeps its qualifier verbatim, and IndexByte
lives in the same package, so the import can never be orphaned) and
keeps the ORIGINAL needle literal byte-for-byte: a rune literal such
as 'x', '\t' or '\x00' — and an integer literal such as 47 —
is an UNTYPED constant, and under the < 0x80 gate it is representable
as byte, so it is passed to IndexByte's byte parameter verbatim with
no re-escaping and no byte(...) wrapper (the compiler folds it to an
immediate byte either way). The rewrite appends the comparison; the
introduced >= 0 binds tighter than && / || and than a leading !, so
plain boolean positions need no extra syntax, and two contexts get
dedicated handling:

  - !strings.ContainsRune(...) absorbs the negation into the
    comparison: strings.IndexByte(...) < 0 — !(x >= 0) == (x < 0) for
    every int;
  - when the replaced expression is itself an operand of a comparison
    (x == strings.ContainsRune(...)) or of another !, the replacement
    is parenthesized — (strings.IndexByte(...) >= 0) — because == and
    >= share a precedence level and would otherwise chain illegally.

A go or defer statement requires a call expression, so the comparison
cannot be spliced there — such a site is reported advisory-only, and
so is a bare call STATEMENT (legal Go that discards the bool), because
a comparison cannot stand alone as a statement. A named constant or
constant expression needle is advisory-only, because spelling out a
copy of its value would discard the symbolic name — and a TYPED rune
constant additionally needs an explicit byte(c) conversion the fix
will not synthesize; write strings.IndexByte(s, byte(c)) >= 0 by hand.
A comment between a leading ! and the call the absorption would fold
away withholds the fix rather than destroying the comment.
bytes.ContainsRune and strings.IndexRune are separate patterns, out of
this check's scope. The fix touches no imports (strings stays
imported).`,
		Before: `if strings.ContainsRune(s, '/') { ... }
found := !strings.ContainsRune(line, '=')`,
		After: `if strings.IndexByte(s, '/') >= 0 { ... }
found := strings.IndexByte(line, '=') < 0`,
		MeasuredWin: `BenchmarkPS5024 (61-byte haystack, needle in the last
field, Apple M2 Pro, go1.26): strings.ContainsRune(s, 'z') 3.61 ns/op
-> strings.IndexByte(s, 'z') >= 0 3.04 ns/op (~1.2x), and
!strings.ContainsRune(s, '=') 2.54 ns/op ->
strings.IndexByte(s, '=') < 0 2.01 ns/op (~1.25x). 0 B/op and
0 allocs/op on every side: the win is pure instruction count — the
ContainsRune and IndexRune wrapper frames and the rune-class dispatch
disappear. The untyped rune literal is passed to IndexByte verbatim —
the compiler folds it to a byte constant.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5024",
		Doc:  "strings.ContainsRune of a constant ASCII rune instead of the direct strings.IndexByte comparison",
		Run:  runPS5024,
	},
})

func runPS5024(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5024Match(pass, call)
			if m == nil {
				return true
			}

			// Context bookkeeping for the introduced comparison operator,
			// exactly as in PS5016/PS5022 (the substring/cutset siblings).
			// A direct leading ! is absorbed into the comparison
			// (!ContainsRune -> IndexByte < 0); the resulting expression
			// then needs parentheses when ITS parent is a comparison
			// operand or another ! (>= and == share a precedence level and
			// would chain illegally; ! binds tighter than >=), while plain
			// boolean positions — if/for conditions, && / || operands,
			// assignments, returns, call arguments — are safe bare. A
			// go/defer statement requires a call expression, so no
			// comparison can be spliced there.
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
				// Only reachable as the OUTER ! of a !!ContainsRune chain
				// (any other unary op on a bool call does not compile): the
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

			repl := "strings.IndexByte(s, " + m.argText + ") " + op
			if needParens {
				repl = "(" + repl + ")"
			}
			msg := "strings.ContainsRune of the constant ASCII rune " + m.argText +
				" chains two wrapper frames (ContainsRune -> IndexRune) before the byte scan; " + repl +
				" answers the same membership question directly — identical boolean for every input"

			diagPos, diagEnd := call.Pos(), call.End()
			var commentSpans []ps5013Span
			if flip != nil {
				// The only source the fix deletes is the ! and any trivia
				// between it and the call — the needle literal is left
				// untouched, so no other span can hold a comment.
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
				// A named constant or constant expression: spelling out a
				// copy of its value would discard the symbolic name, and a
				// TYPED rune constant additionally needs an explicit
				// byte(c) conversion — advisory only.
				diag.Message = msg + "; the needle is a constant expression, not a rune literal — rewrite to strings.IndexByte with a byte conversion by hand"
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
				// Rename the callee identifier, keep the needle literal
				// verbatim (an untyped rune/int constant below 0x80 is
				// representable as byte, so it passes to IndexByte's byte
				// parameter unchanged), fold a leading ! into the
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
				edits = append(edits,
					analysis.TextEdit{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte("IndexByte")},
					analysis.TextEdit{Pos: call.End(), End: call.End(), NewText: []byte(" " + op + closing)},
				)
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace ContainsRune with IndexByte " + op,
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps5024Site is a matched strings.ContainsRune call with a constant ASCII
// rune needle: the selector to rewrite, the needle's literal token (nil
// when the constant is not a direct rune or integer literal — advisory
// only), and the needle's source text for the message.
type ps5024Site struct {
	sel     *ast.SelectorExpr
	lit     *ast.BasicLit
	argText string
}

// ps5024Match matches a call to the standard library's package-level
// strings.ContainsRune whose second argument is a compile-time constant
// rune with value in [0, 0x80). Type information pins the callee (a
// shadowed strings identifier, a same-named method, or a third-party
// package never matches). The bound is load-bearing on both sides: a
// constant >= 0x80 takes a different IndexRune branch (the RuneError
// invalid-UTF-8 search, the constant-false invalid-rune branch, or a
// genuine multi-byte substring search) and a negative constant is a
// constant false — no byte-search rewrite exists for any of them, so
// such needles never match at all. The fix additionally requires a
// direct rune or integer LITERAL: only an untyped literal is guaranteed
// assignable to IndexByte's byte parameter verbatim, and a named
// constant keeps its symbolic name (advisory instead).
// bytes.ContainsRune and strings.IndexRune are separate patterns;
// Contains/ContainsAny of string needles are PS5016/PS5022's territory.
func ps5024Match(pass *analysis.Pass, call *ast.CallExpr) *ps5024Site {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "strings" || fn.Name() != "ContainsRune" {
		return nil
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil
	}
	arg := ps2108Unparen(call.Args[1])
	tv, found := pass.TypesInfo.Types[arg]
	if !found || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return nil
	}
	v, exact := constant.Int64Val(tv.Value)
	if !exact || v < 0 || v >= 0x80 {
		// Non-ASCII, RuneError, invalid and negative runes all take
		// IndexRune branches a truncated IndexByte cannot express.
		return nil
	}
	argText, ok := ps5004ExprText(arg)
	if !ok {
		return nil
	}
	lit, _ := arg.(*ast.BasicLit)
	if lit != nil && lit.Kind != token.CHAR && lit.Kind != token.INT {
		lit = nil
	}
	return &ps5024Site{sel: sel, lit: lit, argText: argText}
}
