package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5104 reports strings.Count / bytes.Count whose result is only
// compared against the literal 0 or 1 to answer a membership question —
// "is there at least one occurrence?" / "is there none?" — where
// strings.Contains / bytes.Contains gives the same boolean and stops at
// the first match instead of tallying every occurrence.
var PS5104 = register(&lint.Check{
	ID:       "PS5104",
	Category: "arith",
	Slug:     "count-membership-to-contains",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Count/bytes.Count compared against 0 or 1 for membership, where Contains short-circuits",
		Text: `strings.Count scans the entire input tallying every
non-overlapping occurrence of the substring; a membership predicate only
cares whether that tally is zero or non-zero. strings.Contains returns
Index(s, sub) >= 0 and short-circuits at the first match — the same
boolean answer for strictly less work. The same holds for bytes.Count
versus bytes.Contains.

Exactly six comparison shapes are membership-equivalent, and only those
are matched (with the literal on either side of the operator):

	Count(s, sub) >  0  →  Contains(s, sub)
	Count(s, sub) >= 1  →  Contains(s, sub)
	Count(s, sub) != 0  →  Contains(s, sub)
	Count(s, sub) == 0  →  !Contains(s, sub)
	Count(s, sub) <  1  →  !Contains(s, sub)
	Count(s, sub) <= 0  →  !Contains(s, sub)

The empty-substring edge case stays bit-identical: strings.Count(s, "")
is utf8.RuneCountInString(s)+1 — always >= 1 — so Count(s, "") > 0 is
always true, exactly like strings.Contains(s, ""); likewise
bytes.Count(b, nil) is len(b)+1 and bytes.Contains(b, nil) is always
true. Both functions read the same inputs, write nothing, and cannot
panic, so the rewrite is behavior-preserving with no guard needed.

The automatic fix edits around the call and never touches the argument
text: same expressions, same evaluation order. Only the selected name
changes (Count -> Contains), so an aliased import keeps its qualifier,
and Contains lives in the same package as Count, so the import can never
be orphaned. When the comparison flips (== 0, < 1, <= 0) the result is
prefixed with !, which binds tighter than any binary operator — safe
inside a larger && / || condition without extra parentheses.

Comparisons that genuinely need the count are left alone: Count(...) > 1,
== 2, >= 2, <= 1 and friends, a Count result bound to a variable or used
arithmetically, and any comparison against a non-literal (a variable or
named constant holding 0 is not matched). A shadowed strings/bytes
identifier or a local method named Count does not resolve to the
standard-library function and is rejected via type information.`,
		Before: `if strings.Count(body, marker) > 0 {
	return true
}`,
		After: `if strings.Contains(body, marker) {
	return true
}`,
		MeasuredWin: `BenchmarkPS5104 (11.5 KB haystack, one needle first
matching at byte offset 6 plus one absent needle, Apple M2 Pro):
6128 ns/op -> 178 ns/op (~34x). The present-needle probe collapses from
a counting scan over all 512 occurrences — Count restarts its Index
machinery after every hit — to a short-circuit at the first match; the
absent-needle probe scans everything either way and bounds the win.
Zero allocations either way.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5104",
		Doc:  "strings.Count/bytes.Count compared against 0 or 1 for membership instead of Contains",
		Run:  runPS5104,
	},
})

func runPS5104(pass *analysis.Pass) (any, error) {
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
			call, pkg, litVal, litOnLeft, ok := ps5104Match(pass, bin)
			if !ok {
				return true
			}
			// Normalize to the call-on-left spelling: `0 < Count(...)`
			// means `Count(...) > 0`, so mirror the operator when the
			// literal is on the left.
			op := bin.Op
			if litOnLeft {
				op = ps5104Mirror(op)
			}
			negate, ok := ps5104Membership(op, litVal)
			if !ok {
				// Count(...) > 1, == 2, <= 1, ... genuinely need the tally.
				return true
			}
			// The comparison is an untyped bool and adopts whatever
			// boolean type its context demands (var f myBool = Count > 0);
			// Contains returns the basic type bool. Skip when the context
			// materialized a named bool type — the rewrite would not
			// compile there.
			if tv, ok := pass.TypesInfo.Types[bin]; ok {
				if b, isBasic := tv.Type.(*types.Basic); !isBasic || b.Info()&types.IsBoolean == 0 {
					return true
				}
			}
			sel := call.Fun.(*ast.SelectorExpr)
			repl := pkg + ".Contains(s, sub)"
			if negate {
				repl = "!" + repl
			}
			// The rewrite edits around the call and never touches the
			// argument text: same expressions, same evaluation order. Only
			// the selected identifier changes (Count -> Contains), so an
			// aliased import keeps working — and since Contains lives in
			// the same package, the import cannot be orphaned.
			var edits []analysis.TextEdit
			if litOnLeft {
				// `0 < call` etc.: fold the leading literal and operator
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
				// Drop the trailing `> 0` / `== 0` / ...
				edits = append(edits, analysis.TextEdit{Pos: call.End(), End: bin.End()})
			}
			pass.Report(analysis.Diagnostic{
				Pos:     bin.Pos(),
				End:     bin.End(),
				Message: pkg + ".Count(...) " + op.String() + " " + litVal.ExactString() + " tests membership only; " + repl + " is faster and stops at the first match",
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

// ps5104Match reports whether bin compares a direct strings.Count or
// bytes.Count call against the literal integer constant 0 or 1,
// returning the call, its package ("strings" or "bytes"), the literal's
// value and which side the literal is on. The call operand must be the
// bare CallExpr (no parentheses) so the surrounding edit ranges are
// exact; the literal may be parenthesized (the whole literal side is
// deleted either way).
func ps5104Match(pass *analysis.Pass, bin *ast.BinaryExpr) (call *ast.CallExpr, pkg string, litVal constant.Value, litOnLeft, ok bool) {
	if c, p := ps5104CountCall(pass, bin.X); c != nil {
		if v, isLit := ps5104ZeroOneLit(pass, bin.Y); isLit {
			return c, p, v, false, true
		}
	}
	if c, p := ps5104CountCall(pass, bin.Y); c != nil {
		if v, isLit := ps5104ZeroOneLit(pass, bin.X); isLit {
			return c, p, v, true, true
		}
	}
	return nil, "", nil, false, false
}

// ps5104Mirror swaps a comparison operator's operand order: `0 < x` is
// `x > 0`. Equality operators are symmetric.
func ps5104Mirror(op token.Token) token.Token {
	switch op {
	case token.LSS:
		return token.GTR
	case token.LEQ:
		return token.GEQ
	case token.GTR:
		return token.LSS
	case token.GEQ:
		return token.LEQ
	}
	return op
}

// ps5104Membership classifies the normalized (call-on-left) comparison
// `Count(...) <op> <lit>` as one of the six membership forms, reporting
// whether the Contains rewrite must be negated. Any other (op, literal)
// pair genuinely needs the count and is not matched.
func ps5104Membership(op token.Token, lit constant.Value) (negate, ok bool) {
	v, exact := constant.Int64Val(lit)
	if !exact {
		return false, false
	}
	switch {
	case op == token.GTR && v == 0, // Count > 0
		op == token.GEQ && v == 1, // Count >= 1
		op == token.NEQ && v == 0: // Count != 0
		return false, true
	case op == token.EQL && v == 0, // Count == 0
		op == token.LSS && v == 1, // Count < 1
		op == token.LEQ && v == 0: // Count <= 0
		return true, true
	}
	return false, false
}

// ps5104CountCall returns e as a call of the package-level function
// strings.Count or bytes.Count along with the package name, or nil.
// Type information pins the callee to the standard library: a local
// variable, field, or method spelled Count — or a shadowed `strings` /
// `bytes` identifier — does not resolve to a receiver-less *types.Func
// with package path "strings" or "bytes" and is rejected.
func ps5104CountCall(pass *analysis.Pass, e ast.Expr) (*ast.CallExpr, string) {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || call.Ellipsis != token.NoPos {
		return nil, ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, ""
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "Count" || fn.Pkg() == nil {
		return nil, ""
	}
	pkg := fn.Pkg().Path()
	if pkg != "strings" && pkg != "bytes" {
		return nil, ""
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return nil, ""
	}
	return call, pkg
}

// ps5104ZeroOneLit reports whether e is (possibly parenthesized) the
// literal integer constant 0 or 1, returning its value. A variable or
// named constant holding 0 or 1 is deliberately NOT matched: the check
// only rewrites the spelling that is provably a membership test against
// the untyped literal.
func ps5104ZeroOneLit(pass *analysis.Pass, e ast.Expr) (constant.Value, bool) {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			break
		}
		e = p.X
	}
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return nil, false
	}
	tv, ok := pass.TypesInfo.Types[lit]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return nil, false
	}
	if v, exact := constant.Int64Val(tv.Value); exact && (v == 0 || v == 1) {
		return tv.Value, true
	}
	return nil, false
}
