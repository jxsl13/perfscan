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

// PS5061 reports fmt.Appendf(buf, "<lit>%c<lit>", r) — one bare %c rune verb
// spliced into literal text over a single rune operand — where a nested append /
// utf8.AppendRune chain writes the identical bytes with no format parse and no
// interface box. The %c companion of PS5059 (integer verbs) and PS5060 (%t/%q),
// and the interleaved-literal sibling of PS5040 (the bare %c).
var PS5061 = register(&lint.Check{
	ID:       "PS5061",
	Category: "alloc",
	Slug:     "appendf-c-rune-in-literal",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Appendf splicing one %c rune verb into literal text runs fmt's formatter; a nested append/utf8.AppendRune chain writes the same bytes directly",
		Text: `fmt.Appendf(buf, "[%c]", r) parses the format string, boxes r into an
interface and drives fmt's formatter to UTF-8-encode one rune between two
literal runs. The nested chain append(utf8.AppendRune(append(buf, "["...), r),
"]"...) writes the identical bytes — the literal segments as constant-string
spreads and the rune via utf8.AppendRune — straight into buf, with no format
parse and no boxing.

The rewrite is BIT-IDENTICAL: "%c" UTF-8-encodes the code point, emitting U+FFFD
for a value outside the rune range, which is exactly what utf8.AppendRune does;
the literal segments are re-emitted via strconv.Quote of their decoded text.
Verified against fmt.Appendf over every rune including the surrogate range and
out-of-range values.

The match is deliberately narrow — it is the whole safety story:
  - the callee is the package-level fmt.Appendf with exactly three arguments and
    no spread;
  - the format is a string literal containing EXACTLY ONE percent sign that is a
    bare %c (a flag, width, %%, or any other verb disqualifies it), with at
    least one non-empty literal segment (the bare %c is PS5040's);
  - the operand's type is lossless in rune (int32/rune, int8, int16, uint8,
    uint16) — wider kinds are excluded (fmt emits U+FFFD past int32's range while
    rune(x) would TRUNCATE), and a constant is excluded (rune(const) could
    overflow int32 at compile time); the operand is rune-wrapped only when its
    type is not already assignable to rune, reusing PS5040's operand gate;
  - the destination is a []byte-underlying expression (a literal nil is silent —
    append cannot take an untyped nil; a named []byte stays advisory), and, when
    the format has a LEADING literal, the chain appends it before the operand is
    evaluated, so the operand must be side-effect-free;
  - unicode/utf8 must be importable, and dropping this call orphans the fmt
    import only in a cgo file (advisory there; elsewhere the fix pipeline prunes
    it).
A comment inside the rewritten scaffolding withholds the fix.`,
		Before: `buf = fmt.Appendf(buf, "[%c]", r)`,
		After:  `buf = append(utf8.AppendRune(append(buf, "["...), r), "]"...)`,
		MeasuredWin: `On "[%c]" with a multibyte rune (Apple M2 Pro, go1.26): fmt.Appendf ~37 ns/op ` +
			`vs the append/utf8.AppendRune chain ~2 ns/op (~15x, 0 allocs/op either way when buf ` +
			`is reused) — the format parse and the interface box disappear.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5061",
		Doc:  "fmt.Appendf of one %c rune verb spliced into literal text instead of a nested append/utf8.AppendRune chain",
		Run:  runPS5061,
	},
})

func runPS5061(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		importAdded := false
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Appendf": true}); !ok {
				return true
			}
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			format, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			pct := strings.IndexByte(format, '%')
			if pct < 0 || pct == len(format)-1 || strings.IndexByte(format[pct+1:], '%') >= 0 {
				return true
			}
			if format[pct+1] != 'c' {
				return true
			}
			prefix, suffix := format[:pct], format[pct+2:]
			if prefix == "" && suffix == "" {
				return true
			}
			operand := call.Args[2]
			needsWrap, ok := ps5040RuneArg(pass, operand)
			if !ok {
				return true
			}
			buf := call.Args[0]
			if !ps2044ByteSliceUnderlying(pass.TypesInfo.TypeOf(buf)) {
				return true
			}

			var fix *analysis.SuggestedFix
			if ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(buf)) &&
				(prefix == "" || ps2033Inert(pass.TypesInfo, operand)) &&
				!ps2111CommentIn(f, call.Pos(), buf.Pos()) &&
				!ps2111CommentIn(f, buf.End(), operand.Pos()) &&
				!ps2111CommentIn(f, operand.End(), call.End()) {
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "utf8", "unicode/utf8")
				if usable && !(needImport && ps2107ImportsC(f)) {
					sel := call.Fun.(*ast.SelectorExpr)
					hasPre, hasSuf := prefix != "", suffix != ""
					castOpen, castClose := "", ""
					if needsWrap {
						castOpen, castClose = "rune(", ")"
					}
					open := useName + ".AppendRune("
					if hasSuf {
						open = "append(" + open
					}
					if hasPre {
						open += "append("
					}
					mid := ", "
					if hasPre {
						mid = ", " + strconv.Quote(prefix) + "...), "
					}
					mid += castOpen
					trail := castClose + ")"
					if hasSuf {
						trail = castClose + "), " + strconv.Quote(suffix) + "...)"
					}
					edits := []analysis.TextEdit{
						{Pos: sel.Pos(), End: buf.Pos(), NewText: []byte(open)},
						{Pos: buf.End(), End: operand.Pos(), NewText: []byte(mid)},
						{Pos: operand.End(), End: call.End(), NewText: []byte(trail)},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "unicode/utf8"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "replace fmt.Appendf with a nested append/utf8.AppendRune chain",
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, site{call, fix})
			return true
		})
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "fmt") > fixable || !ps2110ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "fmt.Appendf splicing one %c rune verb into literal text parses the format and boxes the operand; a nested append/utf8.AppendRune chain writes the identical bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}
