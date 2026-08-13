package checks

import (
	"go/ast"
	"go/constant"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2132 reports a strings.NewReplacer(...) built with constant pairs and
// immediately consumed (chained into .Replace / .WriteString), which rebuilds
// its lookup structure on every call and throws it away.
var PS2132 = register(&lint.Check{
	ID:       "PS2132",
	Category: "alloc",
	Slug:     "newreplacer-per-call",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "strings.NewReplacer with constant pairs built and discarded on every call",
		Text: `strings.NewReplacer(pairs...) is not free: it analyses the pairs and
builds a reusable lookup structure — for a handful of pairs a small
single/byte replacer, and beyond that a generic trie. Calling it INLINE, e.g.

    strings.NewReplacer("&", "&amp;", "<", "&lt;", ...).Replace(s)

rebuilds that whole structure on every call and immediately discards it. Measured
for an HTML-escape-sized pair set: ~728 ns and ~7 KB allocated PER CALL, versus
~58 ns reusing a package-level replacer — roughly 12x, and the allocation is
pure churn.

When the pairs are COMPILE-TIME CONSTANTS the replacer never changes, so it can
be built once at package scope

    var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ...)

and every call site becomes htmlEscaper.Replace(s). Only the inline form with all
constant arguments is reported — a replacer built from runtime values is
genuinely per-call, and one already stored in a variable is out of scope.

DELIBERATELY advisory — no automatic fix: lifting the constructor to a
package-level var (and naming it) is a restructuring a human should place, the
same reason PS2005/PS2127/PS2131 own the analogous regexp hoists rather than
rewriting them mechanically.`,
		Before: `out := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(in)`,
		After: `var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;") // package scope

// ... then:
out := htmlEscaper.Replace(in)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2132",
		Doc:  "strings.NewReplacer with constant pairs built inline and discarded every call",
		Run:  runPS2132,
	},
})

func runPS2132(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			// Match an outer method call whose receiver is a strings.NewReplacer
			// call: strings.NewReplacer(...).Replace(...) / .WriteString(...).
			outer, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := outer.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			inner, ok := sel.X.(*ast.CallExpr)
			if !ok {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, inner.Fun, "strings", map[string]bool{"NewReplacer": true}); !ok {
				return true
			}
			// Every pair argument must be a compile-time constant string — a
			// replacer built from runtime values is genuinely per-call.
			if len(inner.Args) == 0 || inner.Ellipsis.IsValid() {
				return true
			}
			for _, a := range inner.Args {
				v := pass.TypesInfo.Types[a].Value
				if v == nil || v.Kind() != constant.String {
					return true
				}
			}
			pass.Report(analysis.Diagnostic{
				Pos:     inner.Pos(),
				End:     inner.End(),
				Message: "strings.NewReplacer with constant pairs rebuilds its lookup structure on every call and discards it (~12x slower, ~7KB/call for many pairs); hoist `var r = strings.NewReplacer(...)` to package scope and reuse r." + sel.Sel.Name + " — advisory: introducing a package-level var is a human-placed restructuring, so it is not applied automatically",
			})
			return true
		})
	}
	return nil, nil
}
