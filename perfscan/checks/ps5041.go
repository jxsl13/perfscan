package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5041 reports fmt.Appendf(buf, "%q", s) — a lone "%q" verb quoting one
// string into a byte buffer — where strconv.AppendQuote(buf, s) appends the
// identical double-quoted Go string literal without fmt's format parse or
// interface boxing. The "%q" sibling of PS2035 (the "%v" scalar arm) and
// PS2141 (the lone "%s"), all carried to the fmt.Appendf destination.
var PS5041 = register(&lint.Check{
	ID:       "PS5041",
	Category: "alloc",
	Slug:     "appendf-q-string",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Appendf(buf, "%q", s) runs fmt's formatter to quote one string; strconv.AppendQuote(buf, s) writes the identical bytes directly`,
		Text: `fmt.Appendf(buf, "%q", s) parses the format string, boxes s into an
interface and drives fmt's formatter through a pooled buffer — all to append
one double-quoted, escaped Go string literal. strconv.AppendQuote(buf, s)
produces exactly the same literal directly into buf's backing array, with no
format parse and no boxing.

The rewrite is BIT-IDENTICAL: fmt's "%q" verb over a string is defined as
strconv.AppendQuote — the double-quoted form with Go escape sequences, the
same handling of control characters, non-printable runes and invalid UTF-8
(shown as \xNN). Verified equal for every single byte and every rune.

The match is deliberately narrow — it is the whole safety story:
  - the callee is pinned by type information to the package-level fmt.Appendf
    (a shadowed fmt or a method named Appendf never matches) and must not
    spread its arguments;
  - the format is a string literal that is EXACTLY "%q" — the "%+q" ASCII form
    and "%#q" backquote form, any width, flag, or other verb disqualify it;
  - the operand's default type is EXACTLY the predeclared string. A named
    string type is excluded: "%q" consults fmt.Formatter (and, absent that,
    fmt.Stringer) for the operand, so a named type with a Format or String
    method quotes that method's result, not the raw value — while
    strconv.AppendQuote always quotes the raw bytes. An unnamed predeclared
    string carries no methods, so the two are provably identical;
  - the destination is an unnamed []byte, so AppendQuote's []byte result
    matches Appendf's exactly (a named byte-slice destination would change the
    expression's static type — advisory), and the fix is withheld unless the
    file keeps another fmt reference (so dropping this call never orphans the
    fmt import) and strconv is importable (no cgo file, no shadow).
A comment inside the rewritten scaffolding keeps the report advisory. Named
destinations, named or non-string operands, and non-"%q" formats are reported
without a fix.`,
		Before: `buf = fmt.Appendf(buf, "%q", s)`,
		After:  `buf = strconv.AppendQuote(buf, s)`,
		MeasuredWin: `fmt.Appendf(buf, "%q", s) ~78 ns/op vs strconv.AppendQuote(buf, s) ` +
			`~56 ns/op on a short key (~1.4x, Apple M2 Pro, go1.26). The saving is a ` +
			`FIXED ~22 ns — fmt's format parse and interface dispatch, which AppendQuote ` +
			`skips — so it is a larger ratio for short strings (~3x on a 2-char string) ` +
			`and narrows as the escaping cost grows. Both are 0 allocs when buf is reused; ` +
			`the fmt path additionally boxes s (a heap allocation when the call escapes).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5041",
		Doc:  "fmt.Appendf(buf, \"%q\", s) of a string instead of strconv.AppendQuote(buf, s)",
		Run:  runPS5041,
	},
})

func runPS5041(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		// All fixes of a run are applied together, so only the first fix
		// needing the strconv import carries the import edit.
		importAdded := false
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
			}
			// Type info pins the callee to the package-level fmt.Appendf; a
			// shadowed fmt or a method named Appendf never matches.
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Appendf": true}); !ok {
				return true
			}
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if v, err := strconv.Unquote(lit.Value); err != nil || v != "%q" {
				return true
			}
			// The operand's default type must be EXACTLY the predeclared
			// string (types.Default yields a *types.Basic only for the
			// unnamed predeclared type; a named string survives as
			// *types.Named and is skipped — its Format/String method would
			// hijack "%q").
			b, ok := types.Default(pass.TypesInfo.TypeOf(call.Args[2])).(*types.Basic)
			if !ok || b.Kind() != types.String {
				return true
			}
			var fix *analysis.SuggestedFix
			// Destination guard: an unnamed []byte, so the rewrite's []byte
			// result matches Appendf's exactly. (An untyped nil destination
			// fails this too and stays advisory.)
			if ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(call.Args[0])) &&
				!ps2111CommentIn(f, call.Args[0].End(), call.Args[2].Pos()) &&
				!ps2111CommentIn(f, call.Args[2].End(), call.End()) {
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "strconv", "strconv")
				if usable && !(needImport && ps2107ImportsC(f)) {
					// PkgFuncCall matched, so call.Fun is a SelectorExpr.
					sel := call.Fun.(*ast.SelectorExpr)
					edits := []analysis.TextEdit{
						{Pos: sel.Pos(), End: sel.End(), NewText: []byte(useName + ".AppendQuote")},
						{Pos: call.Args[0].End(), End: call.Args[2].Pos(), NewText: []byte(", ")},
						{Pos: call.Args[2].End(), End: call.End(), NewText: []byte(")")},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "strconv"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "replace fmt.Appendf(buf, \"%q\", s) with " + useName + ".AppendQuote",
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, site{call, fix})
			return true
		})
		// Withhold every fix when applying them would strip the file's last
		// fmt reference and orphan the import (the fix adds no import-drop).
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "fmt") > fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "fmt.Appendf(buf, \"%q\", s) parses the format and boxes s to quote one string; strconv.AppendQuote(buf, s) appends the identical Go string literal directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}
