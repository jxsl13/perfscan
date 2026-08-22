package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2130 reports fmt.Sprintf("%s", s) / fmt.Sprintf("%v", s) /
// fmt.Sprint(s) with s statically a plain string — a format parse,
// interface boxing and a fresh heap copy paid just to return the very
// bytes s already holds. The expression IS s.
var PS2130 = register(&lint.Check{
	ID:       "PS2130",
	Category: "alloc",
	Slug:     "sprintf-string-identity",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Sprintf(\"%s\", s) on a plain string is the identity paid at fmt prices; the expression is just s",
		Text: `fmt.Sprintf("%s", s) with s a string parses the format, boxes s
into an interface (one heap allocation) and dispatches through fmt's
reflection printer, which then allocates and copies a fresh result string
— all to return the very bytes s already holds. fmt.Sprintf("%v", s) and
fmt.Sprint(s) with a single string operand are the same pattern. The
whole call is the identity on s: replace it with s and both allocations
vanish.

The result is BIT-IDENTICAL. The %s and %v verbs write a string operand
verbatim, and fmt.Sprint with one operand returns it verbatim too (its
separating space appears only BETWEEN operands). Go strings are
immutable, so the fresh copy Sprintf returns is indistinguishable from s
itself under Go semantics — no program can observe the difference (this
is exactly why the compiler is allowed to intern strings). Crucially the
rewrite is safe against verb injection: any % lives in the ARGUMENT s,
which %s/%v copy as data, never re-parse — the one-verb format itself is
what vanishes. (Contrast a format-STRING rewrite, where a smuggled verb
would change output.) The one boundary: unsafe.StringData(s) exposes the
backing pointer, and s and its Sprintf copy hold DIFFERENT backing arrays —
so a program comparing those raw pointers via the unsafe package would
observe a change. Under ordinary (safe) Go semantics, where strings compare
and behave by value, the two are indistinguishable.

The match is deliberately strict; everything else is out of scope. The
callee is resolved with type information — only the standard library's
package functions fmt.Sprintf and fmt.Sprint match, never a shadowed fmt,
a same-named third-party package or a method. The Sprintf form takes
exactly two arguments whose format is a string LITERAL that is exactly
"%s" or exactly "%v" — one verb, nothing else: "%s\n", "%q", "%d", "%+v",
"%5s", "%[1]s", "%%s" and friends all format differently and never match.
The Sprint form takes exactly one argument (two operands could gain a
separating space). No variadic spread. The operand must be STATICALLY
the predeclared type string: for a defined string type (type S string)
fmt.Sprintf("%s", myS) returns a plain string, so substituting myS would
change the expression's static type from string to S — not an identity —
and a []byte, fmt.Stringer or error operand is formatted through
fmt/String()/Error(), none of which is a verbatim copy. Untyped string
constants default to string and match.

The operand expression is kept byte-verbatim in place — evaluated once,
in the original position; only the fmt wrapper around it is deleted. When
the bare operand would bind differently in the call's context (e.g.
fmt.Sprintf("%s", a+b)[0] must become (a+b)[0], not a+b[0]) the
replacement is parenthesized, exactly as PS2121/PS2124 decide it; a
self-delimiting operand (identifier, selector, call, index, literal)
never needs parentheses. A call used as a bare statement (or under
go/defer, which require a call) cannot become a bare operand — advisory.
The fix never ADDS an import; it drops the fmt import spec when the
rewrites remove the file's LAST fmt reference. A dot-, blank- or
alias-imported fmt, a file importing fmt under more than one name, a cgo
file (whose import block must not be edited when the drop is needed), or
a comment inside the deleted wrapper keeps the report advisory.`,
		Before: `s := fmt.Sprintf("%s", key)`,
		After:  `s := key`,
		MeasuredWin: `BenchmarkPS2130 (a 44-byte string, Apple M2 Pro, go1.26):
fmt.Sprintf("%s", s) 44.1 ns/op 64 B/op 2 allocs -> s 0.55 ns/op 0 B/op
0 allocs (~80x; the fmt call, its format parse, interface boxing and the
result copy all vanish).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2130",
		Doc:  "fmt.Sprintf(\"%s\", s) / fmt.Sprint(s) on a plain string is the identity on s; use s directly",
		Run:  runPS2130,
	},
})

func runPS2130(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// call removes one fmt reference, and whether the fmt import is
		// orphaned depends on ALL of them together (same per-file site
		// collection as PS2129/PS3104).
		type site struct {
			call *ast.CallExpr
			msg  string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, verb, s, ok := ps2130Match(pass, call)
			if !ok {
				return true
			}
			var parent ast.Node
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			fix := ps2130Fix(f, call, s, parent)
			if fix != nil {
				fixable++
			}
			var msg string
			if name == "Sprintf" {
				msg = "fmt.Sprintf(\"" + verb + "\", s) on a plain string pays fmt's format parse, interface boxing and a fresh string copy just to return the bytes s already holds; s itself is bit-identical"
			} else {
				msg = "fmt.Sprint(s) on a single plain string pays fmt's interface boxing and a fresh string copy just to return the bytes s already holds; s itself is bit-identical"
			}
			sites = append(sites, site{call, msg, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call holds exactly one fmt reference (its selector's
		// package identifier); when those are ALL of the file's fmt
		// references, the rewrites orphan the import and the fix must drop
		// it (the runner never prunes imports itself). There is never an
		// import to ADD.
		orphansFmt := fixable > 0 && pkgRefCount(pass, f, "fmt") == fixable
		importEdits, importsOK := ps2130ImportEdits(f, orphansFmt)
		// A file importing "fmt" under more than one name (legal but
		// pathological) breaks the name-blind fmt ref count and the
		// first-match spec selection, and a renamed single spec is outside
		// the drop bookkeeping PS2130 reuses — advisory in both cases (same
		// guard family as ps2129MultipleFmtSpecs).
		if fixable > 0 && (ps2129MultipleFmtSpecs(f) || ps2130RenamedFmt(f)) {
			importsOK = false
		}
		if !importsOK {
			// cgo file needing the drop, a fmt spec we could not locate, or
			// a renamed/duplicated fmt import: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edits (same convention as
			// PS2129/PS3104/PS2110).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: st.msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2130Match matches call as fmt.Sprintf("%s"|"%v", s) or fmt.Sprint(s)
// with s statically the predeclared string type. The callee is pinned by
// type information to the standard library fmt's package functions, so a
// shadowed fmt, a same-named third-party package or a method never
// matches. verb is "" for the Sprint form.
func ps2130Match(pass *analysis.Pass, call *ast.CallExpr) (name, verb string, s ast.Expr, ok bool) {
	if call.Ellipsis.IsValid() {
		return "", "", nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", "", nil, false
	}
	// The qualifier must BE the imported fmt package, not a value whose
	// evaluation the rewrite would drop.
	xid, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", nil, false
	}
	if pn, isPkg := pass.TypesInfo.Uses[xid].(*types.PkgName); !isPkg || pn.Imported().Path() != "fmt" {
		return "", "", nil, false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" {
		return "", "", nil, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return "", "", nil, false
	}
	switch fn.Name() {
	case "Sprintf":
		// Exactly (format, s), and the format must be a string LITERAL
		// whose entire constant value is the one verb "%s" or "%v" —
		// anything else ("%s\n", "%q", "%5s", …) formats differently.
		if len(call.Args) != 2 {
			return "", "", nil, false
		}
		lit, isLit := ps2110Unparen(call.Args[0]).(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			return "", "", nil, false
		}
		tv, has := pass.TypesInfo.Types[call.Args[0]]
		if !has || tv.Value == nil || tv.Value.Kind() != constant.String {
			return "", "", nil, false
		}
		verb = constant.StringVal(tv.Value)
		if verb != "%s" && verb != "%v" {
			return "", "", nil, false
		}
		s = call.Args[1]
	case "Sprint":
		// Exactly (s): a second operand could gain fmt.Sprint's separating
		// space, and either way the result would no longer be one operand.
		if len(call.Args) != 1 {
			return "", "", nil, false
		}
		s = call.Args[0]
	default:
		return "", "", nil, false
	}
	// s must be STATICALLY the predeclared string (aliases included): for
	// a defined string type the call returns a plain string, so the bare
	// operand would change the expression's static type — not an identity
	// — and %s/%v on a []byte, Stringer or error goes through fmt's own
	// formatting, not a verbatim copy.
	st := pass.TypesInfo.TypeOf(s)
	if st == nil || !types.Identical(st, types.Typ[types.String]) {
		return "", "", nil, false
	}
	return fn.Name(), verb, s, true
}

// ps2130Fix builds the identity rewrite for one call — the fmt wrapper
// around the operand is deleted and s's text stays untouched in place,
// preserving its single evaluation and position (same technique as
// PS2129/PS2124) — or nil when a guard fails and the report must stay
// advisory. Import edits are appended later, once per file.
func ps2130Fix(f *ast.File, call *ast.CallExpr, s ast.Expr, parent ast.Node) *analysis.SuggestedFix {
	// A call is a valid statement; its bare operand is not ("s evaluated
	// but not used"), and go/defer require a call outright — advisory.
	switch parent.(type) {
	case *ast.ExprStmt, *ast.GoStmt, *ast.DeferStmt:
		return nil
	}
	// The replaced spans are the call text around the operand (the format
	// literal included); a comment there would be silently destroyed —
	// advisory then.
	if ps2111CommentIn(f, call.Pos(), s.Pos()) ||
		ps2111CommentIn(f, s.End(), call.End()) {
		return nil
	}
	open, closing := "", ""
	if ps2130NeedsParens(parent, call, s) {
		open, closing = "(", ")"
	}
	return &analysis.SuggestedFix{
		Message: "replace the fmt call with its string operand",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: s.Pos(), NewText: []byte(open)},
			{Pos: s.End(), End: call.End(), NewText: []byte(closing)},
		},
	}
}

// ps2130NeedsParens reports whether the bare operand must be
// parenthesized in the call's context. A self-delimiting operand — an
// identifier, selector, call, index, literal or already-parenthesized
// expression — binds the same everywhere and never needs them; a binary,
// receive or dereference operand defers to the context, exactly as
// PS2121/PS2124 decide it (fmt.Sprintf("%s", a+b)[0] must become
// (a+b)[0], not a+b[0]).
func ps2130NeedsParens(parent ast.Node, call *ast.CallExpr, s ast.Expr) bool {
	// An operand whose leftmost token is a TypeName-form composite literal
	// (T{...} / pkg.T{...}) is a SYNTAX ERROR unparenthesized in an
	// if/for/switch header — `if T{...}.s == x` is rejected by the parser. The
	// bare operand replaces the whole call, which may sit in such a header, so
	// parenthesize it. Always safe; the pattern (a composite-literal operand to
	// fmt.Sprintf) is vanishingly rare, so redundant parens elsewhere are moot.
	if ps2130StartsWithNamedCompositeLit(s) {
		return true
	}
	switch s.(type) {
	case *ast.BinaryExpr, *ast.UnaryExpr, *ast.StarExpr:
		return ps2121NeedsParens(parent, call)
	}
	return false
}

// ps2130StartsWithNamedCompositeLit reports whether e's leftmost token is a
// TypeName-form composite literal. map[...]{}, []T{} and [N]T{} are NOT
// TypeName-form and are accepted in control-clause headers, so they do not
// count. A composite literal shielded inside a call's parens (f(T{...})) is not
// leading and does not count either.
func ps2130StartsWithNamedCompositeLit(e ast.Expr) bool {
	for {
		switch x := e.(type) {
		case *ast.CompositeLit:
			switch x.Type.(type) {
			case *ast.Ident, *ast.SelectorExpr:
				return true
			}
			return false
		case *ast.SelectorExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		case *ast.SliceExpr:
			e = x.X
		case *ast.CallExpr:
			e = x.Fun
		case *ast.TypeAssertExpr:
			e = x.X
		default:
			return false
		}
	}
}

// ps2130RenamedFmt reports whether f imports the fmt package under a
// non-plain name (alias, dot or blank). PS2130's drop bookkeeping assumes
// the file's one plain "fmt" spec; a renamed spec keeps every report in
// the file advisory.
func ps2130RenamedFmt(f *ast.File) bool {
	for _, imp := range f.Imports {
		if imp.Path != nil && imp.Path.Value == `"fmt"` && imp.Name != nil {
			return true
		}
	}
	return false
}

// ps2130ImportEdits returns the import-block edits a file's combined
// fixes need: dropping the file's fmt import when the rewrites orphan it
// (via the shared ps2129FmtImportEdit). There is never an import to add.
// ok is false when the import block must not or cannot be edited (cgo
// file, missing fmt spec).
func ps2130ImportEdits(f *ast.File, dropFmt bool) (edits []analysis.TextEdit, ok bool) {
	if !dropFmt {
		return nil, true
	}
	if ps2110ImportsC(f) {
		return nil, false
	}
	edit, found := ps2129FmtImportEdit(f, false)
	if !found {
		return nil, false
	}
	return []analysis.TextEdit{edit}, true
}
