package checks

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3025 reports fmt.Appendf(buf, "constant") — a verbless string literal
// pushed through fmt's whole printf machinery just to append its own bytes —
// where append(buf, "constant"...) writes the identical bytes directly. The
// Appendf sibling of PS5027 (verbless Sprintf) and PS5028 (verbless Fprintf);
// PS5015 covers a single scalar verb and PS2141 a lone %s, so the
// verbless-constant Appendf shape is exactly the gap between them.
var PS3025 = register(&lint.Check{
	ID:       "PS3025",
	Category: "indirect",
	Slug:     "appendf-verbless-constant-to-append",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Appendf of a verbless constant format runs fmt's formatter to append bytes the builtin append writes directly",
		Text: `fmt.Appendf(buf, "constant") gets a printer from fmt's sync.Pool,
walks the format byte-by-byte through doPrintf looking for verbs, copies the
verb-free run into the pooled pp buffer, appends that buffer onto buf and puts
the printer back — the entire formatting pipeline spent reproducing bytes the
source code already spells out. With no verbs and no operands fmt.Appendf's
final act IS append(b, p.buf...) with p.buf equal to the format's bytes, so
the call is byte-for-byte append(buf, "constant"...): the builtin append has a
dedicated string->[]byte fast path with no pool traffic, no format scan and no
intermediate copy. It is one single append of exactly the literal's length in
both spellings, so the resulting bytes, length AND capacity (same growth step)
match — and appending zero-or-more bytes preserves a nil buf's nil-ness
identically.

The match is deliberately narrow — it is the whole safety story, mirroring
PS2141/PS5027. The callee must resolve through type information to the
package-level fmt.Appendf (an aliased fmt import is honored; a shadowed fmt or
a method named Appendf is not), the call must pass EXACTLY two arguments and
not spread them (no ...), and the format must be a string LITERAL whose
unquoted value contains NO '%' byte anywhere. A single '%' disqualifies it:
fmt transforms it (%%->%, a lone verb with no operand prints %!verb(MISSING),
a trailing lone '%' prints %!(NOVERB)), while append keeps the bytes verbatim —
so any '%' breaks bit-identity. There is no operand at all, so no named-type,
String()/Formatter, nil or evaluation-order concern exists; NUL bytes and
invalid UTF-8 ride through both spellings identically.

The fix additionally requires the destination to be an UNNAMED []byte, so
append(buf, ...) reproduces fmt.Appendf's []byte return type exactly; a named
[]byte destination keeps append's named result type, so it stays advisory
(same rule as PS2141). An EMPTY format literal is still bit-identical but the
rewrite append(buf, ""...) is noise, so it stays advisory too. A call whose
result is discarded (a bare statement, or under go/defer) cannot become the
builtin append, which must be used — advisory there. Each rewrite removes the
file's fmt.Appendf selector, so — like PS2141/PS2107/PS2122 — the fixes are
withheld (advisory report only) when applying all of them would rewrite the
file's last fmt reference and orphan the import. A comment inside the
rewritten scaffolding suppresses the fix and keeps the report.`,
		Before: `buf = fmt.Appendf(buf, "HTTP/1.1 200 OK\r\n")`,
		After:  `buf = append(buf, "HTTP/1.1 200 OK\r\n"...)`,
		MeasuredWin: `BenchmarkPS3025 (the 17-byte constant "HTTP/1.1 200 OK\r\n"
appended to a preallocated buffer, Apple M2 Pro, go1.26): fmt.Appendf
~20.9 ns/op, 0 B/op, 0 allocs/op vs append(buf, lit...) ~1.0 ns/op, 0 B/op,
0 allocs/op (~20x faster — the pool round-trip, the byte-by-byte format scan
and the pp-buffer copy all vanish; the builtin append is a single memmove
into buf's existing capacity).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3025",
		Doc:  "fmt.Appendf of a verbless constant format appends the literal's bytes through fmt's whole machinery; append(buf, lit...) is bit-identical and direct",
		Run:  runPS3025,
	},
})

// ps3025Msg is the diagnostic text (shared by the fixed and advisory paths).
const ps3025Msg = "fmt.Appendf on a verbless constant format walks fmt's formatter to append the literal's own bytes; append(buf, lit...) writes the identical bytes directly"

func runPS3025(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: fixes are suppressed when applying all of them would
		// rewrite the file's last fmt reference and orphan the import (the
		// runner never prunes imports; same guard as PS2141/PS2107/PS2122).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			// Type info pins the callee to the package-level fmt.Appendf.
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Appendf": true}); !ok {
				return true
			}
			// The format must be a string LITERAL with no '%' byte anywhere:
			// any '%' makes fmt transform the bytes (%%->%, %!verb(MISSING),
			// %!(NOVERB)) and breaks bit-identity.
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			format, err := strconv.Unquote(lit.Value)
			if err != nil || strings.IndexByte(format, '%') >= 0 {
				return true
			}
			var parent ast.Node
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			// The fix additionally requires an unnamed []byte destination
			// (append's return type then matches Appendf's []byte exactly)
			// and a non-empty literal (append(buf, ""...) is noise).
			fix := (*analysis.SuggestedFix)(nil)
			if len(format) > 0 && ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(call.Args[0])) {
				fix = ps3025Fix(f, call, lit, parent)
				if fix != nil {
					fixable++
				}
			}
			sites = append(sites, site{call, fix})
			return true
		})
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "fmt") > fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: ps3025Msg,
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3025Fix rewrites fmt.Appendf(buf, "lit") to append(buf, "lit"...): buf and
// the literal stay byte-verbatim in place; only the scaffolding is edited (the
// fmt.Appendf selector becomes append, and ... is inserted before the closing
// parenthesis). A comment inside a rewritten span would be destroyed, and the
// builtin append cannot stand as a bare statement or under go/defer — the fix
// is withheld then and the report stays advisory.
func ps3025Fix(f *ast.File, call *ast.CallExpr, lit *ast.BasicLit, parent ast.Node) *analysis.SuggestedFix {
	// fmt.Appendf(...) is a valid statement (its result may be discarded);
	// the builtin append must be used, so a bare-statement / go / defer call
	// cannot be rewritten — advisory.
	switch parent.(type) {
	case *ast.ExprStmt, *ast.GoStmt, *ast.DeferStmt:
		return nil
	}
	// The edited spans: the fmt.Appendf selector (which may carry an interior
	// comment: fmt /*c*/ .Appendf) and literal-end..call-end (where the ... is
	// inserted). A comment there would be silently destroyed — advisory then.
	if ps2111CommentIn(f, call.Fun.Pos(), call.Fun.End()) ||
		ps2111CommentIn(f, lit.End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace fmt.Appendf with append",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Fun.Pos(), End: call.Fun.End(), NewText: []byte("append")},
			{Pos: lit.End(), End: call.End(), NewText: []byte("...)")},
		},
	}
}
