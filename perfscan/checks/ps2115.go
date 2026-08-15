package checks

import (
	"go/ast"
	"go/constant"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2115 reports `[]rune(s)[i]` where s is a string: the conversion
// decodes and allocates the ENTIRE rune slice only to keep one element.
// Advisory only — every candidate rewrite changes panic behavior or code
// shape, so no automatic fix is attached (see the Doc for the reasoning).
//
// PS2115 is a perfscan-original check (the x1xx block per category is
// reserved for checks that did not originate in the upstream reference
// registry).
var PS2115 = register(&lint.Check{
	ID:       "PS2115",
	Category: "alloc",
	Slug:     "rune-conversion-index",
	Level:    lint.LevelIdiomatic,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "[]rune(s)[i] allocates and decodes the whole string to read one rune",
		Text: `[]rune(s) decodes every rune of s into a freshly allocated
slice; indexing the result with [i] keeps ONE element and throws the rest
away. The cost is O(len(s)) decode work on every evaluation, plus a heap
allocation once the string is longer than the compiler's 32-rune stack
buffer — all to read a single rune.

Manual remedies:

  - first rune (index 0): r, size := utf8.DecodeRuneInString(s) decodes
    only the leading rune, allocation-free.
  - a later rune: advance with utf8.DecodeRuneInString in a small loop,
    or count runes in a for-range over the string — both stop after i+1
    runes instead of decoding the entire string.

Rune values match exactly: the []rune conversion replaces each invalid
UTF-8 byte with U+FFFD, and DecodeRuneInString / a string range yield the
same replacement runes at the same positions.

WHY THERE IS NO AUTO-FIX: []rune(s)[i] PANICS when s holds fewer than
i+1 runes — []rune("")[0] in particular. utf8.DecodeRuneInString never
panics (it returns (U+FFFD, 0) for the empty string), and it returns two
values, so no bare expression can replace []rune(s)[0] in place. Even a
statement-shaped rewrite (r := []rune(s)[0] into
r, _ := utf8.DecodeRuneInString(s)) would silently swallow the
out-of-range panic unless s is proven non-empty, which is beyond a local
mechanical fix. For i > 0 the standard library has no single-expression
form at all. Every candidate rewrite therefore changes panic behavior or
requires restructuring, so the check only reports.`,
		Before: `first := []rune(name)[0]`,
		After: `first, _ := utf8.DecodeRuneInString(name)
// (rune, size); "" yields (U+FFFD, 0) where the old code panicked`,
		MeasuredWin: `BenchmarkPS2115 (one first-rune read of a ~79-rune
mixed-width line per op, Apple M2 Pro): 177.3 ns/op, 320 B/op, 1 alloc/op
before vs 2.5 ns/op, 0 B/op, 0 allocs/op after (~72x, allocation-free).
The gap grows linearly with string length; strings inside the 32-rune
stack buffer still pay the full decode pass, just not the heap.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2115",
		Doc:  "[]rune(s)[i] allocates and decodes the whole string to read one rune",
		Run:  runPS2115,
	},
})

func runPS2115(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Index expressions that are written to or have their address
		// taken read no rune at all — the advice below does not apply, so
		// they are skipped. Parents are visited before children, so the
		// set is always populated before the IndexExpr itself is reached.
		skip := map[ast.Expr]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			switch s := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range s.Lhs {
					skip[ps2115Unparen(lhs)] = true
				}
			case *ast.IncDecStmt:
				skip[ps2115Unparen(s.X)] = true
			case *ast.UnaryExpr:
				if s.Op == token.AND {
					skip[ps2115Unparen(s.X)] = true
				}
			}
			ix, ok := n.(*ast.IndexExpr)
			if !ok || skip[ix] {
				return true
			}
			conv := ps2115RuneConvOfString(pass, ix.X)
			if conv == nil {
				return true
			}
			convText := ps2115ExprText(conv)
			idxText := ps2115ExprText(ix.Index)
			argText := ps2115ExprText(conv.Args[0])
			var msg string
			if ps2115IsConstZero(pass, ix.Index) {
				msg = convText + "[" + idxText + "] decodes and allocates every rune of the string to read the first; utf8.DecodeRuneInString(" + argText + ") decodes only that rune — note it returns (rune, size) and yields (U+FFFD, 0) instead of panicking on an empty string"
			} else {
				msg = convText + "[" + idxText + "] decodes and allocates every rune of the string to read one; decode forward with utf8.DecodeRuneInString or a counted for-range over " + argText + " to stop at that rune"
			}
			pass.Report(analysis.Diagnostic{
				Pos:     ix.Pos(),
				End:     ix.End(),
				Message: msg,
			})
			return true
		})
	}
	return nil, nil
}

// ps2115RuneConvOfString returns the (paren-stripped) conversion call
// `[]rune(strExpr)` underneath e, or nil. The callee must denote a type
// whose underlying type is []rune (rune and int32 are the identical
// type, and a named rune-slice type pays the same conversion), and the
// single operand must be string-typed. Type information rejects
// function calls that merely look like conversions.
func ps2115RuneConvOfString(pass *analysis.Pass, e ast.Expr) *ast.CallExpr {
	call, ok := ps2115Unparen(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil
	}
	tv, ok := pass.TypesInfo.Types[call.Fun]
	if !ok || !tv.IsType() {
		return nil
	}
	sl, ok := tv.Type.Underlying().(*types.Slice)
	if !ok {
		return nil
	}
	elem, ok := sl.Elem().Underlying().(*types.Basic)
	if !ok || elem.Kind() != types.Int32 {
		return nil
	}
	argT := pass.TypesInfo.TypeOf(call.Args[0])
	if argT == nil {
		return nil
	}
	b, ok := argT.Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsString == 0 {
		return nil
	}
	return call
}

// ps2115IsConstZero reports whether the index is a compile-time integer
// constant equal to 0 (literal or named) — exactly the cases where the
// single-call utf8.DecodeRuneInString remedy applies.
func ps2115IsConstZero(pass *analysis.Pass, e ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[e]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.Int && constant.Sign(tv.Value) == 0
}

// ps2115Unparen strips any number of surrounding parentheses.
func ps2115Unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// ps2115ExprText renders an expression for diagnostic messages.
func ps2115ExprText(e ast.Expr) string {
	var b strings.Builder
	_ = printer.Fprint(&b, token.NewFileSet(), e)
	return b.String()
}
