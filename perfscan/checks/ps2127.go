package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2127 reports a regexp.MustCompile / MustCompilePOSIX with a constant string
// pattern used inline in a function body: it recompiles the SAME matcher on
// every call. The fix hoists it to a package-level var so the pattern is
// compiled exactly once, at program init. This is the complement of PS2005,
// which owns the in-loop case (and only hoists to the loop head).
var PS2127 = register(&lint.Check{
	ID:       "PS2127",
	Category: "alloc",
	Slug:     "regexp-compile-per-call",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a regexp.MustCompile of a constant pattern used inline in a function body",
		Text: `regexp.MustCompile("…").MatchString(s) in a function body parses the
pattern and builds the matcher program on EVERY call. For a constant
(string-literal) pattern the result never changes, so the standard idiom is a
single package-level var compiled once at init:

	var reFoo = regexp.MustCompile("^a+$")

The automatic fix performs exactly that hoist: it inserts a fresh package-level
binding immediately before the enclosing top-level function and replaces the
in-body call with the new variable. The compiled matcher is byte-for-byte
identical to what the per-call compile produced.

The fix fires ONLY on the provably-safe inline-throwaway shape — the compile
result is immediately the receiver of a READ-ONLY *regexp.Regexp method
(Match…/Find…/Replace…/Split/String/…) and is never bound, returned, or passed
on. That guarantees sharing one package-level instance is sound: a result that
escapes could be mutated via Longest() or depend on pointer identity, so those
are left alone. Only MustCompile / MustCompilePOSIX are rewritten; Compile /
CompilePOSIX return an error whose handling cannot move to a var initializer.

Additional guards (no fix otherwise): the pattern literal must actually compile
(so the hoist never moves a guaranteed panic to program init); the qualifier
must resolve to the real standard-library regexp package by import PATH (an
aliased or shadowed "regexp" is not it); the call must sit in a top-level
function body — not a package-level var initializer (already once), not init(),
and not inside a loop (PS2005 handles that).

L2 (structured): the fix introduces a new package-level declaration. For a
valid literal pattern used read-only the runtime behavior is identical.`,
		Before: `func match(s string) bool {
	return regexp.MustCompile("^a+$").MatchString(s)
}`,
		After: `var psRegexpL2 = regexp.MustCompile("^a+$")

func match(s string) bool {
	return psRegexpL2.MatchString(s)
}`,
		MeasuredWin: "a constant pattern compiled once at init instead of on every call: the per-call parse + program build (typically microseconds and several allocations) drops to zero after startup",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2127",
		Doc:  "regexp.MustCompile of a constant pattern recompiled on every call",
		Run:  runPS2127,
	},
})

var regexpMustFuncs = map[string]bool{
	"MustCompile":      true,
	"MustCompilePOSIX": true,
}

func runPS2127(pass *analysis.Pass) (any, error) {
	// Names generated this run, so two fixes never collide on one package
	// scope (line-based names are unique per file but not across files).
	used := map[string]bool{}
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !regexpMustFuncs[sel.Sel.Name] {
				return true
			}
			fnName := sel.Sel.Name
			// The qualifier must be the real stdlib regexp package, resolved by
			// import PATH — an aliased/shadowed "regexp" identifier is not it.
			if !ps2127IsStdRegexp(pass.TypesInfo, sel.X) {
				return true
			}
			// A single string-literal pattern: the only shape whose compile is
			// provably invariant and safe to move to package init.
			if len(call.Args) != 1 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			// The pattern must actually compile: otherwise the original panics
			// only when reached, but a package-level hoist would panic at init.
			if !ps2127PatternCompiles(fnName, lit) {
				return true
			}
			// PS2005 owns the in-loop case (and hoists it to the loop head).
			if _, inLoop := astutil.InLoop(stack); inLoop {
				return true
			}
			// Must be inside a real top-level function body — not a
			// package-level var initializer (already once) and not init().
			fn, ok := ps2127EnclosingTopFunc(stack)
			if !ok {
				return true
			}
			// Only the inline-throwaway read-only shape is safe to share as a
			// single package-level instance (rules out Longest() mutation and
			// any result that escapes and could be mutated / identity-compared).
			if !ps2127InlineReadOnlyUse(stack, call) {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "regexp." + fnName + " of a constant pattern inside " + ps2127FuncLabel(fn) + " recompiles the same matcher on every call; hoist it to a package-level var compiled once at init",
				SuggestedFixes: []analysis.SuggestedFix{
					ps2127HoistFix(pass, fn, call, fnName, lit, used),
				},
			})
			return true
		})
	}
	return nil, nil
}

// ps2127IsStdRegexp reports whether x is the standard-library regexp package
// qualifier, resolved by import path rather than identifier text.
func ps2127IsStdRegexp(info *types.Info, x ast.Expr) bool {
	id, ok := x.(*ast.Ident)
	if !ok || info == nil {
		return false
	}
	pkgName, ok := info.Uses[id].(*types.PkgName)
	if !ok {
		return false
	}
	return pkgName.Imported() != nil && pkgName.Imported().Path() == "regexp"
}

// ps2127PatternCompiles reports whether the literal pattern actually compiles
// under the same engine the call uses.
func ps2127PatternCompiles(fnName string, lit *ast.BasicLit) bool {
	pat, err := strconv.Unquote(lit.Value)
	if err != nil {
		return false
	}
	if fnName == "MustCompilePOSIX" {
		_, err = regexp.CompilePOSIX(pat)
	} else {
		_, err = regexp.Compile(pat)
	}
	return err == nil
}

// ps2127InlineReadOnlyUse reports whether call is used immediately as the
// receiver of a read-only *regexp.Regexp method call — the only shape where a
// single shared package-level instance is behavior-identical to a fresh
// per-call one. The compile result must be the selector base of a called
// method, and that method must not mutate (Longest) or the result escape.
func ps2127InlineReadOnlyUse(stack []ast.Node, call *ast.CallExpr) bool {
	if len(stack) < 2 {
		return false
	}
	parentSel, ok := stack[len(stack)-1].(*ast.SelectorExpr)
	if !ok || parentSel.X != ast.Expr(call) {
		return false
	}
	gp, ok := stack[len(stack)-2].(*ast.CallExpr)
	if !ok || gp.Fun != ast.Expr(parentSel) {
		return false
	}
	return ps2127ReadOnlyRegexpMethod(parentSel.Sel.Name)
}

// ps2127ReadOnlyRegexpMethod reports whether a *regexp.Regexp method neither
// mutates the matcher (only Longest does) nor is the deprecated Copy. The
// Find/Match/Replace families plus the remaining accessors are all read-only
// and safe for concurrent shared use per the regexp docs.
func ps2127ReadOnlyRegexpMethod(name string) bool {
	if strings.HasPrefix(name, "Find") ||
		strings.HasPrefix(name, "Match") ||
		strings.HasPrefix(name, "Replace") {
		return true
	}
	switch name {
	case "Split", "String", "NumSubexp", "SubexpNames", "SubexpIndex",
		"LiteralPrefix", "MarshalText", "AppendText":
		return true
	}
	return false
}

// ps2127EnclosingTopFunc returns the top-level FuncDecl enclosing the current
// node, or false when the node is inside a package-level var/const initializer
// (stack[1] is a GenDecl) or the enclosing function is init(). With
// WithStack(f, …) the stack is [file, topDecl, …]; the top-level declaration
// is therefore stack[1].
func ps2127EnclosingTopFunc(stack []ast.Node) (*ast.FuncDecl, bool) {
	if len(stack) < 2 {
		return nil, false
	}
	fn, ok := stack[1].(*ast.FuncDecl)
	if !ok || fn.Body == nil {
		return nil, false
	}
	// init() runs once at startup already — hoisting buys nothing.
	if fn.Recv == nil && fn.Name != nil && fn.Name.Name == "init" {
		return nil, false
	}
	return fn, true
}

func ps2127FuncLabel(fn *ast.FuncDecl) string {
	if fn.Name != nil {
		return "function " + fn.Name.Name
	}
	return "a function"
}

// ps2127HoistFix builds the two-edit transform: insert a fresh package-level
// binding just before fn, and replace the call with the new variable name.
func ps2127HoistFix(pass *analysis.Pass, fn *ast.FuncDecl, call *ast.CallExpr, fnName string, lit *ast.BasicLit, used map[string]bool) analysis.SuggestedFix {
	varName := ps2127FreshName(pass, fn, used)
	// Insert before the function, ahead of its doc comment when present, so the
	// var does not wedge between the doc and its func.
	insertPos := fn.Pos()
	if fn.Doc != nil {
		insertPos = fn.Doc.Pos()
	}
	// Reuse the SOURCE qualifier (regexp, or its import alias), not a hardcoded
	// "regexp": on an aliased import (import rx "regexp") the name "regexp" is
	// not bound, so a hardcoded qualifier hoists to uncompilable code. The caller
	// validated call.Fun is a stdlib-regexp selector, so this assertion holds.
	qualifier := "regexp"
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		qualifier = exprTextRendered(sel.X)
	}
	binding := "var " + varName + " = " + qualifier + "." + fnName + "(" + lit.Value + ")\n\n"
	return analysis.SuggestedFix{
		Message: "hoist regexp." + fnName + " to a package-level var",
		TextEdits: []analysis.TextEdit{
			{Pos: insertPos, End: insertPos, NewText: []byte(binding)},
			{Pos: call.Pos(), End: call.End(), NewText: []byte(varName)},
		},
	}
}

// ps2127FreshName derives a deterministic, collision-free package-level name.
// It seeds from the source line and bumps a suffix until the name is free in
// both the real package scope and the set of names already minted this run.
func ps2127FreshName(pass *analysis.Pass, fn *ast.FuncDecl, used map[string]bool) string {
	line := pass.Fset.Position(fn.Pos()).Line
	base := "psRegexpL" + strconv.Itoa(line)
	name := base
	// Concatenate rather than fmt.Sprintf in the loop (perfscan's own PS2103).
	for i := 2; ps2127NameTaken(pass, used, name); i++ {
		name = base + "_" + strconv.Itoa(i)
	}
	used[name] = true
	return name
}

func ps2127NameTaken(pass *analysis.Pass, used map[string]bool, name string) bool {
	if used[name] {
		return true
	}
	if pass.Pkg != nil && pass.Pkg.Scope() != nil && pass.Pkg.Scope().Lookup(name) != nil {
		return true
	}
	return false
}
