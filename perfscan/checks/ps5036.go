package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5036 reports bytes.LastIndex whose int result is only compared
// against the literal -1 or 0 to answer a membership question — "does
// sub occur in b at all?" — where bytes.Contains gives the identical
// boolean via the optimized FORWARD scan that stops at the first match,
// instead of LastIndex's pure-Go backward Rabin-Karp pass. The bytes
// twin of PS5031 (the strings version).
var PS5036 = register(&lint.Check{
	ID:       "PS5036",
	Category: "arith",
	Slug:     "bytes-lastindex-membership-to-contains",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "bytes.LastIndex compared against -1/0 for membership runs a backward Rabin-Karp; Contains is the optimized forward scan that stops at the first match",
		Text: `For a needle of length 2..len(b)-1, bytes.LastIndex runs a
pure-Go BACKWARD Rabin-Karp: it computes a reverse rolling hash over the
haystack with no assembly fast path and, because it must find the LAST
occurrence, a forward short-circuit cannot exist for it. A membership
test does not care WHERE the needle occurs — only WHETHER it occurs —
and bytes.Contains(b, sub) is Index(b, sub) != -1, which dispatches to
the bytealg substring machinery: an IndexByte-accelerated (SIMD where
the platform has it) forward scan that returns at the FIRST match, with
Rabin-Karp only as a pathological-input fallback. The rewrite trades a
scalar reverse hash over (potentially) the whole haystack for the
optimized forward scanner — the same asymptotics, materially less work,
and an early exit the backward search can never take. This is NOT the
rejected Index != -1 -> Contains readability rewrite: there Contains
wraps the very same Index call (identical work); here the source
function runs a genuinely slower algorithm. The bytes implementations
mirror the strings ones one-for-one, so this is exactly PS5031 for
[]byte haystacks.

Exactly six comparison shapes are membership-equivalent, and only those
are matched (with the literal on either side of the operator):

	LastIndex(b, sub) != -1  →  Contains(b, sub)
	LastIndex(b, sub) >  -1  →  Contains(b, sub)
	LastIndex(b, sub) >= 0   →  Contains(b, sub)
	LastIndex(b, sub) == -1  →  !Contains(b, sub)
	LastIndex(b, sub) <  0   →  !Contains(b, sub)
	LastIndex(b, sub) <= -1  →  !Contains(b, sub)

The predicate is total and bit-identical: bytes.LastIndex(b, sub)
returns >= 0 exactly when sub occurs in b, which is exactly when
bytes.Contains(b, sub) is true. Every edge agrees — the empty or nil
needle (LastIndex(b, nil) is len(b), always >= 0; Contains(b, nil) is
true), a nil haystack, an absent needle (-1 / false), a needle equal to
or longer than the haystack, NUL bytes and invalid UTF-8 (both sides
match raw bytes with no decoding). Both b and sub are plain []byte
values evaluated EXACTLY once in both forms, both functions read their
arguments and write nothing, and neither can panic, so the rewrite is
behavior-preserving with no guard needed. Comparisons that genuinely
use the POSITION are left alone: == 0, != 0, > 0, >= 1, >= -1 (constant
true), a result bound to a variable, used as an index, or compared
against anything but the direct literal -1/0 (a variable or named
constant holding -1 is not matched).

The automatic fix edits around the call and never touches the argument
text: same expressions, same evaluation order. Only the selected name
changes (LastIndex -> Contains), so an aliased bytes import keeps its
qualifier, and Contains lives in the same package as LastIndex, so no
import is ever added or dropped. When the comparison flips (== -1, < 0,
<= -1) the result is prefixed with !, which binds tighter than any
binary operator — safe inside a larger && / || condition without extra
parentheses. The comparison is an untyped bool that adopts its context's
boolean type; Contains returns the basic type bool, so a context that
materialized a NAMED bool type is skipped — the rewrite would not
compile there.

A needle that is statically ONE byte — a one-element unkeyed composite
literal []byte{X} or a conversion []byte("z") of a one-byte string
literal — is deliberately NOT matched: PS5013 already rewrites that
whole call to the direct bytes.LastIndexByte byte scan (and emitting
Contains there would be PS5014's Before-shape, which the next -fix pass
would rewrite again — churn both checks' idempotence forbids). A
conversion []byte(c) of a one-byte NAMED string constant keeps the
plain Contains rewrite — PS5013 and PS5014 both report that shape
advisory-only (indexing the constant by hand would discard its symbolic
name), so nothing churns. The bytes.LastIndexByte membership sibling
(backward byte scan vs the forward SIMD IndexByte) is likewise left
out: PS5013's fix output would otherwise be this check's input,
re-fixed on the following pass. strings.LastIndex is PS5031's territory.
A shadowed bytes identifier or a local method named LastIndex does not
resolve to the standard-library function and is rejected via type
information.`,
		Before: `if bytes.LastIndex(body, marker) != -1 {
	return true
}`,
		After: `if bytes.Contains(body, marker) {
	return true
}`,
		MeasuredWin: `BenchmarkPS5036 (11.3 KB haystack of log lines, the
needle occurring only in the header line at byte offset 27, plus one
absent needle, Apple M2 Pro, go1.26): present-needle probe
13900 ns/op -> 43 ns/op (~320x — the backward Rabin-Karp hashes
essentially the whole haystack before reaching the sole match near the
front, while Contains short-circuits there); absent needle
14500 ns/op -> 3600 ns/op (~4.0x — both sides scan everything, the
SIMD-assisted forward scan vs the scalar reverse rolling hash). 0 B/op
and 0 allocs/op on every side.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5036",
		Doc:  "bytes.LastIndex compared against -1/0 for membership instead of the forward bytes.Contains scan",
		Run:  runPS5036,
	},
})

func runPS5036(pass *analysis.Pass) (any, error) {
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
			call, litVal, litOnLeft, ok := ps5036Match(pass, bin)
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
			// A statically one-byte literal needle ([]byte{X} or a one-byte
			// []byte("z") conversion) is PS5013's territory: PS5013 rewrites
			// the whole call to LastIndexByte, and a Contains spelling here
			// would be PS5014's Before-shape — either way the two fixes would
			// collide on this span or churn on the next -fix pass. Stay
			// silent; PS5013 reports the call itself.
			if ps5036OneByteNeedle(pass, call.Args[1]) {
				return true
			}
			sel := call.Fun.(*ast.SelectorExpr)

			repl := "bytes.Contains(b, sub)"
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
				Message: "bytes.LastIndex(...) " + op.String() + " " + strconv.FormatInt(litVal, 10) + " tests membership only; " + repl + " is the optimized forward scan and stops at the first match instead of running the backward Rabin-Karp",
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

// ps5036Match reports whether bin compares a direct bytes.LastIndex
// call against the literal integer constant -1 or 0, returning the call,
// the literal's value and which side the literal is on. The call operand
// must be the bare CallExpr (no parentheses) so the surrounding edit
// ranges are exact; the literal may be parenthesized (the whole literal
// side is deleted either way).
func ps5036Match(pass *analysis.Pass, bin *ast.BinaryExpr) (call *ast.CallExpr, litVal int64, litOnLeft, ok bool) {
	if c := ps5036LastIndexCall(pass, bin.X); c != nil {
		if v, isLit := ps5031NegOneZeroLit(pass, bin.Y); isLit {
			return c, v, false, true
		}
	}
	if c := ps5036LastIndexCall(pass, bin.Y); c != nil {
		if v, isLit := ps5031NegOneZeroLit(pass, bin.X); isLit {
			return c, v, true, true
		}
	}
	return nil, 0, false, false
}

// ps5036LastIndexCall returns e as a call of the package-level function
// bytes.LastIndex, or nil. Type information pins the callee to the
// standard library: a local variable, field, or method spelled LastIndex
// — or a shadowed `bytes` identifier — does not resolve to a
// receiver-less *types.Func with package path "bytes" and is rejected.
// strings.LastIndex is PS5031's territory, out of scope here.
func ps5036LastIndexCall(pass *analysis.Pass, e ast.Expr) *ast.CallExpr {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || call.Ellipsis != token.NoPos {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "LastIndex" || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
		return nil
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return nil
	}
	return call
}

// ps5036OneByteNeedle reports whether the needle is one of the two
// statically-one-byte literal shapes PS5013 auto-fixes to the byte
// forms: a one-element unkeyed byte-slice composite literal []byte{X},
// or a byte-slice conversion []byte("z") of a DIRECT string literal of
// byte-length exactly 1. Those sites are PS5013's to rewrite (and a
// Contains spelling would be PS5014's Before-shape). A conversion of a
// one-byte NAMED string constant returns false — PS5013/PS5014 report
// that shape advisory-only, so the plain Contains rewrite does not
// churn and keeps the symbolic name. The empty needle ([]byte{},
// []byte(""), nil) also returns false: PS5013/PS5014 exclude it, and
// the membership rewrite stays bit-identical there (constant true).
func ps5036OneByteNeedle(pass *analysis.Pass, e ast.Expr) bool {
	arg := ps2108Unparen(e)

	// Shape A: a one-element, unkeyed composite literal []byte{X}.
	if lit, isLit := arg.(*ast.CompositeLit); isLit {
		if !ps5013ByteSlice(pass.TypesInfo.TypeOf(lit)) || len(lit.Elts) != 1 {
			return false
		}
		if _, keyed := lit.Elts[0].(*ast.KeyValueExpr); keyed {
			// []byte{0: 'x'} is out of scope for PS5013/PS5014 alike, so
			// the Contains rewrite cannot churn.
			return false
		}
		return true
	}

	// Shape B: a conversion []byte("z") of a direct string literal of
	// decoded byte-length exactly 1.
	conv, isConv := arg.(*ast.CallExpr)
	if !isConv || len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return false
	}
	if tv, found := pass.TypesInfo.Types[conv.Fun]; !found || !tv.IsType() || !ps5013ByteSlice(tv.Type) {
		return false
	}
	operand := ps2108Unparen(conv.Args[0])
	lit, isLit := operand.(*ast.BasicLit)
	if !isLit || lit.Kind != token.STRING {
		// A named constant or constant expression: PS5013/PS5014 are
		// advisory-only there — the Contains rewrite stands.
		return false
	}
	tv, found := pass.TypesInfo.Types[operand]
	if !found || tv.Value == nil || tv.Value.Kind() != constant.String {
		return false
	}
	return len(constant.StringVal(tv.Value)) == 1
}
