package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2129 reports fmt.Fprintf(w, "%s", s) / fmt.Fprintf(w, "%v", s) /
// fmt.Fprint(w, s) with s statically a plain string — a format parse,
// interface boxing and reflection dispatch paid just to copy bytes that
// io.WriteString(w, s) writes directly.
var PS2129 = register(&lint.Check{
	ID:       "PS2129",
	Category: "alloc",
	Slug:     "fprintf-string-to-writestring",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Fprintf(w, \"%s\", s) on a plain string pays fmt's whole machinery; io.WriteString(w, s) writes the bytes directly",
		Text: `fmt.Fprintf(w, "%s", s) with s a string parses the format,
boxes s into an interface (one heap allocation) and dispatches through
fmt's reflection printer — all to hand w the very bytes s already holds.
fmt.Fprintf(w, "%v", s) and fmt.Fprint(w, s) with a single string operand
are the same pattern. io.WriteString(w, s) writes the bytes directly:
w.WriteString(s) when w implements io.StringWriter, w.Write([]byte(s))
otherwise.

The result is BIT-IDENTICAL. The %s and %v verbs write a string operand
verbatim, and fmt.Fprint with one operand writes it verbatim too (its
separating space appears only BETWEEN operands) — so the bytes reaching w
are exactly s. Crucially the rewrite is safe against verb injection: any
% lives in the ARGUMENT s, which %s/%v copy as data, never re-parse — the
one-verb format itself is what vanishes. (Contrast a format-STRING
rewrite, where a smuggled verb would change output.) Both calls return
(n int, err error) — the count of bytes written and w's error — from the
same underlying Write, so results carry over verbatim. (One boundary:
io.WriteString prefers w.WriteString when w is an io.StringWriter, whereas
fmt always calls w.Write; the io.StringWriter contract REQUIRES WriteString
to write the same bytes as Write([]byte(s)), so identity holds for any
contract-conforming writer — a writer that violates it is already buggy.)

The match is deliberately strict; everything else is out of scope. The
callee is resolved with type information — only the standard library's
package functions fmt.Fprintf and fmt.Fprint match, never a shadowed fmt,
a same-named third-party package or a method. The Fprintf form takes
exactly three arguments whose format is a string LITERAL that is exactly
"%s" or exactly "%v" — one verb, nothing else: "%s\n", "%q", "%d", "%+v",
"% s" and friends all format differently and never match. The Fprint form
takes exactly two arguments (two operands would gain a separating space).
No variadic spread. The written operand must be STATICALLY the
predeclared type string: a defined string type would not compile as
io.WriteString's argument without a conversion, and a []byte,
fmt.Stringer or error operand is formatted through String()/Error() —
none match.

Two positions are excluded entirely: a matched call lexically inside w's
own WriteString or Write method. io.WriteString(w, s) dispatches to
w.WriteString when w implements io.StringWriter and to w.Write
otherwise, so inside either method the rewrite could dispatch back to
the enclosing method itself — unbounded recursion that still compiles.
The check reports nothing there; writing to a different object (a
field, another variable) inside those methods is still reported.

The writer and operand expressions are kept byte-verbatim in place —
evaluated once, in the original order; only the wrapper around them is
rewritten to io.WriteString(w, s). The fix edits imports as needed: the
io import is added when missing, and the fmt import is dropped when the
rewrites remove the file's LAST fmt reference — when they replace the
whole import, the fmt spec is swapped for "io" in place. A dot-, blank-
or alias-imported io, a local declaration shadowing the io name at the
call site, a comment inside the rewritten punctuation, or a cgo file
(whose import block must not be edited) keeps the report advisory.`,
		Before: `fmt.Fprintf(w, "%s", s)`,
		After:  `io.WriteString(w, s)`,
		MeasuredWin: `BenchmarkPS2129 (writing a 45-byte string to io.Discard,
Apple M2 Pro, go1.26): fmt.Fprintf(w,"%s",s) 32.6 ns/op 16 B/op 1 alloc ->
io.WriteString(w,s) 2.6 ns/op 0 B/op 0 allocs (~12.5x, the alloc is the
boxed interface arg fmt.Fprintf allocates).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2129",
		Doc:  "fmt.Fprintf(w, \"%s\", s) / fmt.Fprint(w, s) on a plain string instead of io.WriteString(w, s)",
		Run:  runPS2129,
	},
})

func runPS2129(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// call removes one fmt reference, and whether the fmt import is
		// orphaned depends on ALL of them together (same per-file site
		// collection as PS2118/PS3104).
		type site struct {
			call *ast.CallExpr
			msg  string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, verb, w, s, ok := ps2129Match(pass, call)
			if !ok {
				return true
			}
			// io.WriteString(w, s) dispatches to w.WriteString when w
			// implements io.StringWriter and to w.Write otherwise: inside
			// the writer's own WriteString or Write method the rewrite
			// would dispatch back to the enclosing method itself — stay
			// silent, no valid rewrite exists there.
			if writeFixSelfDispatches(pass, call, w, "WriteString", "Write") {
				return true
			}
			fix := ps2129Fix(pass, f, call, w, s)
			if fix != nil {
				fixable++
			}
			var msg string
			if name == "Fprintf" {
				msg = "fmt.Fprintf(w, \"" + verb + "\", s) on a plain string pays fmt's format parse, interface boxing and reflection just to copy the bytes; io.WriteString(w, s) writes them directly with the same (n, err)"
			} else {
				msg = "fmt.Fprint(w, s) on a single plain string pays fmt's interface boxing and reflection just to copy the bytes; io.WriteString(w, s) writes them directly with the same (n, err)"
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
		// it (the runner never prunes imports itself). The io import is
		// needed when the FILE lacks a plain one — a site-local shadow only
		// suppresses that site's fix, never the file's import edit.
		ioImported := false
		for _, imp := range f.Imports {
			if imp.Path.Value == `"io"` && imp.Name == nil {
				ioImported = true
				break
			}
		}
		needImport := fixable > 0 && !ioImported
		orphansFmt := fixable > 0 && pkgRefCount(pass, f, "fmt") == fixable
		importEdits, importsOK := ps2129ImportEdits(f, needImport, orphansFmt)
		// A file importing "fmt" under more than one name (e.g. `import (f
		// "fmt"; "fmt")` — legal but pathological) breaks the name-blind fmt ref
		// count and the first-match spec selection: rewriting a fmt call reached
		// through one name can orphan the OTHER spec, yielding "imported and not
		// used". Refuse to fix any site in such a file — advisory only.
		if fixable > 0 && ps2129MultipleFmtSpecs(f) {
			importsOK = false
		}
		if !importsOK {
			// cgo file needing import surgery, or a fmt spec we could not
			// locate: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edits (same convention as
			// PS3104/PS2110).
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

// ps2129Match matches call as fmt.Fprintf(w, "%s"|"%v", s) or
// fmt.Fprint(w, s) with s statically the predeclared string type. The
// callee is pinned by type information to the standard library fmt's
// package functions, so a shadowed fmt, a same-named third-party package
// or a method never matches. verb is "" for the Fprint form.
func ps2129Match(pass *analysis.Pass, call *ast.CallExpr) (name, verb string, w, s ast.Expr, ok bool) {
	if call.Ellipsis.IsValid() {
		return "", "", nil, nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", "", nil, nil, false
	}
	// The qualifier must BE the imported fmt package, not a value whose
	// evaluation the rewrite would drop.
	xid, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", nil, nil, false
	}
	if pn, isPkg := pass.TypesInfo.Uses[xid].(*types.PkgName); !isPkg || pn.Imported().Path() != "fmt" {
		return "", "", nil, nil, false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" {
		return "", "", nil, nil, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return "", "", nil, nil, false
	}
	switch fn.Name() {
	case "Fprintf":
		// Exactly (w, format, s), and the format must be a string LITERAL
		// whose entire constant value is the one verb "%s" or "%v" —
		// anything else ("%s\n", "%q", "% s", …) formats differently.
		if len(call.Args) != 3 {
			return "", "", nil, nil, false
		}
		lit, isLit := ps2110Unparen(call.Args[1]).(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			return "", "", nil, nil, false
		}
		tv, has := pass.TypesInfo.Types[call.Args[1]]
		if !has || tv.Value == nil || tv.Value.Kind() != constant.String {
			return "", "", nil, nil, false
		}
		verb = constant.StringVal(tv.Value)
		if verb != "%s" && verb != "%v" {
			return "", "", nil, nil, false
		}
		w, s = call.Args[0], call.Args[2]
	case "Fprint":
		// Exactly (w, s): a second operand would gain fmt.Fprint's
		// separating space, which io.WriteString cannot reproduce.
		if len(call.Args) != 2 {
			return "", "", nil, nil, false
		}
		w, s = call.Args[0], call.Args[1]
	default:
		return "", "", nil, nil, false
	}
	// s must be STATICALLY the predeclared string (aliases included):
	// io.WriteString takes a real string, so a defined string type would
	// not compile without a conversion, and %s/%v on a []byte, Stringer or
	// error goes through fmt's own formatting, not the raw bytes.
	st := pass.TypesInfo.TypeOf(s)
	if st == nil || !types.Identical(st, types.Typ[types.String]) {
		return "", "", nil, nil, false
	}
	return fn.Name(), verb, w, s, true
}

// ps2129Fix builds the io.WriteString(w, s) rewrite for one call, or nil
// when a guard fails and the report must stay advisory. Only the wrapper
// around the two operands is replaced; w's and s's text stays untouched in
// place, preserving their single evaluation and order (same technique as
// PS3104/PS2110). Import edits are appended later, once per file.
func ps2129Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, w, s ast.Expr) *analysis.SuggestedFix {
	if !ps2129IoUsable(pass, f, call.Pos()) {
		return nil
	}
	// The replaced spans are the call text around the operands (the format
	// literal included); a comment there would be silently destroyed —
	// advisory then.
	if ps2111CommentIn(f, call.Pos(), w.Pos()) ||
		ps2111CommentIn(f, w.End(), s.Pos()) ||
		ps2111CommentIn(f, s.End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with io.WriteString(w, s)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: w.Pos(), NewText: []byte("io.WriteString(")},
			{Pos: w.End(), End: s.Pos(), NewText: []byte(", ")},
			{Pos: s.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

// ps2129IoUsable reports whether the fix may spell io.WriteString at pos:
// the file's io import must be un-renamed (the emitted text is exactly
// "io.WriteString", so a dot, blank or alias import keeps the report
// advisory), or absent and addable, and no other object may own the io
// name at the call site.
func ps2129IoUsable(pass *analysis.Pass, f *ast.File, pos token.Pos) bool {
	for _, imp := range f.Imports {
		if imp.Path.Value != `"io"` {
			continue
		}
		if imp.Name != nil {
			// Renamed (alias, dot or blank): the bare io qualifier the fix
			// emits would not resolve to the package — advisory.
			return false
		}
		needImport, ok := ps2110PkgUsable(pass, pos, "io", "io")
		return ok && !needImport
	}
	needImport, ok := ps2110PkgUsable(pass, pos, "io", "io")
	return ok && needImport
}

// ps2129MultipleFmtSpecs reports whether f imports the fmt package under more
// than one import spec. The rewrite's import bookkeeping assumes a single fmt
// spec; more than one makes both the ref count and the spec choice unreliable,
// so PS2129 declines to fix (advisory).
func ps2129MultipleFmtSpecs(f *ast.File) bool {
	n := 0
	for _, imp := range f.Imports {
		if imp.Path != nil && imp.Path.Value == `"fmt"` {
			n++
		}
	}
	return n > 1
}

// ps2129ImportEdits returns the import-block edits a file's combined fixes
// need: adding "io" when missing (via the shared sorted-position insert
// PS2110/PS3104 use) and dropping the file's fmt import when the rewrites
// orphan it. When both apply, the fmt spec is replaced by "io" in place —
// one edit, no overlap. ok is false when the import block must not or
// cannot be edited (cgo file, missing fmt spec).
func ps2129ImportEdits(f *ast.File, needIo, dropFmt bool) (edits []analysis.TextEdit, ok bool) {
	if !needIo && !dropFmt {
		return nil, true
	}
	if ps2110ImportsC(f) {
		return nil, false
	}
	if !dropFmt {
		return []analysis.TextEdit{ps2110ImportEdit(f, "io")}, true
	}
	edit, found := ps2129FmtImportEdit(f, needIo)
	if !found {
		return nil, false
	}
	return []analysis.TextEdit{edit}, true
}

// ps2129FmtImportEdit returns the edit removing the file's fmt import
// spec — or, when replaceWithIo is set, swapping the whole spec (alias
// included) for "io" in place. Unlike PS3104's sort→slices swap a std
// path can order between "fmt" and "io" (hash, html, image, index/…), so
// the swapped group may need a gofmt pass to re-sort — it always compiles.
func ps2129FmtImportEdit(f *ast.File, replaceWithIo bool) (analysis.TextEdit, bool) {
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for i, spec := range gd.Specs {
			is, ok := spec.(*ast.ImportSpec)
			if !ok || is.Path == nil || is.Path.Value != `"fmt"` {
				continue
			}
			if replaceWithIo {
				return analysis.TextEdit{Pos: is.Pos(), End: is.End(), NewText: []byte(`"io"`)}, true
			}
			switch {
			case len(gd.Specs) == 1:
				// Sole spec: drop the whole declaration (parenthesized or
				// not); gofmt normalizes the leftover blank line.
				return analysis.TextEdit{Pos: gd.Pos(), End: gd.End()}, true
			case i+1 < len(gd.Specs):
				return analysis.TextEdit{Pos: is.Pos(), End: gd.Specs[i+1].Pos()}, true
			default:
				return analysis.TextEdit{Pos: gd.Specs[i-1].End(), End: is.End()}, true
			}
		}
	}
	return analysis.TextEdit{}, false
}
