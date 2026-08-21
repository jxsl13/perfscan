package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2037 reports string([]rune{r}) — a single-element rune slice built
// purely to be converted to a string — and rewrites it to string(rune(r)),
// which encodes the one rune directly through the runtime's single-rune
// fast path with no slice header, no backing array and no slice loop.
// The composite-literal sibling of the conversion round-trips PS2108/
// PS2024: here nothing is round-tripped, the slice is scaffolding from
// the moment it is built.
var PS2037 = register(&lint.Check{
	ID:       "PS2037",
	Category: "alloc",
	Slug:     "single-rune-slice-string",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string([]rune{r}) builds a throwaway 1-element rune slice; string(rune(r)) encodes the rune directly",
		Text: `string([]rune{r}) materializes a []rune backing array and a
slice header just to hand one rune to the []rune->string conversion,
which dispatches to the generic runtime.slicerunetostring: a counting
pass over the slice, then an encoding pass — and the one-element
backing array can escape and heap-allocate on top of the result
string. string(rune(r)) is the direct rune->string conversion: the
runtime's single-rune path encodes r into a small on-stack buffer and
allocates only the result string. Strictly less work per call — no
slice construction, no slice header, no generic two-pass slice loop —
and one fewer potential allocation.

The rewrite is bit-identical for EVERY int32 value. A []rune composite
literal converts its single element to type rune exactly as the
explicit rune(E) conversion does (the element must be assignable to
rune, so E is a rune/int32 value, an alias of it, or an untyped
constant representable as one — a named integer type is not assignable
and never matches), and string([]rune{x}) with a one-element slice runs
the identical UTF-8 encoder on the same rune value as string(rune(x)):
ASCII, multi-byte, and every invalid rune — negative, above
utf8.MaxRune, the surrogate range — encode to the same bytes (invalid
runes become the U+FFFD replacement encoding in both forms). E is
evaluated exactly once in both spellings, so side effects are preserved
in count and order. No String()/Error()/Format() method can intercept:
both spellings are builtin conversions on an integer value, not fmt
paths. The replacement rune(E) is itself a call-shaped primary sitting
in the unchanged argument slot of the unchanged outer conversion, so it
never needs parentheses.

The automatic fix applies only when type information proves the exact
shape:

  - the outer conversion is the predeclared string — the identifier
    resolves to the universe string and is a type, not a same-named
    function or a named string type;
  - the argument is DIRECTLY a composite literal whose type is spelled
    as the slice type []rune (or []int32 — the identical type), with
    the element-type identifier resolving to the universe rune/int32; a
    named or aliased slice type stays out (deleting its spelling could
    orphan an import, and the narrow spelling is the whole shape);
  - the literal has EXACTLY one element, positionally (a keyed element
    like []rune{5: r} pads the slice with zero runes and is a different
    string; more or fewer elements are a different string too), and the
    element's type is exactly rune/int32 (untyped constants take the
    element type from context);
  - a slice stored in a variable first may have other consumers and is
    out of scope.

The fix keeps the element byte-verbatim in place — same text, same
single evaluation — deletes the [] before the element type, and turns
the literal's braces into the conversion's parentheses, so []rune{r}
becomes rune(r) and a literal spelled []int32{r} becomes the identical
int32(r). Because the element-type identifier is reused verbatim at its
original source position, a shadowed rune/int32 can never be captured —
the identifier already failed the universe check in that case. No
import is added or removed. A comment inside the deleted scaffolding
would be destroyed by the edits — the fix is withheld there and the
report stays advisory.`,
		Before: `s := string([]rune{r})`,
		After:  `s := string(rune(r))`,
		MeasuredWin: `BenchmarkPS2037 (one rune per op, cycling ASCII,
multi-byte, 4-byte and invalid runes, Apple M2 Pro, gc 1.26):
string([]rune{r}) 16.2 ns/op, 8 B/op, 1 allocs/op ->
string(rune(r)) 12.9 ns/op, 4 B/op, 1 allocs/op fully inlined; behind a
non-inlined call boundary the gap widens to 26.2 -> 11.0 ns/op (~2.4x)
— runtime.slicerunetostring's two-pass slice loop and the slice
construction disappear, leaving the single-rune encoder. When the slice
escapes, the Before pays a second allocation (10 B, 2 allocs/op) for
the backing array; the After never allocates more than the result
string.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2037",
		Doc:  "string([]rune{r}) builds a throwaway single-element rune slice; string(rune(r)) encodes the rune directly",
		Run:  runPS2037,
	},
})

func runPS2037(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			// The outer conversion must be the predeclared string —
			// ps2024StringConv resolves the identifier to the universe
			// string and confirms it is used as a type (a shadowing local
			// or a named string type never matches).
			conv, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			call := ps2024StringConv(pass, conv)
			if call == nil {
				return true
			}
			lit, eltType, elem := ps2037SingleRuneLit(pass, ps2108Unparen(call.Args[0]))
			if lit == nil {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "string(" + ps2125ExprText(lit) + ") builds a throwaway single-element rune slice just to encode one rune; string(" + eltType.Name + "(" + ps2125ExprText(elem) + ")) encodes it directly with no slice",
			}
			// A comment inside the deleted scaffolding — between the
			// literal's start and its element type, inside the braces
			// before the element, or after it — would be destroyed by the
			// edits: the fix is withheld and the report stays advisory.
			// The element itself stays byte-verbatim, so comments inside
			// it are preserved.
			if !ps2111CommentIn(f, lit.Pos(), eltType.Pos()) &&
				!ps2111CommentIn(f, eltType.End(), elem.Pos()) &&
				!ps2111CommentIn(f, elem.End(), lit.End()) {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "encode the rune directly with string(" + eltType.Name + "(...)) instead of a 1-element []rune",
					TextEdits: []analysis.TextEdit{
						// []rune{r} -> rune(r): delete the [], keep the
						// element-type identifier and the element verbatim
						// at their original positions, and turn the braces
						// (with any trailing comma) into parentheses.
						{Pos: lit.Pos(), End: eltType.Pos(), NewText: nil},
						{Pos: eltType.End(), End: elem.Pos(), NewText: []byte("(")},
						{Pos: elem.End(), End: lit.End(), NewText: []byte(")")},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2037SingleRuneLit matches e as a composite literal []rune{E} (or the
// identical []int32{E}) with EXACTLY one positional element: the type is
// spelled as a slice type whose element-type identifier resolves to the
// universe rune or int32 (a named or aliased slice type, or a shadowed
// rune, never matches), the literal has exactly one element, that element
// is not keyed ([]rune{5: r} pads the slice with zero runes — a different
// string), and the element's type is exactly rune/int32 (an untyped
// constant takes the element type from context; a named integer type is
// not assignable to rune and cannot appear). Returns the literal, the
// element-type identifier reused verbatim by the fix, and the element.
func ps2037SingleRuneLit(pass *analysis.Pass, e ast.Expr) (*ast.CompositeLit, *ast.Ident, ast.Expr) {
	lit, ok := e.(*ast.CompositeLit)
	if !ok || len(lit.Elts) != 1 {
		return nil, nil, nil
	}
	arr, ok := lit.Type.(*ast.ArrayType)
	if !ok || arr.Len != nil {
		return nil, nil, nil
	}
	eltType, ok := arr.Elt.(*ast.Ident)
	if !ok {
		return nil, nil, nil
	}
	if obj := pass.TypesInfo.Uses[eltType]; obj != types.Universe.Lookup("rune") && obj != types.Universe.Lookup("int32") {
		return nil, nil, nil
	}
	elem := lit.Elts[0]
	if _, keyed := elem.(*ast.KeyValueExpr); keyed {
		return nil, nil, nil
	}
	// Belt and suspenders: the element's type (in context) must be
	// exactly rune/int32 — assignability already guarantees it, but a
	// missing type entry bails out rather than fixing blind.
	t := pass.TypesInfo.TypeOf(elem)
	if t == nil || !types.Identical(types.Default(t), types.Typ[types.Int32]) {
		return nil, nil, nil
	}
	return lit, eltType, elem
}
