package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2134 reports template.New(name).Parse(constPattern) built inline inside a
// function — each call recreates and re-parses a template whose text is fixed at
// compile time.
//
// When the parse is wrapped in template.Must(...) (so the value is a plain
// *Template) and the resulting template is used READ-ONLY — either inline as the
// receiver of Execute/ExecuteTemplate, or bound to a local that is only ever
// executed — the fix hoists the whole template.Must(template.New(name).Parse(
// text)) expression to a package-level var parsed once at init, mirroring PS2127
// and PS2132. A template that is only executed is safe to share (concurrent
// Execute is documented safe, and the rendered output is byte-for-byte identical
// to a fresh per-call parse); a template that is later re-Parsed, extended via
// New, passed on, reassigned, or address-taken could be mutated through the
// shared instance, so those keep the advisory report and no fix.
var PS2134 = register(&lint.Check{
	ID:       "PS2134",
	Category: "alloc",
	Slug:     "template-parse-per-call",
	Level:    lint.LevelStructured,
	AutoFix:  true,
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

The automatic fix performs exactly that hoist when it is provably safe: the parse
is wrapped in template.Must(...) (a single *Template value), and the template is
used READ-ONLY — inline as the receiver of Execute/ExecuteTemplate, or bound to a
local that is only ever executed (never re-Parsed, extended via New, reassigned,
address-taken, or passed on). A template that is only executed is safe to share:
concurrent Execute is documented safe, and the rendered output is byte-for-byte
identical to a fresh per-call parse.

Only the inline template.New(name).Parse(constText) chain INSIDE A FUNCTION is
reported: a package/file-scope var initializer already parses once at init; a
template parsed from a runtime string genuinely varies; and t.Clone() (the
correct per-request copy of a base template) is not a New().Parse() and is left
alone. When the safe-shape conditions do not hold — no Must wrapper, or the
template escapes / is mutated — the finding stays advisory with no fix.

L2 (structured): the fix introduces a new package-level declaration. For a
constant template used read-only the runtime behavior is identical.`,
		Before: `func render(w io.Writer, data any) {
	t := template.Must(template.New("page").Parse(pageTmpl))
	t.Execute(w, data)
}`,
		After: `var psTemplateL1 = template.Must(template.New("page").Parse(pageTmpl))

func render(w io.Writer, data any) {
	t := psTemplateL1
	t.Execute(w, data)
}`,
		MeasuredWin: "a constant template parsed once at init instead of on every call: the per-call parse (~3100 ns and ~4.5 KB / 65 allocations for a small template) drops to zero after startup, roughly 6x on parse+execute",
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
	// Names generated this run, so two fixes never collide on one package scope.
	used := map[string]bool{}
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
			if !ps2134InFunc(stack) {
				return true
			}
			pkgName := "text/template"
			if pkgPath == "html/template" {
				pkgName = "html/template"
			}
			diag := analysis.Diagnostic{
				Pos:     newCall.Pos(),
				End:     parse.End(),
				Message: pkgName + ": New(...).Parse of a constant template inline re-parses it on every call (~3µs, ~4.5KB, dozens of allocations); parse it once into a package-level var (`var t = template.Must(template.New(name).Parse(text))`) and reuse t.Execute",
			}
			if fix, ok := ps2134HoistFix(pass, stack, parse, pkgPath, used); ok {
				diag.SuggestedFixes = []analysis.SuggestedFix{fix}
			} else {
				diag.Message += " — advisory: not auto-hoisted (no template.Must wrapper, or the template is mutated / escapes rather than only executed)"
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2134InFunc reports whether the current node sits inside a function body
// rather than a package/file-scope declaration initializer.
func ps2134InFunc(stack []ast.Node) bool {
	for _, a := range stack {
		switch a.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return true
		}
	}
	return false
}

// ps2134ReadOnlyTemplateMethod reports whether name is a *Template method that
// neither mutates the template nor exposes internal sub-templates that could be
// mutated — the only methods whose presence keeps a shared package instance
// behavior-identical to a fresh per-call one.
func ps2134ReadOnlyTemplateMethod(name string) bool {
	switch name {
	case "Execute", "ExecuteTemplate", "Name", "DefinedTemplates":
		return true
	}
	return false
}

// ps2134HoistFix builds the two-edit transform (insert a package-level binding
// before the enclosing top-level function, replace the inline
// template.Must(New(name).Parse(text)) expression with the new variable) when the
// shape is provably safe: the parse is wrapped in template.Must, and the template
// is used read-only either inline or through a single bound local.
func ps2134HoistFix(pass *analysis.Pass, stack []ast.Node, parse *ast.CallExpr, pkgPath string, used map[string]bool) (analysis.SuggestedFix, bool) {
	// The parse must be the sole argument of a template.Must(...) wrapper, so the
	// value to hoist is a plain *Template (not a (*Template, error) tuple).
	// WithStack's stack excludes the node itself, so stack[len-1] is the parse
	// call's immediate parent — the template.Must(...) call.
	if len(stack) < 1 {
		return analysis.SuggestedFix{}, false
	}
	mustCall, ok := stack[len(stack)-1].(*ast.CallExpr)
	if !ok || len(mustCall.Args) != 1 || mustCall.Args[0] != ast.Expr(parse) {
		return analysis.SuggestedFix{}, false
	}
	if !ps2134IsTemplateMust(pass.TypesInfo, mustCall.Fun, pkgPath) {
		return analysis.SuggestedFix{}, false
	}
	// The Must(...) value must be used read-only: inline as the receiver of a
	// read-only method, or bound to a single local that is only ever executed.
	if !ps2134MustUsedReadOnly(pass.TypesInfo, stack, mustCall) {
		return analysis.SuggestedFix{}, false
	}
	fn, ok := ps2127EnclosingTopFunc(stack)
	if !ok {
		return analysis.SuggestedFix{}, false
	}
	varName := ps2134FreshName(pass, fn, used)
	insertPos := fn.Pos()
	if fn.Doc != nil {
		insertPos = fn.Doc.Pos()
	}
	binding := "var " + varName + " = " + exprTextRendered(mustCall) + "\n\n"
	return analysis.SuggestedFix{
		Message: "hoist the template parse to a package-level var",
		TextEdits: []analysis.TextEdit{
			{Pos: insertPos, End: insertPos, NewText: []byte(binding)},
			{Pos: mustCall.Pos(), End: mustCall.End(), NewText: []byte(varName)},
		},
	}, true
}

// ps2134IsTemplateMust reports whether fun is template.Must of the same
// template package (by import path) as the New call being hoisted.
func ps2134IsTemplateMust(info *types.Info, fun ast.Expr, pkgPath string) bool {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Must" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	pn, ok := info.Uses[id].(*types.PkgName)
	if !ok {
		return false
	}
	return pn.Imported() != nil && pn.Imported().Path() == pkgPath
}

// ps2134MustUsedReadOnly reports whether the template.Must(...) value is used in
// a provably read-only way: either inline as the receiver of a read-only method
// call, or bound (via := or var) to a single local that is only ever the
// receiver of read-only methods within the enclosing function.
func ps2134MustUsedReadOnly(info *types.Info, stack []ast.Node, mustCall *ast.CallExpr) bool {
	// stack (from the parse node's walk) excludes the node itself: stack[len-1]
	// is the Must call, so stack[len-2] is the Must call's parent.
	if len(stack) < 2 {
		return false
	}
	parent := stack[len(stack)-2]
	switch p := parent.(type) {
	case *ast.SelectorExpr:
		// Inline: template.Must(...).Execute(...) — p.X is the Must call, and its
		// parent is the enclosing method call with a read-only selector.
		if p.X != ast.Expr(mustCall) || len(stack) < 3 {
			return false
		}
		call, ok := stack[len(stack)-3].(*ast.CallExpr)
		return ok && call.Fun == ast.Expr(p) && ps2134ReadOnlyTemplateMethod(p.Sel.Name)
	case *ast.AssignStmt:
		// t := template.Must(...) — single define, single value.
		if p.Tok != token.DEFINE || len(p.Lhs) != 1 || len(p.Rhs) != 1 || p.Rhs[0] != ast.Expr(mustCall) {
			return false
		}
		return ps2134BoundLocalReadOnly(info, stack, p.Lhs[0])
	case *ast.ValueSpec:
		// var t = template.Must(...) — single name, single value.
		if len(p.Names) != 1 || len(p.Values) != 1 || p.Values[0] != ast.Expr(mustCall) {
			return false
		}
		return ps2134BoundLocalReadOnly(info, stack, p.Names[0])
	}
	return false
}

// ps2134BoundLocalReadOnly reports whether the local bound by lhs is used only as
// the receiver of read-only *Template methods within its enclosing top-level
// function — never reassigned, address-taken, returned, passed on, or the
// receiver of a mutating method. Any such escape/mutation means sharing one
// package instance could change behavior, so the fix is withheld.
func ps2134BoundLocalReadOnly(info *types.Info, stack []ast.Node, lhs ast.Expr) bool {
	id, ok := lhs.(*ast.Ident)
	if !ok {
		return false
	}
	obj := info.Defs[id]
	if obj == nil {
		return false
	}
	fn, ok := ps2127EnclosingTopFunc(stack)
	if !ok {
		return false
	}
	safe := true
	astutil.WithStack(fn, func(n ast.Node, s []ast.Node) bool {
		use, isID := n.(*ast.Ident)
		if !isID || info.Uses[use] != obj {
			return true
		}
		// A use (not the definition) must be the receiver X of a read-only method
		// call: use -> SelectorExpr(.M) -> CallExpr(Fun == that selector). s
		// excludes the use node, so s[len-1] is the selector, s[len-2] the call.
		if len(s) >= 2 {
			if selE, okS := s[len(s)-1].(*ast.SelectorExpr); okS && selE.X == ast.Expr(use) {
				if call, okC := s[len(s)-2].(*ast.CallExpr); okC && call.Fun == ast.Expr(selE) && ps2134ReadOnlyTemplateMethod(selE.Sel.Name) {
					return true
				}
			}
		}
		safe = false
		return false
	})
	return safe
}

// ps2134FreshName derives a deterministic, collision-free package-level name,
// seeded from the enclosing function's source line, mirroring PS2127/PS2132.
func ps2134FreshName(pass *analysis.Pass, fn *ast.FuncDecl, used map[string]bool) string {
	line := pass.Fset.Position(fn.Pos()).Line
	// Concatenate rather than fmt.Sprintf in the loop (perfscan's own PS2103).
	base := "psTemplateL" + strconv.Itoa(line)
	name := base
	for i := 2; ps2127NameTaken(pass, used, name); i++ {
		name = base + "_" + strconv.Itoa(i)
	}
	used[name] = true
	return name
}
