package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS5062 reports for _, r := range bytes.Runes(b) — decoding a byte slice into a
// throwaway []rune only to range its runes — where for _, r := range string(b)
// yields the identical runes with no []rune allocation (the compiler ranges the
// string's bytes directly). Fires only when the range KEY is unused: over a
// []rune the key is the rune index, over a string it is the byte offset, so a
// used key would change value.
var PS5062 = register(&lint.Check{
	ID:       "PS5062",
	Category: "alloc",
	Slug:     "range-bytes-runes-to-string",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "for range bytes.Runes(b) decodes the slice into a throwaway []rune; for range string(b) yields the same runes with no allocation",
		Text: `bytes.Runes(b) decodes every rune of b into a freshly allocated []rune —
one O(len) heap allocation — and the loop then ranges that slice. Ranging
string(b) instead visits the identical rune sequence: the compiler decodes the
string's bytes in place with no intermediate slice, so the allocation disappears.

The rewrite is BIT-IDENTICAL in the runes iterated. bytes.Runes decodes b with
the same UTF-8 machinery a string range uses — each invalid byte becomes U+FFFD
consuming one byte in both — so the value sequence is identical, verified over
valid and invalid UTF-8. b is converted to a string once (evaluated exactly once,
like the single bytes.Runes call), so a later mutation of b is not observed by
either form.

The match is deliberately narrow — it is the whole safety story:
  - the ranged expression is a call of the package-level bytes.Runes with one
    argument;
  - the range KEY is blank or absent. Over a []rune the key is the rune index
    (0, 1, 2, ...); over a string it is the BYTE offset of each rune, so a used
    key would take different values — only an unused key is safe;
  - the predeclared type string is not shadowed at the site (a local named
    string would capture the conversion), and dropping bytes.Runes is withheld
    if it would orphan the bytes import (advisory then).
The value variable, if any, is a rune in both forms and carries over verbatim. A
comment inside the bytes.Runes selector withholds the fix.`,
		Before: `for _, r := range bytes.Runes(b) {`,
		After:  `for _, r := range string(b) {`,
		MeasuredWin: `On a ~112-byte mixed-script slice (Apple M2 Pro, go1.26): ` +
			`for _, r := range bytes.Runes(b) ~362 ns/op, 560 B/op, 2 allocs/op vs ` +
			`for _, r := range string(b) ~125 ns/op, 144 B/op, 1 alloc/op (~2.9x) — the []rune ` +
			`allocation is eliminated; the saving grows with the slice length.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5062",
		Doc:  "for range bytes.Runes(b) instead of for range string(b)",
		Run:  runPS5062,
	},
})

func runPS5062(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			sel *ast.SelectorExpr
			fix *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			rng, ok := n.(*ast.RangeStmt)
			if !ok || !ps2105KeyIsBlank(rng.Key) {
				return true
			}
			call, ok := rng.X.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "bytes", map[string]bool{"Runes": true}); !ok {
				return true
			}
			sel := call.Fun.(*ast.SelectorExpr)

			var fix *analysis.SuggestedFix
			// The predeclared string type must not be shadowed at the site, and
			// a comment inside the selector would be destroyed.
			if ps5062StringUnshadowed(pass, sel.Pos()) &&
				!ps2111CommentIn(f, sel.Pos(), sel.End()) {
				fix = &analysis.SuggestedFix{
					Message: "range the string directly with for range string(b)",
					TextEdits: []analysis.TextEdit{
						{Pos: sel.Pos(), End: sel.End(), NewText: []byte("string")},
					},
				}
				fixable++
			}
			sites = append(sites, site{sel, fix})
			return true
		})
		// Each fixable range drops one bytes reference; withhold all fixes if
		// that would orphan the bytes import.
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "bytes") > fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.sel.Pos(),
				End:     st.sel.End(),
				Message: "for range bytes.Runes(b) decodes the slice into a throwaway []rune; for range string(b) yields the same runes with no allocation",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5062StringUnshadowed reports whether the predeclared type string resolves to
// the universe type at pos — i.e. it is not shadowed by a local variable, type,
// or import alias that would capture the string(b) conversion.
func ps5062StringUnshadowed(pass *analysis.Pass, pos token.Pos) bool {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		scope = pass.Pkg.Scope()
	}
	_, obj := scope.LookupParent("string", pos)
	return obj == types.Universe.Lookup("string")
}
