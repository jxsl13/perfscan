package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2132 reports a strings.NewReplacer(...) built with constant pairs and
// immediately consumed (chained into .Replace / .WriteString), which rebuilds
// its lookup structure on every call and throws it away.
//
// When the call sits in a function body and every pair is a compile-time
// constant, the fix hoists the constructor to a package-level var compiled once
// at init and rewrites the call site to reuse it — exactly the shape PS2127
// applies for regexp.MustCompile. A strings.Replacer is immutable after
// construction and documented safe for concurrent use, and its only methods
// (Replace / WriteString) are read-only, so sharing one package instance is
// byte-for-byte equivalent to a fresh per-call one.
var PS2132 = register(&lint.Check{
	ID:       "PS2132",
	Category: "alloc",
	Slug:     "newreplacer-per-call",
	Level:    lint.LevelStructured,
	AutoFix:  true,
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

The automatic fix performs exactly that hoist when it is provably safe: it
inserts a fresh package-level binding immediately before the enclosing top-level
function and replaces the inline strings.NewReplacer(...) with the new variable,
leaving the .Replace/.WriteString call intact. A strings.Replacer is immutable
after construction and documented safe for concurrent use, and Replace /
WriteString are the only (read-only) methods, so a single shared package
instance is byte-for-byte equivalent to a fresh per-call one.

The fix fires ONLY on that safe shape (otherwise the finding stays advisory): the
qualifier must resolve to the standard-library strings package by import PATH (an
aliased or shadowed "strings" is not it); the call must sit in a top-level
function body — not a package-level var initializer (already once) and not
init(); and the pair count must be EVEN, so the hoist never moves a guaranteed
NewReplacer panic (odd argument count) to program init.

L2 (structured): the fix introduces a new package-level declaration. For a
constant pair set consumed read-only the runtime behavior is identical.`,
		Before: `func escape(in string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(in)
}`,
		After: `var psReplacerL1 = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escape(in string) string {
	return psReplacerL1.Replace(in)
}`,
		MeasuredWin: "a constant-pair replacer built once at init instead of on every call: the per-call structure build (~728 ns and ~7 KB for an HTML-escape-sized set) drops to zero after startup, roughly 12x",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2132",
		Doc:  "strings.NewReplacer with constant pairs built inline and discarded every call",
		Run:  runPS2132,
	},
})

func runPS2132(pass *analysis.Pass) (any, error) {
	// Names generated this run, so two fixes never collide on one package scope.
	used := map[string]bool{}
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
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
			innerSel, ok := inner.Fun.(*ast.SelectorExpr)
			if !ok || innerSel.Sel.Name != "NewReplacer" {
				return true
			}
			// The qualifier must be the real stdlib strings package, resolved by
			// import PATH — an aliased/shadowed "strings" identifier is not it.
			if !ps2132IsStdStrings(pass.TypesInfo, innerSel.X) {
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
			diag := analysis.Diagnostic{
				Pos:     inner.Pos(),
				End:     inner.End(),
				Message: "strings.NewReplacer with constant pairs rebuilds its lookup structure on every call and discards it (~12x slower, ~7KB/call for many pairs); hoist `var r = strings.NewReplacer(...)` to package scope and reuse r." + sel.Sel.Name,
			}
			// Attach the hoist fix only on the provably-safe shape; otherwise the
			// finding stays advisory (still reported, no automatic rewrite).
			if fix, ok := ps2132HoistFix(pass, stack, inner, sel, used); ok {
				diag.SuggestedFixes = []analysis.SuggestedFix{fix}
			} else {
				diag.Message += " — advisory: this call shape is not auto-hoisted (odd argument count, or not inside a top-level function body)"
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2132IsStdStrings reports whether x is the standard-library strings package
// qualifier, resolved by import path rather than identifier text.
func ps2132IsStdStrings(info *types.Info, x ast.Expr) bool {
	id, ok := x.(*ast.Ident)
	if !ok || info == nil {
		return false
	}
	pkgName, ok := info.Uses[id].(*types.PkgName)
	if !ok {
		return false
	}
	return pkgName.Imported() != nil && pkgName.Imported().Path() == "strings"
}

// ps2132ReadOnlyMethod reports whether name is a read-only *strings.Replacer
// method. Replace and WriteString are the type's only methods and neither
// mutates the replacer, so sharing one instance across calls is sound.
func ps2132ReadOnlyMethod(name string) bool {
	return name == "Replace" || name == "WriteString"
}

// ps2132HoistFix builds the two-edit transform (insert a package-level binding
// before the enclosing top-level function, replace the inline constructor with
// the new variable) when the call shape is provably safe to hoist.
func ps2132HoistFix(pass *analysis.Pass, stack []ast.Node, inner *ast.CallExpr, outerSel *ast.SelectorExpr, used map[string]bool) (analysis.SuggestedFix, bool) {
	// An odd argument count makes NewReplacer panic at runtime; hoisting to a
	// package var would move that panic to program init — leave it advisory.
	if len(inner.Args)%2 != 0 {
		return analysis.SuggestedFix{}, false
	}
	if !ps2132ReadOnlyMethod(outerSel.Sel.Name) {
		return analysis.SuggestedFix{}, false
	}
	// Must be inside a real top-level function body — not a package-level var
	// initializer (already once) and not init().
	fn, ok := ps2127EnclosingTopFunc(stack)
	if !ok {
		return analysis.SuggestedFix{}, false
	}
	parts := make([]string, len(inner.Args))
	for i, a := range inner.Args {
		parts[i] = exprTextRendered(a)
	}
	varName := ps2132FreshName(pass, fn, used)
	// Insert before the function, ahead of its doc comment when present, so the
	// var does not wedge between the doc and its func.
	insertPos := fn.Pos()
	if fn.Doc != nil {
		insertPos = fn.Doc.Pos()
	}
	binding := fmt.Sprintf("var %s = strings.NewReplacer(%s)\n\n", varName, strings.Join(parts, ", "))
	return analysis.SuggestedFix{
		Message: "hoist strings.NewReplacer to a package-level var",
		TextEdits: []analysis.TextEdit{
			{Pos: insertPos, End: insertPos, NewText: []byte(binding)},
			{Pos: inner.Pos(), End: inner.End(), NewText: []byte(varName)},
		},
	}, true
}

// ps2132FreshName derives a deterministic, collision-free package-level name,
// seeded from the enclosing function's source line, mirroring PS2127.
func ps2132FreshName(pass *analysis.Pass, fn *ast.FuncDecl, used map[string]bool) string {
	line := pass.Fset.Position(fn.Pos()).Line
	base := "psReplacerL" + strconv.Itoa(line)
	name := base
	for i := 2; ps2127NameTaken(pass, used, name); i++ {
		name = base + "_" + strconv.Itoa(i)
	}
	used[name] = true
	return name
}
