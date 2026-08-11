package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5101 reports bytes.Compare(a, b) compared against the literal 0 with
// == or != — an equality test spelled through a three-way comparison.
var PS5101 = register(&lint.Check{
	ID:       "PS5101",
	Category: "arith",
	Slug:     "bytes-compare-equality",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "bytes.Compare used only for equality, where bytes.Equal is faster",
		Text: `bytes.Compare computes a full three-way ordering; testing its
result against 0 throws the ordering away and keeps only equality.
bytes.Equal answers exactly that question with optimized assembly: it
short-circuits on a length mismatch without touching a single byte
(Compare must scan the whole common prefix first) and its equal-scan is
the runtime's memequal, tuned harder than Compare's ordering loop.

For any a, b: bytes.Equal(a, b) == (bytes.Compare(a, b) == 0). Both
treat nil and empty slices identically, read the same bytes, write
nothing, and cannot panic — the rewrite is bit-identical.

The automatic fix rewrites bytes.Compare(a, b) == 0 to
bytes.Equal(a, b) and bytes.Compare(a, b) != 0 to !bytes.Equal(a, b),
with the 0 on either side. The arguments are left untouched in place
(same evaluation, same order) and only the selected name changes, so an
aliased bytes import is preserved. bytes.Equal keeps the bytes import in
use, so the fix can never orphan an import. Only a comparison against
the literal constant 0 is matched: a variable or named constant holding
0, and every ordering operator (<, <=, >, >=), are left alone —
bytes.Compare used for ordering is exactly what Compare is for.`,
		Before: `if bytes.Compare(a, b) == 0 {
	return true
}`,
		After: `if bytes.Equal(a, b) {
	return true
}`,
		MeasuredWin: `BenchmarkPS5101 (one 1152-byte equal pair plus one
proper-prefix pair, Apple M2 Pro): 33.6 ns/op -> 22.6 ns/op (~1.5x).
On a length mismatch bytes.Equal is O(1) while Compare still scans the
whole common prefix, so long shared-prefix inputs win by far more.
Zero allocations either way.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5101",
		Doc:  "bytes.Compare compared against 0 instead of bytes.Equal",
		Run:  runPS5101,
	},
})

func runPS5101(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
				return true
			}
			call, zeroOnLeft, ok := ps5101Match(pass, bin)
			if !ok {
				return true
			}
			sel := call.Fun.(*ast.SelectorExpr)
			op, repl := "==", "bytes.Equal(a, b)"
			if bin.Op == token.NEQ {
				op, repl = "!=", "!bytes.Equal(a, b)"
			}
			// The rewrite edits around the call and never touches the
			// argument text: same expressions, same evaluation order. Only
			// the selected identifier changes (Compare -> Equal), so an
			// aliased bytes import keeps working — and since bytes.Equal
			// still uses the package, the import cannot be orphaned.
			var edits []analysis.TextEdit
			if zeroOnLeft {
				// `0 == call` / `0 != call`: fold the leading `0 ==` into
				// the (optional) negation.
				lead := ""
				if bin.Op == token.NEQ {
					lead = "!"
				}
				edits = append(edits, analysis.TextEdit{Pos: bin.Pos(), End: call.Pos(), NewText: []byte(lead)})
			} else if bin.Op == token.NEQ {
				edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: call.Pos(), NewText: []byte("!")})
			}
			edits = append(edits, analysis.TextEdit{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Equal")})
			if !zeroOnLeft {
				// Drop the trailing `== 0` / `!= 0`.
				edits = append(edits, analysis.TextEdit{Pos: call.End(), End: bin.End()})
			}
			pass.Report(analysis.Diagnostic{
				Pos:     bin.Pos(),
				End:     bin.End(),
				Message: "bytes.Compare(...) " + op + " 0 tests equality only; " + repl + " is faster and computes no ordering",
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

// ps5101Match reports whether bin compares a direct bytes.Compare call
// against the literal constant 0, returning the call and which side the
// zero is on. The call operand must be the bare CallExpr (no parentheses)
// so the surrounding edit ranges are exact; the zero may be parenthesized
// (the whole zero side is deleted either way).
func ps5101Match(pass *analysis.Pass, bin *ast.BinaryExpr) (call *ast.CallExpr, zeroOnLeft bool, ok bool) {
	if c := ps5101CompareCall(pass, bin.X); c != nil && ps5101IsZeroLit(pass, bin.Y) {
		return c, false, true
	}
	if c := ps5101CompareCall(pass, bin.Y); c != nil && ps5101IsZeroLit(pass, bin.X) {
		return c, true, true
	}
	return nil, false, false
}

// ps5101CompareCall returns e as a call of the package-level function
// bytes.Compare, or nil. Type information pins the callee to the standard
// library's bytes package: a local variable, field, or method spelled
// Compare — or a shadowed `bytes` identifier — does not resolve to a
// receiver-less *types.Func with package path "bytes" and is rejected.
func ps5101CompareCall(pass *analysis.Pass, e ast.Expr) *ast.CallExpr {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || call.Ellipsis != token.NoPos {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "Compare" || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
		return nil
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return nil
	}
	return call
}

// ps5101IsZeroLit reports whether e is (possibly parenthesized) the
// literal integer constant 0. A variable or named constant holding 0 is
// deliberately NOT matched: the check only rewrites the spelling that is
// provably an equality test against the untyped constant 0.
func ps5101IsZeroLit(pass *analysis.Pass, e ast.Expr) bool {
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
	return ok && tv.Value != nil && tv.Value.Kind() == constant.Int && constant.Sign(tv.Value) == 0
}
