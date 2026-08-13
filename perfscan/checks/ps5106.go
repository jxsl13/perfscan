package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5106 reports strings.Compare(a, b) compared against the literal 0, where
// the direct comparison operator is bit-identical, allocation-free, and the
// form the standard library itself recommends.
var PS5106 = register(&lint.Check{
	ID:       "PS5106",
	Category: "arith",
	Slug:     "strings-compare-to-operator",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Compare(a, b) compared to 0 where the direct operator is clearer and faster",
		Text: `strings.Compare(a, b) returns exactly -1, 0 or +1 — it is defined
by the comparison operators — so testing its result against the literal 0 is a
round-trip through a function call for something the operators already express:

  strings.Compare(a, b) == 0  ->  a == b
  strings.Compare(a, b) != 0  ->  a != b
  strings.Compare(a, b) <  0  ->  a <  b
  strings.Compare(a, b) <= 0  ->  a <= b
  strings.Compare(a, b) >  0  ->  a >  b
  strings.Compare(a, b) >= 0  ->  a >= b

The literal may sit on either side; when strings.Compare is the RIGHT operand
the operator is mirrored (0 < Compare(a,b) means Compare(a,b) > 0, i.e. a > b).
Both spellings read a and b exactly once in the same order, allocate nothing,
and cannot panic, so the rewrite is behavior-preserving. It is also the form the
standard library documents: "Compare is included only for symmetry with package
bytes; it is usually clearer and always faster to use the built-in string
comparison operators." (bytes.Compare(a,b)==0 has a dedicated bytes.Equal — that
is PS5101; strings has no Equal, so this rewrites to the operator instead.)

The automatic fix replaces the whole comparison with a OP b, splicing the two
argument expressions verbatim. A named type could not change the meaning here:
strings.Compare takes plain strings and the result is an untyped bool exactly
like a == b, so no type guard is needed. The report stays advisory when a
comment sits inside the replaced range (the re-rendered expression could not
reproduce it) and, in a cgo file whose import block must not be edited, when the
rewrite would orphan the strings import.`,
		Before: `if strings.Compare(a, b) == 0 {
	return true
}`,
		After: `if a == b {
	return true
}`,
		MeasuredWin: `The operator comparison skips the strings.Compare call
entirely — a plain machine compare instead of a function call that computes a
full three-way ordering. Zero allocations either way.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5106",
		Doc:  "strings.Compare compared against 0 instead of the direct comparison operator",
		Run:  runPS5106,
	},
})

// ps5106OpString renders a comparison operator, or "" if op is not one.
func ps5106OpString(op token.Token) string {
	switch op {
	case token.EQL:
		return "=="
	case token.NEQ:
		return "!="
	case token.LSS:
		return "<"
	case token.LEQ:
		return "<="
	case token.GTR:
		return ">"
	case token.GEQ:
		return ">="
	}
	return ""
}

// ps5106Mirror flips an ordering operator (< <-> >, <= <-> >=); == and != are
// symmetric and returned unchanged. Used when strings.Compare is the RIGHT
// operand of the comparison with 0.
func ps5106Mirror(op token.Token) token.Token {
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

func runPS5106(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			bin *ast.BinaryExpr
			msg string
			fix *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok || ps5106OpString(bin.Op) == "" {
				return true
			}
			// Exactly one side is strings.Compare(a, b), the other the literal 0.
			var call *ast.CallExpr
			litOnLeft := false
			if c := ps5106CompareCall(pass, bin.X); c != nil && ps5105ZeroLit(pass, bin.Y) {
				call, litOnLeft = c, false
			} else if c := ps5106CompareCall(pass, bin.Y); c != nil && ps5105ZeroLit(pass, bin.X) {
				call, litOnLeft = c, true
			}
			if call == nil {
				return true
			}
			// When Compare is the right operand the operator is mirrored so the
			// meaning is preserved (0 < Compare(a,b)  <=>  a > b).
			op := bin.Op
			if litOnLeft {
				op = ps5106Mirror(op)
			}
			opStr := ps5106OpString(op)
			msg := "strings.Compare(a, b) " + ps5106OpString(bin.Op) + " 0 round-trips a function call for what `a " + opStr + " b` expresses directly (faster, no ordering computed)"

			var fix *analysis.SuggestedFix
			aText, okA := ps2107ExprText(call.Args[0])
			bText, okB := ps2107ExprText(call.Args[1])
			// A comment inside the replaced comparison would be silently dropped
			// by the re-rendered expression — withhold the fix there.
			if okA && okB && !ps2111CommentIn(f, bin.Pos(), bin.End()) {
				repl := aText + " " + opStr + " " + bText
				fix = &analysis.SuggestedFix{
					Message:   "replace with " + repl,
					TextEdits: []analysis.TextEdit{{Pos: bin.Pos(), End: bin.End(), NewText: []byte(repl)}},
				}
				fixable++
			}
			sites = append(sites, site{bin, msg, fix})
			return true
		})
		// Removing strings.Compare may orphan the strings import; the fix
		// pipeline prunes it — except in a cgo file, whose import block must not
		// be edited. There, withhold the fixes only when they would orphan
		// strings (no other strings reference survives).
		emitFixes := pkgRefCount(pass, f, "strings") > fixable || !ps2110ImportsC(f)
		for _, st := range sites {
			diag := analysis.Diagnostic{Pos: st.bin.Pos(), End: st.bin.End(), Message: st.msg}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5106CompareCall returns e as a call of the package-level function
// strings.Compare, or nil. Type information pins the callee to the standard
// library: a shadowed strings identifier or a local Compare method does not
// resolve to a receiver-less *types.Func in package "strings".
func ps5106CompareCall(pass *analysis.Pass, e ast.Expr) *ast.CallExpr {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || call.Ellipsis != token.NoPos {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "Compare" || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
		return nil
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return nil
	}
	return call
}
