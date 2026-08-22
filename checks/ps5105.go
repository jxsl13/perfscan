package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5105 reports strings.Index / bytes.Index whose result is compared
// with == against the literal 0 — "does it start with sub?" — where
// strings.HasPrefix / bytes.HasPrefix gives the same boolean and stops
// at the first differing byte instead of scanning the whole input for
// the first occurrence.
var PS5105 = register(&lint.Check{
	ID:       "PS5105",
	Category: "arith",
	Slug:     "index-zero-to-hasprefix",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Index/bytes.Index compared == 0 for a prefix test, where HasPrefix stops at the first mismatch",
		Text: `strings.Index(s, sub) == 0 is true exactly when s begins
with sub — but Index scans the entire input to locate the first
occurrence, and when sub sits anywhere other than position 0 the == 0
comparison throws that work away. strings.HasPrefix(s, sub) returns the
same boolean via a length check and a single prefix compare that fails
at the first differing byte. The same holds for bytes.Index versus
bytes.HasPrefix.

Exactly one comparison shape is prefix-equivalent, and only it is
matched (with the literal on either side of the operator):

	Index(s, sub) == 0  →  HasPrefix(s, sub)

The empty-substring edge case stays bit-identical: strings.Index(s, "")
is 0 — so Index(s, "") == 0 is always true, exactly like
strings.HasPrefix(s, ""); likewise bytes.Index(b, nil) is 0 and
bytes.HasPrefix(b, nil) is always true. Both functions read the same
two inputs exactly once, write nothing, and cannot panic, so the
rewrite is behavior-preserving with no guard needed.

The automatic fix edits around the call and never touches the argument
text: same expressions, same evaluation order. Only the selected name
changes (Index -> HasPrefix), so an aliased import keeps its qualifier,
and HasPrefix lives in the same package as Index, so the import can
never be orphaned. The replacement is a bare call — a primary
expression — so it needs no parentheses in any context the comparison
was legal in.

Comparisons that mean something else are left alone: Index(...) >= 0,
> 0, != 0, < 1 and friends are membership or position questions, an
Index result bound to a variable and compared later is out of scope,
and any comparison against a non-literal (a variable or named constant
holding 0 is not matched). A shadowed strings/bytes identifier or a
local method named Index does not resolve to the standard-library
function and is rejected via type information. The HasSuffix analog
(LastIndex(s, sub) == len(s)-len(sub)) is deliberately NOT rewritten:
when sub is absent and len(sub) == len(s)+1 both sides are -1 and the
original is true while HasSuffix is false — not behavior-preserving.`,
		Before: `if strings.Index(line, "# ") == 0 {
	return true
}`,
		After: `if strings.HasPrefix(line, "# ") {
	return true
}`,
		MeasuredWin: `BenchmarkPS5105 (11.5 KB haystack whose only match
sits at the very end, Apple M2 Pro): 172.2 ns/op -> 2.3 ns/op (~74x).
Index scans the full haystack (SIMD-assisted, but still O(len(s))) to
locate the far-away first occurrence and the == 0 comparison discards
it; HasPrefix fails at the first mismatched byte in O(len(sub)). Zero
allocations either way.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5105",
		Doc:  "strings.Index/bytes.Index compared == 0 for a prefix test instead of HasPrefix",
		Run:  runPS5105,
	},
})

func runPS5105(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok || bin.Op != token.EQL {
				return true
			}
			call, pkg, litOnLeft, ok := ps5105Match(pass, bin)
			if !ok {
				return true
			}
			// The comparison is an untyped bool and adopts whatever
			// boolean type its context demands (var f myBool = Index == 0);
			// HasPrefix returns the basic type bool. Skip when the context
			// materialized a named bool type — the rewrite would not
			// compile there.
			if tv, ok := pass.TypesInfo.Types[bin]; ok {
				if b, isBasic := tv.Type.(*types.Basic); !isBasic || b.Info()&types.IsBoolean == 0 {
					return true
				}
			}
			sel := call.Fun.(*ast.SelectorExpr)
			repl := pkg + ".HasPrefix(s, sub)"
			// The rewrite edits around the call and never touches the
			// argument text: same expressions, same evaluation order. Only
			// the selected identifier changes (Index -> HasPrefix), so an
			// aliased import keeps working — and since HasPrefix lives in
			// the same package, the import cannot be orphaned.
			var fixes []analysis.SuggestedFix
			// A comment inside the scaffolding the fix deletes (`== 0` /
			// `0 ==`) would be silently dropped — withhold the fix there
			// and report advisory only.
			commentSafe := true
			if litOnLeft {
				commentSafe = !ps2111CommentIn(f, bin.Pos(), call.Pos())
			} else {
				commentSafe = !ps2111CommentIn(f, call.End(), bin.End())
			}
			if commentSafe {
				var edits []analysis.TextEdit
				if litOnLeft {
					// `0 == Index(...)`: drop the leading `0 ==`.
					edits = append(edits, analysis.TextEdit{Pos: bin.Pos(), End: call.Pos()})
				}
				edits = append(edits, analysis.TextEdit{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("HasPrefix")})
				if !litOnLeft {
					// Drop the trailing `== 0`.
					edits = append(edits, analysis.TextEdit{Pos: call.End(), End: bin.End()})
				}
				fixes = []analysis.SuggestedFix{{
					Message:   "replace with " + repl,
					TextEdits: edits,
				}}
			}
			pass.Report(analysis.Diagnostic{
				Pos:            bin.Pos(),
				End:            bin.End(),
				Message:        pkg + ".Index(...) == 0 tests for a prefix only; " + repl + " is faster and stops at the first mismatched byte",
				SuggestedFixes: fixes,
			})
			return true
		})
	}
	return nil, nil
}

// ps5105Match reports whether bin compares a direct strings.Index or
// bytes.Index call with == against the literal integer constant 0,
// returning the call, its package ("strings" or "bytes") and which side
// the literal is on. The call operand must be the bare CallExpr (no
// parentheses) so the surrounding edit ranges are exact; the literal
// may be parenthesized (the whole literal side is deleted either way).
func ps5105Match(pass *analysis.Pass, bin *ast.BinaryExpr) (call *ast.CallExpr, pkg string, litOnLeft, ok bool) {
	if c, p := ps5105IndexCall(pass, bin.X); c != nil {
		if ps5105ZeroLit(pass, bin.Y) {
			return c, p, false, true
		}
	}
	if c, p := ps5105IndexCall(pass, bin.Y); c != nil {
		if ps5105ZeroLit(pass, bin.X) {
			return c, p, true, true
		}
	}
	return nil, "", false, false
}

// ps5105IndexCall returns e as a call of the package-level function
// strings.Index or bytes.Index along with the package name, or nil.
// Type information pins the callee to the standard library: a local
// variable, field, or method spelled Index — or a shadowed `strings` /
// `bytes` identifier — does not resolve to a receiver-less *types.Func
// with package path "strings" or "bytes" and is rejected.
func ps5105IndexCall(pass *analysis.Pass, e ast.Expr) (*ast.CallExpr, string) {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || call.Ellipsis != token.NoPos {
		return nil, ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, ""
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "Index" || fn.Pkg() == nil {
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

// ps5105ZeroLit reports whether e is (possibly parenthesized) the
// literal integer constant 0. A variable or named constant holding 0 is
// deliberately NOT matched: the check only rewrites the spelling that
// is provably a prefix test against the untyped literal.
func ps5105ZeroLit(pass *analysis.Pass, e ast.Expr) bool {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			break
		}
		e = p.X
	}
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	tv, ok := pass.TypesInfo.Types[lit]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return false
	}
	v, exact := constant.Int64Val(tv.Value)
	return exact && v == 0
}
