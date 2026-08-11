package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2108 reports the conversion round-trip string([]byte(x)) where x is
// statically a string: strings are immutable, so the byte slice is a
// throwaway copy and the whole expression equals x.
var PS2108 = register(&lint.Check{
	ID:       "PS2108",
	Category: "alloc",
	Slug:     "redundant-string-byte-roundtrip",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string([]byte(s)) round-trips a string through a byte slice for nothing",
		Text: `string([]byte(x)) with x already a string copies the string's
bytes into a fresh slice and then copies that slice into a fresh string —
two allocations and two memmoves to produce a value bit-identical to x.
Strings are immutable and nothing can mutate the intermediate slice (it is
unnamed and dies inside the expression), so the round-trip is pure waste.

The automatic fix rewrites string([]byte(x)) to x. It applies only when
every part is proven by type information:

  - the outer call resolves to the PREDECLARED string conversion (a named
    string type or a shadowed identifier spelled string does not match —
    a named result type would change the expression's static type);
  - the inner call is a conversion to exactly []byte (unnamed slice of
    the predeclared byte; a defined slice type is not matched);
  - the inner argument's static type is exactly the predeclared string —
    a NAMED string argument is not reported at all, because there the
    round-trip changes the static type (named -> string) and removing it
    would change assignability and interface boxing.

The argument stays in place, evaluated exactly once either way, and is
re-parenthesized when it does not bind as tightly as the call it replaces.
A constant argument keeps an advisory report without a fix: substituting a
constant for a non-constant expression could, for example, create a
duplicate case in a switch that previously compiled. The reverse spelling
[]byte(string(b)) is never touched — that copy un-aliases the slice and
may be relied upon. The fix removes only text, so it is import-neutral.`,
		Before: `key := string([]byte(name))`,
		After:  `key := name`,
		MeasuredWin: `BenchmarkPS2108 (1024 short strings, Apple M2 Pro):
9496 ns/op, 1024 allocs/op -> 522 ns/op, 0 allocs/op (~18x). The After is
a plain string assignment: the entire round-trip cost is eliminated.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2108",
		Doc:  "string([]byte(x)) of a string value equals x and copies twice for nothing",
		Run:  runPS2108,
	},
})

func runPS2108(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			outer, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			arg, fixable := ps2108Match(pass, outer)
			if arg == nil {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     outer.Pos(),
				End:     outer.End(),
				Message: "string([]byte(x)) round-trips a string through a throwaway byte slice (two copies); the expression equals x — use the string directly",
			}
			// The fix deletes the two conversion wrappers and leaves the
			// argument text untouched in place: same expression, evaluated
			// once either way, comments inside the argument preserved. A
			// comment sitting in the deleted wrapper text would be lost, so
			// such a site keeps the advisory report only.
			if fixable && !ps2108CommentsInWrappers(f, outer, arg) {
				lead, trail := "", ""
				if !ps2108Primary(arg) {
					// The call is a primary expression; a looser-binding
					// argument (a+b, *p, <-ch) must keep its grouping in
					// every syntactic context (indexing, slicing, selectors).
					lead, trail = "(", ")"
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "drop the string([]byte(...)) round-trip",
					TextEdits: []analysis.TextEdit{
						{Pos: outer.Pos(), End: arg.Pos(), NewText: []byte(lead)},
						{Pos: arg.End(), End: outer.End(), NewText: []byte(trail)},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2108Match matches outer against string([]byte(x)) with x statically a
// string. It returns the inner argument (nil when the shape does not
// match) and whether the rewrite is provably safe to apply automatically.
// A constant argument (typed or untyped string constant) is reported but
// not fixable: substituting a constant expression for a non-constant one
// can change compile-time properties (e.g. duplicate switch cases).
func ps2108Match(pass *analysis.Pass, outer *ast.CallExpr) (arg ast.Expr, fixable bool) {
	if len(outer.Args) != 1 || outer.Ellipsis.IsValid() {
		return nil, false
	}
	if !ps2108IsUniverseString(pass, outer.Fun) {
		return nil, false
	}
	inner, ok := ps2108Unparen(outer.Args[0]).(*ast.CallExpr)
	if !ok || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
		return nil, false
	}
	if !ps2108IsByteSliceConv(pass, inner.Fun) {
		return nil, false
	}
	a := inner.Args[0]
	tv, ok := pass.TypesInfo.Types[a]
	if !ok || tv.Type == nil {
		return nil, false
	}
	b, ok := types.Unalias(tv.Type).(*types.Basic)
	if !ok || b.Info()&types.IsString == 0 {
		// A NAMED string type is deliberately not reported: there the
		// round-trip converts named -> string, so removing it would change
		// the expression's static type.
		return nil, false
	}
	// Fix only the exact predeclared string, and only non-constant values.
	return a, b.Kind() == types.String && tv.Value == nil
}

// ps2108IsUniverseString reports whether fun resolves to the predeclared
// string type — the universe object itself, so a shadowed identifier, a
// named string type, or an alias does not match.
func ps2108IsUniverseString(pass *analysis.Pass, fun ast.Expr) bool {
	id, ok := ps2108Unparen(fun).(*ast.Ident)
	if !ok {
		return false
	}
	return pass.TypesInfo.Uses[id] == types.Universe.Lookup("string")
}

// ps2108IsByteSliceConv reports whether fun is a type expression whose
// type is exactly []byte: an unnamed slice of the predeclared byte.
// Aliases of []byte match (identical type); defined slice types and named
// byte elements do not.
func ps2108IsByteSliceConv(pass *analysis.Pass, fun ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[ps2108Unparen(fun)]
	if !ok || !tv.IsType() {
		return false
	}
	sl, ok := types.Unalias(tv.Type).(*types.Slice)
	if !ok {
		return false
	}
	eb, ok := types.Unalias(sl.Elem()).(*types.Basic)
	return ok && eb.Kind() == types.Byte
}

// ps2108Unparen strips any parentheses around e.
func ps2108Unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// ps2108Primary reports whether e binds at least as tightly as the call
// expression it replaces, so no parentheses are needed around it.
func ps2108Primary(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.BasicLit, *ast.CallExpr,
		*ast.IndexExpr, *ast.SliceExpr, *ast.ParenExpr, *ast.CompositeLit:
		return true
	}
	return false
}

// ps2108CommentsInWrappers reports whether a comment overlaps the wrapper
// text the fix would delete — everything in the call except the argument.
func ps2108CommentsInWrappers(f *ast.File, outer *ast.CallExpr, arg ast.Expr) bool {
	for _, cg := range f.Comments {
		if cg.End() > outer.Pos() && cg.Pos() < arg.Pos() {
			return true
		}
		if cg.End() > arg.End() && cg.Pos() < outer.End() {
			return true
		}
	}
	return false
}
