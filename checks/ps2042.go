package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2042 reports fmt.Fprintf(w, "%s", b) with b statically an unnamed
// []byte — a format parse, an interface boxing and a pooled-buffer copy
// paid just to hand w the very bytes b already holds. The []byte arm of
// PS2129 (whose string arm rewrites to io.WriteString): here the writer's
// own Write method takes the operand directly, so the rewrite is simply
// w.Write(b) and needs no import at all.
var PS2042 = register(&lint.Check{
	ID:       "PS2042",
	Category: "alloc",
	Slug:     "fprintf-bytes-to-write",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Fprintf(w, \"%s\", b) on a []byte pays fmt's whole machinery; w.Write(b) hands the bytes to the writer directly",
		Text: `fmt.Fprintf(w, "%s", b) with b a []byte parses the one-verb format,
boxes b's slice header into an interface (one heap allocation), copies
b's bytes into fmt's pooled pp buffer and finally calls w.Write on that
buffer — all to hand w the very bytes b already holds. w.Write(b) hands
them over directly: no boxing, no format parse, no intermediate copy.

The result is BIT-IDENTICAL and return-value-identical. The %s verb on a
[]byte writes the bytes verbatim — invalid UTF-8 preserved, fmt
metacharacters inside b copied as data (never re-parsed), a nil or empty
slice writing zero bytes — and fmt.Fprintf's final step is exactly
n, err = w.Write(p.buf) with p.buf holding precisely b's bytes, so the
(n int, err error) pair is the one w.Write(b) returns, short writes and
sticky errors included. Both forms call w.Write exactly once and
evaluate w then b, left to right, exactly once each. w.Write(b) always
compiles: fmt.Fprintf's first parameter is io.Writer, so w's static type
carries Write([]byte) (int, error) in its method set. (One boundary: fmt
passes its pooled buffer, the rewrite passes b itself — an io.Writer
that retains or mutates the slice it is handed could tell them apart,
but both acts already violate io.Writer's documented contract.)

The match is deliberately strict; everything else is out of scope. The
callee is resolved with type information — only the standard library's
package function fmt.Fprintf matches, never a shadowed fmt, a same-named
third-party package or a method. The call takes exactly three arguments
with no variadic spread, and the format is a string LITERAL that is
exactly "%s" — one verb, nothing else. "%v" NEVER matches: %v of a
[]byte prints the decimal slice form "[104 105]", not the bytes (which
is also why the two-argument fmt.Fprint(w, b) form has no []byte arm at
all). "%s\n", "%q", "%x", "% s" and friends all format differently. The
operand must be STATICALLY the UNNAMED predeclared []byte (aliases
included): a defined type with underlying []byte could carry a
String()/Format() method that %s honors and Write would not — the same
unnamed-slice guard PS2107/PS2141 use. A writer that is the untyped nil
literal is skipped entirely (nil.Write would not compile, and the
original panics at runtime anyway).

A matched call lexically inside w's own Write method is excluded for
parity with PS2129 — the original fmt.Fprintf there already recurses
through w.Write, so no useful rewrite exists; writing to a different
object (a field, another variable) inside Write is still reported.

The writer and operand expressions are kept byte-verbatim in place —
evaluated once, in the original order; only the wrapper around them is
rewritten to w.Write(b), with the writer parenthesized when it is not a
primary expression (so (&buf).Write(b) binds correctly). The rewrite
needs no import; it only REMOVES one fmt reference per fixed call, so —
like PS2129 — the fix drops the file's fmt import when the rewrites
orphan it. A comment inside the rewritten punctuation, a cgo file whose
orphaned import block must not be edited, or a file importing fmt under
more than one name keeps the report advisory.`,
		Before: `fmt.Fprintf(w, "%s", b)`,
		After:  `w.Write(b)`,
		MeasuredWin: `BenchmarkPS2042 (writing a 45-byte []byte to io.Discard,
Apple M2 Pro, go1.26): fmt.Fprintf(w,"%s",b) 45.7 ns/op 24 B/op 1 alloc ->
w.Write(b) 2.0 ns/op 0 B/op 0 allocs (~22x; the alloc is the boxed
interface arg, the rest is fmt's format parse and pp-buffer round-trip).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2042",
		Doc:  "fmt.Fprintf(w, \"%s\", b) on an unnamed []byte instead of w.Write(b)",
		Run:  runPS2042,
	},
})

const ps2042Msg = `fmt.Fprintf(w, "%s", b) on a []byte pays fmt's format parse, interface boxing and pooled-buffer copy just to hand the bytes to a single w.Write; w.Write(b) writes them directly with the same (n, err)`

func runPS2042(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide the import edit once per file: every fixable
		// call removes one fmt reference, and whether the fmt import is
		// orphaned depends on ALL of them together (same per-file site
		// collection as PS2129/PS2118).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			w, b, ok := ps2042Match(pass, call)
			if !ok {
				return true
			}
			// Inside w's own Write method the original fmt.Fprintf already
			// dispatches back into the enclosing method — the code is
			// degenerate either way and no useful rewrite exists, so stay
			// silent (parity with PS2129's self-dispatch guard).
			if writeFixSelfDispatches(pass, call, w, "Write") {
				return true
			}
			fix := ps2042Fix(f, call, w, b)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// A file importing "fmt" under more than one name (e.g. `import (f
		// "fmt"; "fmt")` — legal but pathological) breaks the name-blind fmt
		// ref count: rewriting a call reached through one name can orphan the
		// OTHER spec, yielding "imported and not used". Advisory only (same
		// guard as PS2129).
		if fixable > 0 && ps2129MultipleFmtSpecs(f) {
			for i := range sites {
				sites[i].fix = nil
			}
			fixable = 0
		}
		// Each fixable call holds exactly one fmt reference (its selector's
		// package identifier); when those are ALL of the file's fmt
		// references, the rewrites orphan the import and the fix must drop it
		// (the runner never prunes imports itself). All fixes of a run are
		// applied together, so the drop edit rides on the first fixable site.
		if fixable > 0 && pkgRefCount(pass, f, "fmt") == fixable {
			edit, ok := ps2042FmtDropEdit(f)
			if !ok {
				// cgo file, or a fmt spec we could not locate: keep every
				// report advisory rather than emit a broken import block.
				for i := range sites {
					sites[i].fix = nil
				}
			} else {
				for i := range sites {
					if sites[i].fix != nil {
						sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, edit)
						break
					}
				}
			}
		}
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: ps2042Msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2042Match matches call as fmt.Fprintf(w, "%s", b) with the format a
// string literal that is exactly "%s" and b statically the unnamed
// predeclared []byte. The callee is pinned by type information to the
// standard library fmt's package function, so a shadowed fmt, a same-named
// third-party package or a method never matches.
func ps2042Match(pass *analysis.Pass, call *ast.CallExpr) (w, b ast.Expr, ok bool) {
	if call.Ellipsis.IsValid() || len(call.Args) != 3 {
		return nil, nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, nil, false
	}
	// The qualifier must BE the imported fmt package, not a value whose
	// evaluation the rewrite would drop.
	xid, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, nil, false
	}
	if pn, isPkg := pass.TypesInfo.Uses[xid].(*types.PkgName); !isPkg || pn.Imported().Path() != "fmt" {
		return nil, nil, false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" || fn.Name() != "Fprintf" {
		return nil, nil, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, nil, false
	}
	// The format must be a string LITERAL whose entire constant value is the
	// one verb "%s". NEVER "%v": %v of a []byte prints the decimal slice
	// form "[104 105]", not the bytes. Anything longer formats differently.
	lit, isLit := ps2110Unparen(call.Args[1]).(*ast.BasicLit)
	if !isLit || lit.Kind != token.STRING {
		return nil, nil, false
	}
	tv, has := pass.TypesInfo.Types[call.Args[1]]
	if !has || tv.Value == nil || tv.Value.Kind() != constant.String {
		return nil, nil, false
	}
	if constant.StringVal(tv.Value) != "%s" {
		return nil, nil, false
	}
	w, b = call.Args[0], call.Args[2]
	// b must be STATICALLY the unnamed predeclared []byte (aliases resolve
	// to it; an unnamed slice can carry no methods). A defined type with
	// underlying []byte could carry a String()/Format() method that %s
	// honors and Write would not — excluded entirely.
	bt := pass.TypesInfo.TypeOf(b)
	if bt == nil || !types.Identical(bt, types.NewSlice(types.Typ[types.Uint8])) {
		return nil, nil, false
	}
	// An untyped nil writer compiles as io.Writer but nil.Write(b) would
	// not; the original panics at runtime anyway — skip.
	wt := pass.TypesInfo.TypeOf(w)
	if wt == nil {
		return nil, nil, false
	}
	if basic, isBasic := wt.(*types.Basic); isBasic && basic.Kind() == types.UntypedNil {
		return nil, nil, false
	}
	return w, b, true
}

// ps2042Fix builds the w.Write(b) rewrite for one call, or nil when a guard
// fails and the report must stay advisory. Only the wrapper around the two
// operands is replaced; w's and b's text stays untouched in place,
// preserving their single evaluation and order (same technique as PS2129).
// The per-file fmt-import drop edit is appended later, once per file.
func ps2042Fix(f *ast.File, call *ast.CallExpr, w, b ast.Expr) *analysis.SuggestedFix {
	// The replaced spans are the call text around the operands (the format
	// literal included); a comment there would be silently destroyed —
	// advisory then.
	if ps2111CommentIn(f, call.Pos(), w.Pos()) ||
		ps2111CommentIn(f, w.End(), b.Pos()) ||
		ps2111CommentIn(f, b.End(), call.End()) {
		return nil
	}
	open, dot := "", ".Write("
	if ps2042WriterNeedsParens(w) {
		open, dot = "(", ").Write("
	}
	return &analysis.SuggestedFix{
		Message: "replace with w.Write(b)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: w.Pos(), NewText: []byte(open)},
			{Pos: w.End(), End: b.Pos(), NewText: []byte(dot)},
			{Pos: b.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

// ps2042WriterNeedsParens reports whether the writer expression must be
// parenthesized to serve as the operand of the emitted .Write selector.
// As a CALL ARGUMENT any expression was legal, but a selector binds
// tighter than unary operators: &buf.Write(b) would parse as
// &(buf.Write(b)). Primary expressions bind tightly already; everything
// else (unary &, deref, and composite literals — which additionally may
// not open an if/for/switch header unparenthesized) gets wrapped. Extra
// parens are always semantically neutral.
func ps2042WriterNeedsParens(w ast.Expr) bool {
	switch w.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.CallExpr, *ast.IndexExpr,
		*ast.IndexListExpr, *ast.TypeAssertExpr, *ast.SliceExpr, *ast.ParenExpr:
		return false
	}
	return true
}

// ps2042FmtDropEdit returns the edit removing the file's (single) fmt
// import spec, delegating to PS2129's spec locator. ok is false when the
// import block must not or cannot be edited (cgo file, missing spec).
func ps2042FmtDropEdit(f *ast.File) (analysis.TextEdit, bool) {
	if ps2110ImportsC(f) {
		return analysis.TextEdit{}, false
	}
	return ps2129FmtImportEdit(f, false)
}
