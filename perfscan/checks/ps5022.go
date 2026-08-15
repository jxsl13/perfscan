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

// PS5022 reports strings.IndexAny / strings.ContainsAny /
// bytes.IndexAny / bytes.ContainsAny whose cutset is a compile-time
// constant string of exactly one ASCII byte: for that cutset the "any of
// a set" machinery collapses to a plain byte search, but only after the
// call has paid the cutset-length dispatch and two more non-inlined call
// frames. strings.IndexByte / bytes.IndexByte take the byte directly.
// The SET-needle sibling of PS5007/PS5013 (Index/LastIndex) and
// PS5016/PS5014 (Contains), which deliberately leave IndexAny out.
var PS5022 = register(&lint.Check{
	ID:       "PS5022",
	Category: "arith",
	Slug:     "indexany-single-ascii-cutset",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "IndexAny/ContainsAny with a one-ASCII-byte cutset pays the set machinery for a plain byte search; IndexByte jumps straight to the scan",
		Text: `strings.IndexAny(s, chars) — and bytes.IndexAny, and the
ContainsAny twins, which are literally defined as IndexAny(...) >= 0 —
is a non-inlined library call that, for a one-character cutset, still
pays its full per-call dispatch before doing any real work: the
chars == "" empty check, the len(chars) == 1 branch, a rune(chars[0])
conversion and a utf8.RuneSelf comparison, and then a CALL to
IndexRune, which repeats its own r < RuneSelf range check and CALLS
IndexByte. strings.IndexByte(s, "/"[0]) jumps straight to the
SIMD/bytealg byte scan, removing two non-inlined call frames plus
several branches on every invocation. Neither form allocates — the win
is pure instruction count, in exactly the tokenizing and splitting
loops where these probes cluster. This is the SET-needle sibling of
PS5007/PS5013 (Index/LastIndex of a one-byte needle) and PS5016/PS5014
(Contains), which deliberately leave IndexAny out.

Only a cutset that is a compile-time constant string of byte-length
EXACTLY 1 whose single byte is ASCII (< 0x80) is matched. Both bounds
are load-bearing:

  - the length rule is bytes, not runes: "é" (two bytes of UTF-8) and
    "" (zero bytes) fall outside the len == 1 fast path and are never
    matched — a multi-character cutset is a genuine SET search that
    IndexByte cannot express, and IndexAny(s, "") is a constant -1
    (ContainsAny false);
  - the ASCII bound is STRICTER than the single-byte rule of
    PS5007/PS5016 and exists because IndexAny REMAPS a non-ASCII
    cutset byte: for len(chars) == 1 it sets r = rune(chars[0]) and,
    when r >= utf8.RuneSelf (e.g. "\xff" or "\x80"), replaces r with
    utf8.RuneError and searches for the first INVALID-UTF-8 position —
    which is NOT IndexByte(s, 0xff). Those cutsets are excluded
    entirely (not even advisory: no byte-search rewrite exists for
    them). The equivalence suite pins a concrete divergence witness.

Under those constraints the identity is exact for ALL inputs:
IndexAny's len(chars) == 1 path sets r = rune(chars[0]); since
r < utf8.RuneSelf it delegates to IndexRune(s, r), which for
r < RuneSelf delegates to IndexByte(s, byte(r)) — a pure byte scan
with no rune decoding, no UTF-8 validation, and no case folding — so
the returned index is bit-identical for every haystack: nil or empty
(both -1), needle absent, needle at either end, repeated needles, NUL
bytes, and invalid UTF-8 (bytes.IndexAny's extra len(s) == 1 fast path
agrees byte-for-byte too: an ASCII probe into a one-byte haystack is
the same byte comparison on both sides). ContainsAny(s, chars) is
defined as IndexAny(s, chars) >= 0, so the membership form inherits
the identity verbatim. The haystack expression passes through
untouched, evaluated exactly once in the same position, and the callee
is pinned by type information — a shadowed strings/bytes identifier or
a same-named method never matches.

The automatic fix renames the callee (IndexAny -> IndexByte,
ContainsAny -> IndexByte; an aliased strings/bytes import keeps its
qualifier verbatim, and IndexByte lives in the same package, so the
import can never be orphaned) and wraps the ORIGINAL cutset literal as
"/"[0] exactly like PS5007/PS5016 — reusing the literal token
byte-for-byte, so no re-escaping is ever performed and the byte value
is exact whatever the source spelling (escape sequences such as "\t"
or "\x00" and raw backquoted literals included; the compiler folds the
literal index to an immediate byte). The IndexAny form stays a call
returning the same int, so it drops in everywhere with no
parenthesization concerns. The ContainsAny form appends the
comparison; the introduced >= 0 binds tighter than && / || and than a
leading !, so plain boolean positions need no extra syntax, and two
contexts get dedicated handling:

  - !strings.ContainsAny(...) absorbs the negation into the
    comparison: strings.IndexByte(...) < 0 — !(x >= 0) == (x < 0) for
    every int;
  - when the replaced expression is itself an operand of a comparison
    (x == strings.ContainsAny(...)) or of another !, the replacement
    is parenthesized — (strings.IndexByte(...) >= 0) — because == and
    >= share a precedence level and would otherwise chain illegally.

A go or defer statement requires a call expression, so the ContainsAny
comparison cannot be spliced there — such a site is reported
advisory-only, and so is a bare ContainsAny call STATEMENT (legal Go
that discards the bool). As in PS5007/PS5016, a named constant or
constant expression cutset is advisory-only, because wrapping a
spelled-out copy of its value would discard the symbolic name — index
the constant itself (c[0]) when rewriting by hand. A comment between a
leading ! and the call the absorption would fold away withholds the
fix rather than destroying the comment. LastIndexAny is out of scope
(a separate backward-scan pattern). The fix touches no imports.`,
		Before: `i := strings.IndexAny(s, "/")
if bytes.ContainsAny(b, "=") { ... }`,
		After: `i := strings.IndexByte(s, "/"[0])
if bytes.IndexByte(b, "="[0]) >= 0 { ... }`,
		MeasuredWin: `BenchmarkPS5022 (61-byte haystack, needle in the last
field, Apple M2 Pro, go1.26): strings.IndexAny(s, "z") 4.52 ns/op ->
strings.IndexByte(s, "z"[0]) 3.07 ns/op (~1.5x);
bytes.IndexAny(b, "z") 5.32 ns/op -> bytes.IndexByte(b, "z"[0])
3.13 ns/op (~1.7x); strings.ContainsAny(s, "=") 3.59 ns/op ->
strings.IndexByte(s, "="[0]) >= 0 2.08 ns/op (~1.7x); and
!bytes.ContainsAny(b, "=") 4.19 ns/op ->
bytes.IndexByte(b, "="[0]) < 0 2.19 ns/op (~1.9x). 0 B/op and
0 allocs/op on every side: the win is pure instruction count — the
cutset-length dispatch, the rune conversion and RuneSelf checks, and
the IndexRune call frame all disappear. The "z"[0] spelling is free —
the compiler folds it to a byte constant.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5022",
		Doc:  "strings/bytes IndexAny/ContainsAny with a one-ASCII-byte constant cutset instead of the direct IndexByte scan",
		Run:  runPS5022,
	},
})

func runPS5022(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5022Match(pass, call)
			if m == nil {
				return true
			}
			if m.fn == "IndexAny" {
				ps5022ReportIndex(pass, call, m)
			} else {
				ps5022ReportContains(pass, f, call, stack, m)
			}
			return true
		})
	}
	return nil, nil
}

// ps5022ReportIndex handles the IndexAny form: the replacement is still a
// call returning the same int, so it drops into every syntactic position
// with no parenthesization, statement, or go/defer concerns. Both replaced
// spans are single tokens (the selected identifier and the string
// literal), so no comment can ever sit inside deleted syntax.
func ps5022ReportIndex(pass *analysis.Pass, call *ast.CallExpr, m *ps5022Site) {
	msg := m.pkg + ".IndexAny of the one-ASCII-byte cutset " + m.argText +
		" pays the cutset dispatch and two non-inlined call frames before the byte scan; " +
		m.pkg + ".IndexByte(" + m.hay + ", " + m.argText +
		"[0]) jumps straight to the same scan — identical index for every input"
	diag := analysis.Diagnostic{
		Pos:     call.Pos(),
		End:     call.End(),
		Message: msg,
	}
	if m.lit == nil {
		// A named constant or constant expression: wrapping a spelled-out
		// copy of its value would discard the symbolic name — advisory
		// only, index the constant by hand.
		diag.Message = msg + "; the cutset is a constant expression, not a string literal — rewrite to " + m.pkg + ".IndexByte by hand"
	} else {
		// Rename the callee identifier and replace the literal token with
		// ITSELF plus the [0] index — the literal's source bytes carry
		// over verbatim (lit.Value is the raw token), so no re-escaping
		// happens.
		diag.SuggestedFixes = []analysis.SuggestedFix{{
			Message: "replace IndexAny with IndexByte",
			TextEdits: []analysis.TextEdit{
				{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte("IndexByte")},
				{Pos: m.lit.Pos(), End: m.lit.End(), NewText: []byte(m.lit.Value + "[0]")},
			},
		}}
	}
	pass.Report(diag)
}

// ps5022ReportContains handles the ContainsAny form, which introduces a
// comparison: context bookkeeping exactly as in PS5016/PS5014. A direct
// leading ! is absorbed into the comparison (!ContainsAny ->
// IndexByte < 0); the resulting expression then needs parentheses when
// ITS parent is a comparison operand or another ! (>= and == share a
// precedence level and would chain illegally; ! binds tighter than >=),
// while plain boolean positions — if/for conditions, && / || operands,
// assignments, returns, call arguments — are safe bare. A go/defer
// statement requires a call expression, and a comparison cannot stand
// alone as a statement, so those positions are advisory-only.
func ps5022ReportContains(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, stack []ast.Node, m *ps5022Site) {
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
		// Only reachable as the OUTER ! of a !!ContainsAny chain (any
		// other unary op on a bool call does not compile): the absorbed
		// comparison must be parenthesized under it.
		needParens = true
	}
	// A bare call STATEMENT compiles (the bool is discarded), but a
	// comparison cannot stand alone as a statement — the rewrite would
	// not compile. Parens around the call keep it a legal statement, so
	// walk through them; a !call statement does not compile at all, so
	// flip is necessarily nil here.
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

	repl := m.pkg + ".IndexByte(" + m.hay + ", " + m.argText + "[0]) " + op
	if needParens {
		repl = "(" + repl + ")"
	}
	msg := m.pkg + ".ContainsAny of the one-ASCII-byte cutset " + m.argText +
		" routes a single byte through the cutset machinery; " + repl +
		" answers the same membership question directly — identical boolean for every input"

	diagPos, diagEnd := call.Pos(), call.End()
	var commentSpans []ps5013Span
	if flip != nil {
		// The only source the fix deletes is the ! and any trivia between
		// it and the call — the literal token itself is reused in place,
		// so no other span can hold a comment.
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
		// A named constant or constant expression: wrapping a spelled-out
		// copy of its value would discard the symbolic name — advisory
		// only, index the constant by hand.
		diag.Message = msg + "; the cutset is a constant expression, not a string literal — rewrite to " + m.pkg + ".IndexByte by hand"
	case goDefer:
		// go/defer require a call expression; a comparison cannot be
		// spliced into that position.
		diag.Message = msg + "; a go/defer statement requires a call expression, so the comparison cannot be spliced here — rewrite by hand"
	case stmtOnly:
		// The bool is discarded by a bare call statement; a comparison
		// cannot stand alone as a statement.
		diag.Message = msg + "; the call is a statement discarding its result — a comparison cannot stand alone as a statement, so no fix is offered — rewrite by hand"
	case ps5013CommentInSpans(f, commentSpans):
		// A comment sits between the ! and the call the absorption would
		// fold away: withhold the fix rather than destroy the comment.
		diag.Message = msg + "; a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand"
	default:
		// Rename the callee identifier, wrap the ORIGINAL literal token
		// as lit[0] (its source bytes carry over verbatim, so no
		// re-escaping ever happens), fold a leading ! into the comparison
		// direction, and append the comparison — parenthesized when the
		// surrounding operator requires it.
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
			Message:   "replace ContainsAny with IndexByte " + op,
			TextEdits: edits,
		}}
	}
	pass.Report(diag)
}

// ps5022Site is a matched strings/bytes IndexAny/ContainsAny call with a
// one-ASCII-byte constant cutset: the selector to rewrite, the package
// name (which doubles as the message qualifier — "strings" or "bytes"),
// the function name, the conventional haystack placeholder for the
// message ("s" or "b"), the cutset's literal token (nil when the constant
// is not a direct string literal — advisory only), and the cutset's
// source text for the message.
type ps5022Site struct {
	sel     *ast.SelectorExpr
	pkg     string
	fn      string
	hay     string
	lit     *ast.BasicLit
	argText string
}

// ps5022Match matches a call to the standard library's package-level
// strings.IndexAny, strings.ContainsAny, bytes.IndexAny or
// bytes.ContainsAny whose second argument (the cutset — a string in BOTH
// packages) is a compile-time constant string of byte-length exactly 1
// whose single byte is ASCII (< 0x80). Type information pins the callee
// (a shadowed strings/bytes identifier, a same-named method, or a
// third-party package never matches). The constant's DECODED byte length
// is measured — "é" (two bytes) is out — and the ASCII bound is
// load-bearing: for a single byte >= 0x80 IndexAny remaps the cutset to
// utf8.RuneError and searches for the first invalid-UTF-8 position, which
// IndexByte cannot express, so such cutsets never match at all.
func ps5022Match(pass *analysis.Pass, call *ast.CallExpr) *ps5022Site {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil {
		return nil
	}
	pkg := fn.Pkg().Path()
	if pkg != "strings" && pkg != "bytes" {
		return nil
	}
	switch fn.Name() {
	case "IndexAny", "ContainsAny":
	default:
		// Index/LastIndex/Contains of a one-byte SUBSTRING are
		// PS5007/PS5013/PS5016/PS5014's territory; LastIndexAny is a
		// separate backward-scan pattern.
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
	val := constant.StringVal(tv.Value)
	if len(val) != 1 || val[0] >= 0x80 {
		// Multi-byte and empty cutsets are genuine SET searches; a single
		// non-ASCII byte is remapped to utf8.RuneError by IndexAny — no
		// byte-search rewrite exists for either.
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
	hay := "s"
	if pkg == "bytes" {
		hay = "b"
	}
	return &ps5022Site{sel: sel, pkg: pkg, fn: fn.Name(), hay: hay, lit: lit, argText: argText}
}
