package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2138 reports len(bytes.Runes(b)): bytes.Runes decodes EVERY rune of b
// into a freshly allocated []rune, and taking only its len discards that
// slice immediately — utf8.RuneCount(b) computes the identical integer in
// one allocation-free pass. The []byte sibling of PS2125's len([]rune(s))
// arm, with one crucial difference: the compiler special-cases the
// conversion composition, but bytes.Runes is an ordinary library call it
// does NOT peephole, so here the allocation is real on every toolchain.
var PS2138 = register(&lint.Check{
	ID:       "PS2138",
	Category: "alloc",
	Slug:     "len-bytes-runes",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "len(bytes.Runes(b)) allocates a full []rune just to count it; utf8.RuneCount(b) counts allocation-free",
		Text: `bytes.Runes(s) is implemented as make([]rune, utf8.RuneCount(s))
followed by a decode-and-fill loop, so len(bytes.Runes(b)) already
computes utf8.RuneCount(b) internally — and then additionally allocates
and populates a []rune slice whose only use is its len. utf8.RuneCount(b)
returns the same integer in a single allocation-free pass.

The two are bit-identical for EVERY input as an algebraic identity:
bytes.Runes sizes its result exactly as make([]rune, utf8.RuneCount(s)),
so len(bytes.Runes(b)) == utf8.RuneCount(b) by construction. Both walk
the bytes with the same utf8.DecodeRune semantics, so invalid or
truncated UTF-8 is counted identically (each erroneous byte is one rune
of width 1); nil and empty both yield 0. The result is a plain int, and
utf8.RuneCount's parameter is []byte — exactly bytes.Runes's parameter
type — so the argument expression is passed through verbatim, evaluated
exactly once, with side effects preserved in count and order.

Unlike PS2125's len([]rune(s)) arm, there is no compiler peephole here:
cmd/compile lowers the len([]rune(s)) CONVERSION composition to an
allocation-free runtime.countrunes call, but bytes.Runes is an ordinary
library call it does not special-case, so the wasted O(rune count)
allocation is real and measured on current gc.

The match is strict: the outer callee must be the predeclared builtin
len (a shadowed len is rejected via type information), and the argument
must be DIRECTLY a call type-pinned to the standard library's
bytes.Runes (a shadowed bytes qualifier, a same-named method or a
third-party package imported as bytes never matches; a result stored in
a variable first may have other consumers and is out of scope).

The fix keeps the argument verbatim in place — same text, same single
evaluation — and edits only the scaffolding: len(bytes.Runes(x)) becomes
utf8.RuneCount(x). A call is a primary expression, so no outer
parentheses are ever needed. The unicode/utf8 import is added when
missing (reusing an existing import's alias); the vanishing bytes.Runes
spelling may orphan the file's bytes import, which the fix pipeline
prunes afterwards — except in a cgo file, whose import block is never
edited, so there the fixes are withheld and the reports stay advisory
when the fixed call was the file's last bytes reference. A comment
inside the rewritten scaffolding likewise downgrades the fix to an
advisory report.`,
		Before: `n := len(bytes.Runes(b))`,
		After:  `n := utf8.RuneCount(b)`,
		MeasuredWin: `BenchmarkPS2138 (27-byte mixed-width []byte of 17 runes,
Apple M2 Pro, gc): len(bytes.Runes(b)) 69.4 ns/op, 80 B/op, 1 alloc/op vs
utf8.RuneCount(b) 24.5 ns/op, 0 B/op, 0 allocs/op — ~2.8x faster with the
allocation eliminated; the gap widens with input length since the wasted
[]rune is O(rune count).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2138",
		Doc:  "len(bytes.Runes(b)) allocates a throwaway []rune; utf8.RuneCount(b) counts allocation-free",
		Run:  runPS2138,
	},
})

func runPS2138(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last
		// bytes reference and orphan the import. The fix pipeline prunes
		// such an orphan afterwards — except in a cgo file (import "C"),
		// whose import block is never edited, so there the fixes are
		// withheld and the reports stay advisory (same per-file collection
		// as PS2137).
		type site struct {
			call *ast.CallExpr
			msg  string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		// All fixes of a run are applied together, so only the first fix
		// needing the unicode/utf8 import carries the import edit (same
		// convention as PS2125).
		importAdded := false
		ast.Inspect(f, func(n ast.Node) bool {
			lenCall, ok := n.(*ast.CallExpr)
			if !ok || len(lenCall.Args) != 1 || lenCall.Ellipsis.IsValid() {
				return true
			}
			// The outer callee must be the predeclared builtin len — a
			// shadowed len resolves to some other object and is rejected.
			id, ok := lenCall.Fun.(*ast.Ident)
			if !ok || id.Name != "len" {
				return true
			}
			if pass.TypesInfo.Uses[id] != types.Universe.Lookup("len") {
				return true
			}
			// The argument must be DIRECTLY the bytes.Runes call: a result
			// stored in a variable first may have other consumers. The
			// callee is resolved by type information (astutil.PkgFuncCall),
			// so an aliased bytes import matches and a shadowed bytes or a
			// same-named third-party package does not.
			inner, ok := ps2109Unparen(lenCall.Args[0]).(*ast.CallExpr)
			if !ok || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, inner.Fun, "bytes", map[string]bool{"Runes": true}); !ok {
				return true
			}
			x := inner.Args[0]
			msg := "len(" + ps2125ExprText(inner) + ") allocates a throwaway []rune of every decoded rune just to count them; utf8.RuneCount(" + ps2125ExprText(x) + ") is the direct, bit-identical count (allocation-free)"
			fix := ps2138Fix(pass, f, lenCall, x, &importAdded)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{lenCall, msg, fix})
			return true
		})
		// Each applied fix removes exactly one bytes qualifier from the
		// file (the one in the rewritten bytes.Runes); references inside
		// the preserved argument stay. When the fixes would orphan the
		// import in a cgo file, they are withheld (advisory reports only).
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "bytes") > fixable || !ps2110ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: st.msg,
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2138Fix builds the rewrite to <utf8>.RuneCount(x), or returns nil
// when any guard fails and the report must stay advisory. x's text is
// untouched in place — same expression, same single evaluation — only
// the surrounding scaffolding changes; the replacement is a call (a
// primary expression), so no outer parentheses are ever needed.
func ps2138Fix(pass *analysis.Pass, f *ast.File, lenCall *ast.CallExpr, x ast.Expr, importAdded *bool) *analysis.SuggestedFix {
	// A comment inside the rewritten scaffolding would be destroyed —
	// the fix is withheld then and the report stays advisory.
	if ps2111CommentIn(f, lenCall.Pos(), x.Pos()) || ps2111CommentIn(f, x.End(), lenCall.End()) {
		return nil
	}
	// Reuse the file's existing unicode/utf8 import name (possibly an
	// alias); dot and blank imports leave no usable qualifier.
	name, ok := ps2125Utf8Name(f)
	if !ok {
		return nil
	}
	// The name must actually reference unicode/utf8 at the call site —
	// a shadowing local would capture the qualifier.
	needImport, usable := ps2110PkgUsable(pass, lenCall.Pos(), name, "unicode/utf8")
	if !usable {
		return nil
	}
	if needImport && ps2110ImportsC(f) {
		return nil
	}
	edits := []analysis.TextEdit{
		{Pos: lenCall.Pos(), End: x.Pos(), NewText: []byte(name + ".RuneCount(")},
		{Pos: x.End(), End: lenCall.End(), NewText: []byte(")")},
	}
	if needImport && !*importAdded {
		edits = append(edits, ps2110ImportEdit(f, "unicode/utf8"))
		*importAdded = true
	}
	return &analysis.SuggestedFix{
		Message:   "replace with " + name + ".RuneCount(...)",
		TextEdits: edits,
	}
}
