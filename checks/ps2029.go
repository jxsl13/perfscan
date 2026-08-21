package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2029 reports len(strings.SplitN(s, sep, 2)) compared against the
// literal 1 or 2 to answer a membership question — "does s contain the
// separator?" — where the SplitN call allocates a two-slot []string
// (plus the piece headers) just to have its length compared and the
// slice thrown away. strings.Contains(s, sep) answers the identical
// boolean by scanning to the first occurrence with zero allocation.
// The bytes twin is matched the same way. Only fires when the
// separator is provably non-empty: for the empty separator the two
// expressions genuinely differ.
var PS2029 = register(&lint.Check{
	ID:       "PS2029",
	Category: "alloc",
	Slug:     "len-splitn-membership",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "len(strings.SplitN(s, sep, 2)) compared against 1 or 2 allocates the piece slice just to test for the separator; strings.Contains is allocation-free",
		Text: `strings.SplitN(s, sep, 2) allocates a two-slot []string (one
slice allocation plus up to two string headers) and scans to the first
separator — just so len(...) can answer a yes/no membership question.
strings.Contains(s, sep) answers the identical boolean with the same
single scan and ZERO allocation. For the bytes twin the win is larger:
the [][]byte and its two slice headers vanish. The compared slice is
provably discarded (only its len is consumed), so this is a pure
allocation-elimination win.

For a NON-EMPTY separator, SplitN(s, sep, 2) returns min(k+1, 2)
pieces, where k is the number of non-overlapping occurrences of sep:
the result length is exactly 2 when the separator occurs (k >= 1) and
exactly 1 when it does not (k == 0) — never 0 and never more than 2,
including the empty haystack (SplitN("", sep, 2) is [""], length 1,
and Contains("", sep) is false). Exactly eight comparison shapes are
therefore membership-equivalent, and only those are matched (with the
literal on either side of the operator):

	len(SplitN(s, sep, 2)) == 2  →  Contains(s, sep)
	len(SplitN(s, sep, 2)) >= 2  →  Contains(s, sep)
	len(SplitN(s, sep, 2)) >  1  →  Contains(s, sep)
	len(SplitN(s, sep, 2)) != 1  →  Contains(s, sep)
	len(SplitN(s, sep, 2)) == 1  →  !Contains(s, sep)
	len(SplitN(s, sep, 2)) <= 1  →  !Contains(s, sep)
	len(SplitN(s, sep, 2)) <  2  →  !Contains(s, sep)
	len(SplitN(s, sep, 2)) != 2  →  !Contains(s, sep)

The empty separator is the ONE case where the identity breaks:
SplitN(s, "", 2) rune-explodes and its length is min(2, rune count) —
it can be 0, and it tracks the rune count rather than membership —
while Contains(s, "") is always true. The check therefore only matches
a PROVABLY non-empty separator, with exactly PS2121's guard: for
strings the separator must be a compile-time constant (a string
literal or a named constant, resolved via go/types constant
evaluation) whose value is not ""; for bytes it must be a
[]byte("...") conversion of a non-empty constant string or a
[]byte{...} composite literal with at least one element. A variable
separator — even one that happens to be non-empty at run time — is
never reported.

The limit must be the LITERAL 2: any other limit (or a variable or
named constant holding 2) changes the length algebra and is never
matched. The outer len must be the predeclared builtin and the callee
is pinned with type information to the package-level strings.SplitN /
bytes.SplitN — a shadowed len or strings/bytes, or a local function or
method named SplitN, resolves elsewhere and is rejected. Only the
direct len(SplitN(...)) composition is matched: a SplitN result stored
in a variable first may have other consumers. SplitAfterN keeps the
separators but counts pieces the same way — it is still out of scope
here (use SplitN for membership); Split and SplitAfter have no limit
and are PS2121's territory.

The rewrite is BIT-IDENTICAL: both sides evaluate the haystack and the
separator exactly once, in the same order, with the same byte-wise
first-occurrence scan (Contains is Index >= 0, and SplitN's piece
count for n=2 is decided by the same Index call); neither side can
panic, bytes.Contains reads its arguments without mutating them, and
the discarded [][]byte cannot be aliased, so there is no backing-array
or nil-slice observability. The automatic fix keeps the haystack and
separator expressions byte-verbatim — same text, same evaluation order
— and rewrites only the scaffolding: the comparison and the len(, the
", 2" limit and the closing parentheses collapse into a (possibly
negated) Contains call. Contains lives in the same package as SplitN,
so an aliased qualifier is reused verbatim and the import can never be
orphaned. The replacement is a call (or a !-prefixed call, which binds
tighter than any binary operator), so every position that accepted the
comparison accepts it unchanged with no added parentheses.

Two needle shapes chain further, exactly as in PS5104: a strings
separator that is a direct ONE-BYTE string literal (PS5016's
Before-shape) and a bytes separator that is a one-element []byte{X}
composite or a []byte("z") conversion of a one-byte string literal
(PS5014's Before-shape). Emitting Contains there would make the next
-fix pass rewrite this fix's own output, so the fix emits the final
fixed point directly: strings.IndexByte(s, "="[0]) >= 0 (negated: < 0)
and bytes.IndexByte(b, X) >= 0, reusing the literal token
byte-for-byte. A named-constant separator keeps the plain Contains
spelling (PS5016/PS5014 report those advisory-only, so no churn).

Two guards keep the fix honest: the comparison is an untyped bool that
adopts whatever boolean type its context demands, while Contains
returns the basic type bool — a context that materialized a named bool
type is skipped entirely (the rewrite would not compile there); and a
comment inside the scaffolding the fix would delete withholds the fix
(the report stays advisory) rather than destroying the comment.

Comparisons that genuinely inspect the piece count are left alone:
len(SplitN(s, sep, 2)) == 0, > 2, >= 1, <= 2 and friends (those are
constant-foldable trivia, not membership tests), and any comparison
against a non-literal.`,
		Before: `if len(strings.SplitN(s, "=>", 2)) == 2 {
	// has separator
}`,
		After: `if strings.Contains(s, "=>") {
	// has separator
}`,
		MeasuredWin: `BenchmarkPS2029 (Apple M2 Pro, go1.26): a ~1.6 KB
line with the two-byte separator in the middle drops from 49.7 ns/op,
32 B/op, 1 alloc/op to 16.1 ns/op, 0 B/op, 0 allocs/op (~3.1x) — the
two-slot []string and its piece headers vanish while the scan itself
is the same first-occurrence Index. The bytes twin drops from
61.3 ns/op, 48 B/op, 1 alloc/op to 15.2 ns/op, 0 B/op, 0 allocs/op
(~4x; the [][]byte's two slice headers make the vanished allocation
larger). The win is pure allocation elimination on hot membership
probes, where the discarded slice otherwise churns the GC.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2029",
		Doc:  "len(strings.SplitN(s, sep, 2)) compared against 1 or 2 allocates the piece slice just to test for the separator; strings.Contains answers it with zero allocation",
		Run:  runPS2029,
	},
})

func runPS2029(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			switch bin.Op {
			case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			default:
				return true
			}
			m := ps2029Match(pass, bin)
			if m == nil {
				return true
			}
			// The comparison is an untyped bool and adopts whatever
			// boolean type its context demands (var b myBool = len(...) == 2);
			// Contains returns the basic type bool. Skip when the context
			// materialized a named bool type — the rewrite would not
			// compile there.
			if tv, ok := pass.TypesInfo.Types[bin]; ok {
				if b, isBasic := tv.Type.(*types.Basic); !isBasic || b.Info()&types.IsBoolean == 0 {
					return true
				}
			}
			edits, afterText, guards := ps2029Rewrite(pass, bin, m)
			if afterText == "" {
				return true
			}
			beforeText, okText := ps5004ExprText(bin)
			if !okText {
				return true
			}
			msg := beforeText + " allocates the up-to-two-piece slice just to test for the separator; " + afterText + " is the identical boolean with zero allocation"
			diag := analysis.Diagnostic{Pos: bin.Pos(), End: bin.End(), Message: msg}
			withheld := false
			for i := 0; i+1 < len(guards); i += 2 {
				if ps2111CommentIn(f, guards[i], guards[i+1]) {
					withheld = true
					break
				}
			}
			if withheld {
				// A comment sits inside the scaffolding the fix would
				// delete: withhold the fix rather than destroy the comment.
				diag.Message = msg + "; a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand"
			} else {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace with " + afterText,
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2029Site is a matched membership test: the verbatim haystack and
// separator expressions of the SplitN call, the (possibly aliased)
// package qualifier, the package path ("strings" or "bytes"), and
// whether the Contains rewrite must be negated.
type ps2029Site struct {
	haystack ast.Expr
	sep      ast.Expr
	qual     string
	pkg      string
	negate   bool
}

// ps2029Match matches a binary comparison whose one operand is
// len(strings.SplitN(s, sep, 2)) / len(bytes.SplitN(b, sep, 2)) — len
// the predeclared builtin, SplitN the package-level function pinned by
// type information, the limit the literal 2, and the separator provably
// non-empty — and whose other operand is the literal integer constant
// 1 or 2, in one of the eight membership-equivalent shapes. Both
// operand orders are matched.
func ps2029Match(pass *analysis.Pass, bin *ast.BinaryExpr) *ps2029Site {
	inner, qual, pkg, ok := ps2029LenSplitN(pass, bin.X)
	litOnLeft := false
	if !ok {
		inner, qual, pkg, ok = ps2029LenSplitN(pass, bin.Y)
		if !ok {
			return nil
		}
		litOnLeft = true
	}
	var litSide ast.Expr
	if litOnLeft {
		litSide = bin.X
	} else {
		litSide = bin.Y
	}
	v, isLit := ps2029OneTwoLit(pass, litSide)
	if !isLit {
		return nil
	}
	// Normalize to the call-on-left spelling: `2 == len(...)` means
	// `len(...) == 2`, so mirror the operator when the literal is on
	// the left, then classify against the eight membership shapes.
	op := bin.Op
	if litOnLeft {
		op = ps5104Mirror(op)
	}
	negate, ok := ps2029Membership(op, v)
	if !ok {
		return nil
	}
	return &ps2029Site{
		haystack: inner.Args[0],
		sep:      inner.Args[1],
		qual:     qual,
		pkg:      pkg,
		negate:   negate,
	}
}

// ps2029Membership classifies the normalized (call-on-left) comparison
// `len(SplitN(s, sep, 2)) <op> <lit>` against the eight membership
// shapes, reporting whether the Contains rewrite must be negated. The
// length is always 1 or 2 (non-empty separator, limit 2), so == 2,
// >= 2, > 1 and != 1 mean "contains" and == 1, <= 1, < 2 and != 2 mean
// "does not contain". Any other (op, literal) pair is constant-foldable
// trivia (>= 1, <= 2, > 2, < 1) and is not matched.
func ps2029Membership(op token.Token, v int64) (negate, ok bool) {
	switch {
	case op == token.EQL && v == 2, // len == 2
		op == token.GEQ && v == 2, // len >= 2
		op == token.GTR && v == 1, // len > 1
		op == token.NEQ && v == 1: // len != 1
		return false, true
	case op == token.EQL && v == 1, // len == 1
		op == token.LEQ && v == 1, // len <= 1
		op == token.LSS && v == 2, // len < 2
		op == token.NEQ && v == 2: // len != 2
		return true, true
	}
	return false, false
}

// ps2029LenSplitN matches e (modulo parentheses) as
// len(strings.SplitN(hay, sep, 2)) or len(bytes.SplitN(hay, sep, 2)),
// returning the SplitN call, the source spelling of the package
// qualifier (which reuses an import alias verbatim) and the package
// path. The len identifier must resolve to the predeclared builtin and
// the SplitN selector to the receiver-less package-level
// strings.SplitN / bytes.SplitN — a shadowed len or strings/bytes, or
// a local function, variable, field or method spelled SplitN, resolves
// elsewhere and is rejected. The limit must be the literal 2 and the
// separator provably non-empty (PS2121's guard); the call must feed
// the comparison DIRECTLY: a SplitN slice stored in a variable first
// may have other consumers and is out of scope.
func ps2029LenSplitN(pass *analysis.Pass, e ast.Expr) (inner *ast.CallExpr, qual, pkg string, ok bool) {
	lenCall, isCall := ps2108Unparen(e).(*ast.CallExpr)
	if !isCall || len(lenCall.Args) != 1 || lenCall.Ellipsis.IsValid() {
		return nil, "", "", false
	}
	lenIdent, isIdent := ps2108Unparen(lenCall.Fun).(*ast.Ident)
	if !isIdent || pass.TypesInfo.Uses[lenIdent] != types.Universe.Lookup("len") {
		return nil, "", "", false
	}
	call, isCall := ps2108Unparen(lenCall.Args[0]).(*ast.CallExpr)
	if !isCall || len(call.Args) != 3 || call.Ellipsis.IsValid() {
		return nil, "", "", false
	}
	sel, isSel := ps2108Unparen(call.Fun).(*ast.SelectorExpr)
	if !isSel {
		return nil, "", "", false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fn.Name() != "SplitN" || fn.Pkg() == nil {
		return nil, "", "", false
	}
	pkg = fn.Pkg().Path()
	if pkg != "strings" && pkg != "bytes" {
		return nil, "", "", false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, "", "", false
	}
	pkgIdent, isIdent := ps2108Unparen(sel.X).(*ast.Ident)
	if !isIdent {
		return nil, "", "", false
	}
	// The limit must be the LITERAL 2: any other limit changes the
	// length algebra, and a variable or named constant holding 2 is
	// deliberately not matched.
	lim, isLim := ps2108Unparen(call.Args[2]).(*ast.BasicLit)
	if !isLim || lim.Kind != token.INT {
		return nil, "", "", false
	}
	ltv, okTv := pass.TypesInfo.Types[lim]
	if !okTv || ltv.Value == nil || ltv.Value.Kind() != constant.Int {
		return nil, "", "", false
	}
	if v, exact := constant.Int64Val(ltv.Value); !exact || v != 2 {
		return nil, "", "", false
	}
	// The identity len(SplitN(s, sep, 2)) ∈ {1, 2} with 2 ⇔ Contains
	// requires a NON-EMPTY separator; without proof the check stays
	// silent (PS2121's exact guard).
	if !ps2121SepNonEmpty(pass, pkg, call.Args[1]) {
		return nil, "", "", false
	}
	return call, pkgIdent.Name, pkg, true
}

// ps2029OneTwoLit reports whether e is (possibly parenthesized) the
// literal integer constant 1 or 2, returning its value. A variable or
// named constant holding 1 or 2 is deliberately NOT matched: the check
// only rewrites the spelling that is provably a membership test
// against the untyped literal.
func ps2029OneTwoLit(pass *analysis.Pass, e ast.Expr) (int64, bool) {
	lit, ok := ps2108Unparen(e).(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, false
	}
	tv, ok := pass.TypesInfo.Types[lit]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return 0, false
	}
	if v, exact := constant.Int64Val(tv.Value); exact && (v == 1 || v == 2) {
		return v, true
	}
	return 0, false
}

// ps2029Rewrite builds the text edits, the rendered replacement text
// and the scaffolding guard spans (pos/end pairs; a comment inside any
// of them withholds the fix). The haystack and separator expressions
// carry over byte-verbatim; only the scaffolding around them is
// rewritten. Chain-aware fast paths mirror PS5104: a Contains whose
// needle is PS5016's / PS5014's auto-fixable Before-shape would be
// rewritten again by the next -fix pass, so those shapes emit the
// final IndexByte fixed point directly.
func ps2029Rewrite(pass *analysis.Pass, bin *ast.BinaryExpr, m *ps2029Site) ([]analysis.TextEdit, string, []token.Pos) {
	hayText, okHay := ps5004ExprText(m.haystack)
	if !okHay {
		return nil, "", nil
	}
	cmp, bang := " >= 0", ""
	if m.negate {
		cmp, bang = " < 0", "!"
	}
	byteHead := m.qual + ".IndexByte("

	// strings separator that is a direct ONE-BYTE string literal:
	// emitting strings.Contains(s, "=") would hand PS5016 its exact
	// Before-shape, so emit strings.IndexByte(s, "="[0]) >= 0 (negated:
	// < 0) directly, reusing the literal token byte-for-byte.
	if m.pkg == "strings" {
		if lit := ps5104OneByteLit(pass, m.sep); lit != nil {
			edits := []analysis.TextEdit{
				{Pos: bin.Pos(), End: m.haystack.Pos(), NewText: []byte(byteHead)},
			}
			if m.sep.Pos() != lit.Pos() {
				edits = append(edits, analysis.TextEdit{Pos: m.sep.Pos(), End: lit.Pos()})
			}
			edits = append(edits, analysis.TextEdit{Pos: lit.End(), End: bin.End(), NewText: []byte("[0])" + cmp)})
			after := byteHead + hayText + ", " + lit.Value + "[0])" + cmp
			guards := []token.Pos{bin.Pos(), m.haystack.Pos(), m.sep.Pos(), lit.Pos(), lit.End(), bin.End()}
			return edits, after, guards
		}
	}

	if m.pkg == "bytes" {
		switch x := ps2121Unparen(m.sep).(type) {
		case *ast.CompositeLit:
			// []byte{X} with EXACTLY one unkeyed element: emitting
			// bytes.Contains(b, []byte{X}) would hand PS5014 its exact
			// Before-shape, so unwrap the element in place and emit
			// bytes.IndexByte(b, X) >= 0 — X is byte-assignable by the
			// composite-literal typing rule and is evaluated exactly
			// once in the same argument position. Keyed or multi-element
			// composites keep the plain Contains rewrite (PS5014 leaves
			// those alone).
			if len(x.Elts) == 1 {
				if _, keyed := x.Elts[0].(*ast.KeyValueExpr); !keyed {
					elt := x.Elts[0]
					eltText, okElt := ps5004ExprText(elt)
					if !okElt {
						return nil, "", nil
					}
					edits := []analysis.TextEdit{
						{Pos: bin.Pos(), End: m.haystack.Pos(), NewText: []byte(byteHead)},
						{Pos: m.sep.Pos(), End: elt.Pos()},
						{Pos: elt.End(), End: bin.End(), NewText: []byte(")" + cmp)},
					}
					after := byteHead + hayText + ", " + eltText + ")" + cmp
					guards := []token.Pos{bin.Pos(), m.haystack.Pos(), m.sep.Pos(), elt.Pos(), elt.End(), bin.End()}
					return edits, after, guards
				}
			}
		case *ast.CallExpr:
			// []byte("z") — a conversion of a direct one-byte string
			// literal is PS5014's other Before-shape: emit
			// bytes.IndexByte(b, "z"[0]) >= 0, reusing the literal token
			// byte-for-byte. A named-constant or multi-byte operand
			// keeps the plain Contains rewrite (advisory-only in
			// PS5014, so no churn).
			if len(x.Args) == 1 && !x.Ellipsis.IsValid() {
				if lit, okLit := ps2121Unparen(x.Args[0]).(*ast.BasicLit); okLit && lit.Kind == token.STRING {
					if tv, okTv := pass.TypesInfo.Types[lit]; okTv && tv.Value != nil &&
						tv.Value.Kind() == constant.String && len(constant.StringVal(tv.Value)) == 1 {
						edits := []analysis.TextEdit{
							{Pos: bin.Pos(), End: m.haystack.Pos(), NewText: []byte(byteHead)},
							{Pos: m.sep.Pos(), End: lit.Pos()},
							{Pos: lit.End(), End: bin.End(), NewText: []byte("[0])" + cmp)},
						}
						after := byteHead + hayText + ", " + lit.Value + "[0])" + cmp
						guards := []token.Pos{bin.Pos(), m.haystack.Pos(), m.sep.Pos(), lit.Pos(), lit.End(), bin.End()}
						return edits, after, guards
					}
				}
			}
		}
	}

	// Plain Contains rewrite: both argument expressions and the text
	// between them (the comma, spacing, even a comment) carry over
	// byte-verbatim; the scaffolding before the haystack and after the
	// separator collapses into a (possibly negated) Contains call.
	sepText, okSep := ps5004ExprText(m.sep)
	if !okSep {
		return nil, "", nil
	}
	head := bang + m.qual + ".Contains("
	edits := []analysis.TextEdit{
		{Pos: bin.Pos(), End: m.haystack.Pos(), NewText: []byte(head)},
		{Pos: m.sep.End(), End: bin.End(), NewText: []byte(")")},
	}
	after := head + hayText + ", " + sepText + ")"
	guards := []token.Pos{bin.Pos(), m.haystack.Pos(), m.sep.End(), bin.End()}
	return edits, after, guards
}
