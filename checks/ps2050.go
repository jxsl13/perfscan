package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2050 reports string(utf8.AppendRune(nil, r)) — UTF-8-encoding one rune into
// a throwaway []byte only to convert it to a string — where string(r) encodes
// the rune straight into the result string. The utf8 twin of PS2032
// (string(strconv.Append*(nil, ...)) -> Format*): both drop a throwaway
// AppendX(nil, ...) allocation plus its copy.
var PS2050 = register(&lint.Check{
	ID:       "PS2050",
	Category: "alloc",
	Slug:     "string-appendrune-nil",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string(utf8.AppendRune(nil, r)) allocates a throwaway []byte then copies it; string(r) encodes the rune directly",
		Text: `utf8.AppendRune(nil, r) grows a fresh []byte from nil and UTF-8-encodes
r into it (one heap allocation), and the surrounding string(...) conversion then
COPIES those bytes into a second heap allocation for the string. string(r) — the
predeclared rune-to-string conversion — encodes r's UTF-8 straight into the
result string in one step (the runtime uses a small on-stack buffer when the
result does not escape), so the intermediate []byte and its extra encode+copy
are eliminated: two allocations drop to at most one.

The rewrite is BIT-IDENTICAL for every rune value. utf8.AppendRune appends the
UTF-8 encoding of r, mapping every invalid rune — negative, above utf8.MaxRune,
or in the surrogate range 0xD800..0xDFFF — to the RuneError encoding U+FFFD
(0xEF 0xBF 0xBD); the predeclared string(r) conversion applies exactly that same
mapping. So string(utf8.AppendRune(nil, r)) and string(r) are the same bytes for
ASCII, multi-byte and every invalid rune. This is transitively the identity
equiv_PS5039_test already pins (append(nil, string(r)...) == utf8.AppendRune(nil,
r)); string of either side is string(r).

The match is deliberately narrow — it is the whole safety story:
  - the outer expression is a conversion to a string type (the predeclared
    string or a named type whose underlying type is string); the conversion is
    kept, so a named target T stays T(r) — T(rune) is as legal as T([]byte) for
    an underlying-string T and yields the same value;
  - the inner call is the package-level unicode/utf8.AppendRune (a shadowed utf8
    or a method never matches) with exactly two arguments and no spread;
  - the destination is the LITERAL untyped nil — a non-nil buffer, or an
    identifier merely NAMED nil that shadows a []byte, would prepend its bytes;
  - the rune argument is NON-CONSTANT, so it is a rune/int32 value (the only
    non-constant types assignable to AppendRune's rune parameter) and string(r)
    is the vet-clean rune conversion — never string(<untyped int constant>).
A comment inside the deleted inner-call scaffolding keeps the report advisory.`,
		Before: `s := string(utf8.AppendRune(nil, r))`,
		After:  `s := string(r)`,
		MeasuredWin: `string(utf8.AppendRune(nil, r)) ~23 ns/op, 16 B/op, 2 allocs/op vs ` +
			`string(r) ~8.6 ns/op, 4 B/op, 1 alloc/op (~2.7x, half the allocations; the ` +
			`throwaway []byte and its copy disappear — Apple M2 Pro, go1.26); allocation-free ` +
			`when the result does not escape.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2050",
		Doc:  "string(utf8.AppendRune(nil, r)) instead of string(r)",
		Run:  runPS2050,
	},
})

func runPS2050(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			outer, ok := n.(*ast.CallExpr)
			if !ok || len(outer.Args) != 1 || outer.Ellipsis.IsValid() {
				return true
			}
			// The outer expression must be a conversion to a string type
			// (predeclared or named-underlying-string). The conversion is kept
			// in the rewrite, so plain vs named does not matter here.
			if _, isConv := ps2032StringConv(pass, outer); !isConv {
				return true
			}
			inner, ok := ps2109Unparen(outer.Args[0]).(*ast.CallExpr)
			if !ok || inner.Ellipsis.IsValid() || len(inner.Args) != 2 {
				return true
			}
			// The inner call must be the package-level unicode/utf8.AppendRune.
			sel, ok := inner.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "AppendRune" || fn.Pkg() == nil || fn.Pkg().Path() != "unicode/utf8" {
				return true
			}
			// The destination must be the LITERAL untyped nil.
			if !ps2032IsUntypedNil(pass, inner.Args[0]) {
				return true
			}
			// The rune argument must be non-constant, so string(r) is the
			// vet-clean rune->string conversion (never string(untyped int)).
			r := inner.Args[1]
			if tv, ok := pass.TypesInfo.Types[r]; !ok || tv.Value != nil {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     outer.Pos(),
				End:     outer.End(),
				Message: "string(utf8.AppendRune(nil, r)) UTF-8-encodes r into a throwaway []byte and copies it into a string; string(r) encodes the rune directly",
			}
			// The fix replaces the inner call with its rune argument, keeping the
			// outer string() conversion and r byte-verbatim. Deleted regions:
			// inner.Pos()..r.Pos() ("utf8.AppendRune(nil, ") and r.End()..
			// inner.End() (the trailing ")"). A comment in either withholds it.
			if !ps2109CommentBetween(f, inner.Pos(), r.Pos()) &&
				!ps2109CommentBetween(f, r.End(), inner.End()) {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace with string(r)",
					TextEdits: []analysis.TextEdit{
						{Pos: inner.Pos(), End: r.Pos()},
						{Pos: r.End(), End: inner.End()},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}
