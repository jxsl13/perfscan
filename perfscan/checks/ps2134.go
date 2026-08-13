package checks

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2134 reports template.New(name).Parse(constPattern) built inline inside a
// function — each call recreates and re-parses a template whose text is fixed at
// compile time.
var PS2134 = register(&lint.Check{
	ID:       "PS2134",
	Category: "alloc",
	Slug:     "template-parse-per-call",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "text/template or html/template New().Parse() of a constant template built inline every call",
		Text: `template.New(name).Parse(text) tokenises and compiles the template
text into a parse tree. Doing it INLINE inside a function — the classic

    t := template.Must(template.New("page").Parse(pageTmpl))
    t.Execute(w, data)

in a request handler — recompiles that tree on every call and discards it.
Measured for a small template: ~3100 ns and ~4.5 KB / 65 allocations PER CALL for
the parse+execute, versus ~500 ns / 10 allocations executing a template parsed
once — the parse is the bulk of the cost.

When the template TEXT is a COMPILE-TIME CONSTANT the parse tree never changes,
so it belongs at package scope:

    var pageTemplate = template.Must(template.New("page").Parse(pageTmpl))

and every call site becomes pageTemplate.Execute(w, data). Both text/template and
html/template are covered.

Only the inline template.New(name).Parse(constText) chain INSIDE A FUNCTION is
reported: a package/file-scope var initializer already parses once at init; a
template parsed from a runtime string genuinely varies; and t.Clone() (the
correct per-request copy of a base template) is not a New().Parse() and is left
alone.

DELIBERATELY advisory — no automatic fix: Parse returns (*Template, error), and
lifting the constructor to a named package-level var (deciding Must vs explicit
error handling) is a restructuring for a human, the same reason PS2131/PS2132/
PS2133 own the analogous regexp / NewReplacer / LoadLocation hoists.`,
		Before: `t := template.Must(template.New("page").Parse(pageTmpl)) // re-parses every call
t.Execute(w, data)`,
		After: `var pageTemplate = template.Must(template.New("page").Parse(pageTmpl)) // package scope

// ... then:
pageTemplate.Execute(w, data)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2134",
		Doc:  "template.New().Parse() of a constant template built inline every call",
		Run:  runPS2134,
	},
})

// ps2134TemplateNew reports whether e is a call to New in either the
// text/template or html/template package, returning the import path. Resolution
// is by import PATH (via types), so an aliased import — e.g.
// `htmltmpl "html/template"` — is handled and a shadowed `template` identifier
// is rejected. (astutil.PkgFuncCall matches the qualifier NAME, which for these
// multi-segment paths is just "template", so it cannot be used here.)
func ps2134TemplateNew(info *types.Info, e ast.Expr) (string, bool) {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "New" {
		return "", false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	pn, ok := info.Uses[id].(*types.PkgName)
	if !ok {
		return "", false
	}
	switch pn.Imported().Path() {
	case "text/template", "html/template":
		return pn.Imported().Path(), true
	}
	return "", false
}

func runPS2134(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			// The Parse method call: X.Parse(text).
			parse, ok := n.(*ast.CallExpr)
			if !ok || len(parse.Args) != 1 {
				return true
			}
			sel, ok := parse.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Parse" {
				return true
			}
			// Receiver must be a fresh template.New(...) call — this is the
			// inline create-and-parse chain, not a Parse added to an existing
			// template and not a Clone.
			newCall, ok := sel.X.(*ast.CallExpr)
			if !ok {
				return true
			}
			pkgPath, ok := ps2134TemplateNew(pass.TypesInfo, newCall.Fun)
			if !ok {
				return true
			}
			// The template text must be a compile-time constant — a runtime
			// string genuinely varies per call.
			v := pass.TypesInfo.Types[parse.Args[0]].Value
			if v == nil || v.Kind() != constant.String {
				return true
			}
			// Only a call inside a function body is per-call; a package/file
			// scope var initializer parses once at init.
			inFunc := false
			for _, a := range stack {
				switch a.(type) {
				case *ast.FuncDecl, *ast.FuncLit:
					inFunc = true
				}
			}
			if !inFunc {
				return true
			}
			pkgName := "text/template"
			if pkgPath == "html/template" {
				pkgName = "html/template"
			}
			pass.Report(analysis.Diagnostic{
				Pos:     newCall.Pos(),
				End:     parse.End(),
				Message: pkgName + ": New(...).Parse of a constant template inline re-parses it on every call (~3µs, ~4.5KB, dozens of allocations); parse it once into a package-level var (`var t = template.Must(template.New(name).Parse(text))`) and reuse t.Execute — advisory: the (*Template, error) return and a package var are a human-placed restructuring, so it is not applied automatically",
			})
			return true
		})
	}
	return nil, nil
}
