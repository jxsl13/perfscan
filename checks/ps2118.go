package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2118 reports io.WriteString(w, string(b)) with b a []byte — the
// conversion allocates a string copy just to hand the same bytes to w.
var PS2118 = register(&lint.Check{
	ID:       "PS2118",
	Category: "alloc",
	Slug:     "writestring-of-byte-string",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "io.WriteString(w, string(b)) allocates; w.Write(b) writes the bytes directly",
		Text: `Converting a []byte to a string just to pass it to io.WriteString
allocates and copies the whole slice on every call. io.WriteString exists
for values that ARE strings; when the value starts as a []byte the direct
w.Write(b) writes exactly the same bytes with no copy. Both calls return
(int, error) with the same meaning (bytes written + error), and w is
statically an io.Writer — io.WriteString's own first parameter — so the
rewrite always compiles and preserves every result.

Semantics are pinned by the io contracts the original call already relies
on: when w implements io.StringWriter, io.WriteString calls
w.WriteString(string(b)), whose contract requires behavior identical to
Write with the same bytes; otherwise io.WriteString itself calls
w.Write([]byte(s)) — the very copy the rewrite skips. io.Writer requires
Write not to modify or retain its argument, so handing w the original b
instead of a private copy changes nothing for a conforming writer.

The callee is resolved with type information — only the package function
io.WriteString matches, never a shadowed io or a same-named method — and
the second argument must be a genuine type conversion string(b) (the
predeclared string type, not a function named string) whose operand's
underlying type is exactly []byte. A named type with underlying []byte
converts and remains assignable to Write's []byte parameter, so it
qualifies; a string, []rune, single rune, or a slice of a NAMED byte type
(convertible to string but not assignable to []byte) does not.

One position is excluded entirely: io.WriteString(w, string(b)) lexically
inside w's own Write method — a Write that delegates through
io.WriteString to its type's WriteString. There the rewrite w.Write(b)
would make Write call itself (unbounded recursion that still compiles),
and the original delegation is already the correct code, so the check
reports nothing. Writing to a different object (a field, another
variable) inside Write is still reported.

The automatic fix replaces the whole call with w.Write(b), rendering both
argument expressions as written (a non-selectable w such as &buf is
parenthesized). Each rewrite removes the file's io.WriteString selector;
the fix applies even when that removes the file's last io reference —
the fix pipeline prunes the now-unused io import — EXCEPT in cgo files
(import "C"), whose import block is never edited: there the fixes are
withheld and the report stays advisory. A comment inside the call is
never silently dropped: it suppresses the fix and keeps the report.`,
		Before: `io.WriteString(w, string(b))`,
		After:  `w.Write(b)`,
		MeasuredWin: `BenchmarkPS2118 (64 slices of ~83 bytes written to a
bytes.Buffer held as an io.Writer, Apple M2 Pro): 1471 ns/op ->
238 ns/op (~6.2x) and 6144 B/op, 64 allocs/op -> 0 B/op, 0 allocs/op —
every string(b) heap-allocated and copied the bytes that Write sends
directly.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2118",
		Doc:  "io.WriteString(w, string(b)) instead of w.Write(b)",
		Run:  runPS2118,
	},
})

func runPS2118(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying ALL fixes may rewrite the file's last io
		// reference and orphan the import. The fix pipeline prunes such an
		// orphan afterwards — except in a cgo file (import "C"), whose
		// import block is never edited, so there the fixes are withheld
		// and the reports stay advisory (same guard as PS2103/PS2107).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package function
			// io.WriteString: a shadowed io resolves sel.Sel to some other
			// object, and a method (including io.StringWriter's own
			// WriteString) carries a receiver.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "WriteString" || fn.Pkg() == nil || fn.Pkg().Path() != "io" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			_, inner := ps2118ByteSliceConv(pass, call.Args[1])
			if inner == nil {
				return true
			}
			// Inside the writer's own Write method the rewrite w.Write(b)
			// would call the enclosing method itself — the classic
			// Write-delegates-to-WriteString implementation. The original
			// code is the correct delegation: stay silent.
			if writeFixSelfDispatches(pass, call, call.Args[0], "Write") {
				return true
			}
			fix := ps2118Fix(f, call, call.Args[0], inner)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fix})
			return true
		})
		// Each fixable call holds exactly one io reference (its selector's
		// package identifier); if those are ALL of the file's io references,
		// the fixes orphan the import — fine in a non-cgo file (the fix
		// pipeline prunes it), advisory only in a cgo file.
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "io") > fixable || !ps2110ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "io.WriteString(w, string(b)) allocates a string copy of the byte slice just to write it; w.Write(b) writes the bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2118ByteSliceConv matches arg (parens stripped) as the conversion
// string(b) where the conversion type is the predeclared string (aliases
// included, named string types excluded — they would not have been
// assignable to io.WriteString's string parameter anyway) and b's type has
// underlying EXACTLY []byte. The identity requirement on the element
// rejects a slice of a named byte type: string([]namedByte) converts, but
// []namedByte is not assignable to Write's []byte parameter, so the
// rewrite would not compile.
func ps2118ByteSliceConv(pass *analysis.Pass, arg ast.Expr) (conv *ast.CallExpr, inner ast.Expr) {
	for {
		p, ok := arg.(*ast.ParenExpr)
		if !ok {
			break
		}
		arg = p.X
	}
	c, ok := arg.(*ast.CallExpr)
	if !ok || len(c.Args) != 1 || c.Ellipsis.IsValid() {
		return nil, nil
	}
	// A TYPE conversion only: a function or variable named string resolves
	// to a value, not a type, and may have side effects.
	tv, ok := pass.TypesInfo.Types[c.Fun]
	if !ok || !tv.IsType() {
		return nil, nil
	}
	tb, ok := types.Unalias(tv.Type).(*types.Basic)
	if !ok || tb.Kind() != types.String {
		return nil, nil
	}
	it := pass.TypesInfo.TypeOf(c.Args[0])
	if it == nil || !types.Identical(it.Underlying(), types.NewSlice(types.Typ[types.Byte])) {
		return nil, nil
	}
	return c, c.Args[0]
}

// ps2118Fix builds the w.Write(b) replacement for the whole call. The
// receiver and argument are re-rendered from their AST (same expressions,
// same left-to-right evaluation order as the original call), so a comment
// anywhere inside the call's source range would be silently destroyed —
// the fix is withheld then and the report stays advisory.
func ps2118Fix(f *ast.File, call *ast.CallExpr, w, b ast.Expr) *analysis.SuggestedFix {
	if ps2111CommentIn(f, call.Pos(), call.End()) {
		return nil
	}
	wText := exprTextRendered(w)
	if ps2118NeedsParens(w) {
		wText = "(" + wText + ")"
	}
	newText := wText + ".Write(" + exprTextRendered(b) + ")"
	return &analysis.SuggestedFix{
		Message: "replace io.WriteString(w, string(b)) with w.Write(b)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: call.End(), NewText: []byte(newText)},
		},
	}
}

// ps2118NeedsParens reports whether e must be parenthesized to serve as
// the operand of a selector: &buf.Write would bind as &(buf.Write), and a
// composite literal at the start of an if/for header would mis-parse.
// Primary expressions — identifiers, selectors, indexes, calls, type
// assertions, slices, and already-parenthesized expressions — need none.
func ps2118NeedsParens(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr, *ast.IndexListExpr,
		*ast.CallExpr, *ast.ParenExpr, *ast.TypeAssertExpr, *ast.SliceExpr:
		return false
	}
	return true
}
