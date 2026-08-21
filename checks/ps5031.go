package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5031 reports strings.LastIndex whose int result is only compared
// against the literal -1 or 0 to answer a membership question — "does
// sub occur in s at all?" — where strings.Contains gives the identical
// boolean via the optimized FORWARD scan that stops at the first match,
// instead of LastIndex's pure-Go backward Rabin-Karp pass. The
// LastIndex twin of PS5104 (Count compared against 0/1 for membership).
var PS5031 = register(&lint.Check{
	ID:       "PS5031",
	Category: "arith",
	Slug:     "lastindex-membership-to-contains",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.LastIndex compared against -1/0 for membership runs a backward Rabin-Karp; Contains is the optimized forward scan that stops at the first match",
		Text: `For a needle of length 2..len(s)-1, strings.LastIndex runs a
pure-Go BACKWARD Rabin-Karp: it computes a reverse rolling hash over the
haystack with no assembly fast path and, because it must find the LAST
occurrence, a forward short-circuit cannot exist for it. A membership
test does not care WHERE the needle occurs — only WHETHER it occurs —
and strings.Contains(s, sub) is Index(s, sub) >= 0, which dispatches to
the bytealg substring machinery: an IndexByte-accelerated (SIMD where
the platform has it) forward scan that returns at the FIRST match, with
Rabin-Karp only as a pathological-input fallback. The rewrite trades a
scalar reverse hash over (potentially) the whole haystack for the
optimized forward scanner — the same asymptotics, materially less work,
and an early exit the backward search can never take. This is NOT the
rejected Index != -1 -> Contains readability rewrite: there Contains
wraps the very same Index call (identical work); here the source
function runs a genuinely slower algorithm.

Exactly six comparison shapes are membership-equivalent, and only those
are matched (with the literal on either side of the operator):

	LastIndex(s, sub) != -1  →  Contains(s, sub)
	LastIndex(s, sub) >  -1  →  Contains(s, sub)
	LastIndex(s, sub) >= 0   →  Contains(s, sub)
	LastIndex(s, sub) == -1  →  !Contains(s, sub)
	LastIndex(s, sub) <  0   →  !Contains(s, sub)
	LastIndex(s, sub) <= -1  →  !Contains(s, sub)

The predicate is total and bit-identical: strings.LastIndex(s, sub)
returns >= 0 exactly when sub occurs in s, which is exactly when
strings.Contains(s, sub) is true. Every edge agrees — the empty needle
(LastIndex(s, "") is len(s), always >= 0; Contains(s, "") is true), an
absent needle (-1 / false), a needle equal to or longer than the
haystack, NUL bytes and invalid UTF-8 (both sides match raw bytes with
no decoding). Both s and sub are plain strings evaluated EXACTLY once in
both forms, both functions read their arguments and write nothing, and
neither can panic, so the rewrite is behavior-preserving with no guard
needed. Comparisons that genuinely use the POSITION are left alone:
== 0, != 0, > 0, >= 1, >= -1 (constant true), a result bound to a
variable, used as an index, or compared against anything but the direct
literal -1/0 (a variable or named constant holding -1 is not matched).

The automatic fix edits around the call and never touches the argument
text: same expressions, same evaluation order. Only the selected name
changes (LastIndex -> Contains), so an aliased strings import keeps its
qualifier, and Contains lives in the same package as LastIndex, so no
import is ever added or dropped. When the comparison flips (== -1, < 0,
<= -1) the result is prefixed with !, which binds tighter than any
binary operator — safe inside a larger && / || condition without extra
parentheses. The comparison is an untyped bool that adopts its context's
boolean type; Contains returns the basic type bool, so a context that
materialized a NAMED bool type is skipped — the rewrite would not
compile there.

A needle that is a direct ONE-BYTE string literal is deliberately NOT
matched: PS5007 already rewrites that whole call to the direct
strings.LastIndexByte byte scan (and emitting Contains there would be
PS5016's Before-shape, which the next -fix pass would rewrite again —
churn both checks' idempotence forbids). A one-byte NAMED constant
needle keeps the plain Contains rewrite — PS5016 reports that shape
advisory-only, so nothing churns. The strings.LastIndexByte membership
sibling (backward byte scan vs the forward SIMD IndexByte) is likewise
left out: PS5007's fix output would otherwise be this check's input,
re-fixed on the following pass. bytes.LastIndex is a different
implementation and out of scope here. A shadowed strings identifier or
a local method named LastIndex does not resolve to the standard-library
function and is rejected via type information.`,
		Before: `if strings.LastIndex(body, marker) != -1 {
	return true
}`,
		After: `if strings.Contains(body, marker) {
	return true
}`,
		MeasuredWin: `BenchmarkPS5031 (11.3 KB haystack of log lines, the
needle occurring only in the header line at byte offset 27, plus one
absent needle, Apple M2 Pro, go1.26): present-needle probe
14000 ns/op -> 43 ns/op (~330x — the backward Rabin-Karp hashes
essentially the whole haystack before reaching the sole match near the
front, while Contains short-circuits there); absent needle
14200 ns/op -> 3700 ns/op (~3.9x — both sides scan everything, the
SIMD-assisted forward scan vs the scalar reverse rolling hash). 0 B/op
and 0 allocs/op on every side.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5031",
		Doc:  "strings.LastIndex compared against -1/0 for membership instead of the forward strings.Contains scan",
		Run:  runPS5031,
	},
})

func runPS5031(pass *analysis.Pass) (any, error) {
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
			call, litVal, litOnLeft, ok := ps5031Match(pass, bin)
			if !ok {
				return true
			}
			// Normalize to the call-on-left spelling: `-1 != LastIndex(...)`
			// means `LastIndex(...) != -1`, so mirror the operator when the
			// literal is on the left.
			op := bin.Op
			if litOnLeft {
				op = ps5104Mirror(op)
			}
			negate, ok := ps5031Membership(op, litVal)
			if !ok {
				// == 0, > 0, >= -1, ... genuinely use the position (or are
				// constant) — not membership.
				return true
			}
			// The comparison is an untyped bool and adopts whatever boolean
			// type its context demands (var f myBool = LastIndex != -1);
			// Contains returns the basic type bool. Skip when the context
			// materialized a named bool type — the rewrite would not compile
			// there.
			if tv, ok := pass.TypesInfo.Types[bin]; ok {
				if b, isBasic := tv.Type.(*types.Basic); !isBasic || b.Info()&types.IsBoolean == 0 {
					return true
				}
			}
			// A direct one-byte string-literal needle is PS5007's territory:
			// PS5007 rewrites the whole call to LastIndexByte, and a Contains
			// spelling here would be PS5016's Before-shape — either way the
			// two fixes would collide on this span or churn on the next -fix
			// pass. Stay silent; PS5007 reports the call itself.
			if ps5104OneByteLit(pass, call.Args[1]) != nil {
				return true
			}
			sel := call.Fun.(*ast.SelectorExpr)

			repl := "strings.Contains(s, sub)"
			if negate {
				repl = "!" + repl
			}
			// The rewrite edits around the call and never touches the
			// argument text: same expressions, same evaluation order. Only
			// the selected identifier changes (LastIndex -> Contains), so an
			// aliased import keeps working — and since Contains lives in the
			// same package, the import cannot be orphaned.
			var edits []analysis.TextEdit
			if litOnLeft {
				// `-1 != call` etc.: fold the leading literal and operator
				// into the (optional) negation.
				lead := ""
				if negate {
					lead = "!"
				}
				edits = append(edits, analysis.TextEdit{Pos: bin.Pos(), End: call.Pos(), NewText: []byte(lead)})
			} else if negate {
				edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: call.Pos(), NewText: []byte("!")})
			}
			edits = append(edits, analysis.TextEdit{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Contains")})
			if !litOnLeft {
				// Drop the trailing `!= -1` / `< 0` / ...
				edits = append(edits, analysis.TextEdit{Pos: call.End(), End: bin.End()})
			}
			pass.Report(analysis.Diagnostic{
				Pos:     bin.Pos(),
				End:     bin.End(),
				Message: "strings.LastIndex(...) " + op.String() + " " + strconv.FormatInt(litVal, 10) + " tests membership only; " + repl + " is the optimized forward scan and stops at the first match instead of running the backward Rabin-Karp",
				SuggestedFixes: []analysis.SuggestedFix{{
					Message:   "replace with " + repl,
					TextEdits: edits,
				}},
			})
			return true
		})
	}
	return nil, nil
}

// ps5031Match reports whether bin compares a direct strings.LastIndex
// call against the literal integer constant -1 or 0, returning the call,
// the literal's value and which side the literal is on. The call operand
// must be the bare CallExpr (no parentheses) so the surrounding edit
// ranges are exact; the literal may be parenthesized (the whole literal
// side is deleted either way).
func ps5031Match(pass *analysis.Pass, bin *ast.BinaryExpr) (call *ast.CallExpr, litVal int64, litOnLeft, ok bool) {
	if c := ps5031LastIndexCall(pass, bin.X); c != nil {
		if v, isLit := ps5031NegOneZeroLit(pass, bin.Y); isLit {
			return c, v, false, true
		}
	}
	if c := ps5031LastIndexCall(pass, bin.Y); c != nil {
		if v, isLit := ps5031NegOneZeroLit(pass, bin.X); isLit {
			return c, v, true, true
		}
	}
	return nil, 0, false, false
}

// ps5031Membership classifies the normalized (call-on-left) comparison
// `LastIndex(...) <op> <lit>` as one of the six membership forms,
// reporting whether the Contains rewrite must be negated. Any other
// (op, literal) pair uses the position (or is constant) and is not
// matched: == 0 / != 0 / > 0 ask "at the start?" / "past the start?",
// >= 1 asks "past the start?", >= -1 and < -1 are constants.
func ps5031Membership(op token.Token, v int64) (negate, ok bool) {
	switch {
	case op == token.NEQ && v == -1, // LastIndex != -1
		op == token.GTR && v == -1, // LastIndex > -1
		op == token.GEQ && v == 0:  // LastIndex >= 0
		return false, true
	case op == token.EQL && v == -1, // LastIndex == -1
		op == token.LSS && v == 0,  // LastIndex < 0
		op == token.LEQ && v == -1: // LastIndex <= -1
		return true, true
	}
	return false, false
}

// ps5031LastIndexCall returns e as a call of the package-level function
// strings.LastIndex, or nil. Type information pins the callee to the
// standard library: a local variable, field, or method spelled LastIndex
// — or a shadowed `strings` identifier — does not resolve to a
// receiver-less *types.Func with package path "strings" and is rejected.
// bytes.LastIndex is a different implementation, out of scope here.
func ps5031LastIndexCall(pass *analysis.Pass, e ast.Expr) *ast.CallExpr {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || call.Ellipsis != token.NoPos {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "LastIndex" || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
		return nil
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return nil
	}
	return call
}

// ps5031NegOneZeroLit reports whether e is (possibly parenthesized) the
// literal integer constant -1 (a unary minus applied to a direct int
// literal) or 0, returning its value. A variable or named constant
// holding -1 or 0 is deliberately NOT matched: the check only rewrites
// the spelling that is provably a membership test against the literal.
func ps5031NegOneZeroLit(pass *analysis.Pass, e ast.Expr) (int64, bool) {
	e = ps2108Unparen(e)
	inner := e
	if u, isUnary := e.(*ast.UnaryExpr); isUnary {
		if u.Op != token.SUB {
			return 0, false
		}
		inner = ps2108Unparen(u.X)
	}
	lit, ok := inner.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, false
	}
	tv, ok := pass.TypesInfo.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return 0, false
	}
	v, exact := constant.Int64Val(tv.Value)
	if !exact || (v != -1 && v != 0) {
		return 0, false
	}
	return v, true
}
