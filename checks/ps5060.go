package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS5060 reports fmt.Appendf(buf, "<lit>%t<lit>", b) and its "%q" string twin —
// one bare bool or string-quote verb spliced into literal text over a single
// operand — where a nested append / strconv.AppendBool|AppendQuote chain writes
// the identical bytes with no format parse and no interface box. The bool/quote
// companion of PS5059 (the integer verbs %d/%x/%b/%o in literal text).
var PS5060 = register(&lint.Check{
	ID:       "PS5060",
	Category: "alloc",
	Slug:     "appendf-bool-quote-verb-in-literal",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Appendf splicing one %t or %q verb into literal text runs fmt's formatter; a nested append/strconv chain writes the same bytes directly",
		Text: `fmt.Appendf(buf, "ok=%t;", b) parses the format string, boxes the
operand into an interface and drives fmt's formatter to append one value between
two literal runs. The nested chain append(strconv.AppendBool(append(buf,
"ok="...), b), ";"...) writes the identical bytes — the literal segments as
constant-string spreads and the value via strconv.AppendBool ("%t") or
strconv.AppendQuote ("%q") — straight into buf, with no format parse and no
boxing.

The rewrite is BIT-IDENTICAL: "%t" prints "true"/"false", exactly what
AppendBool writes, and "%q" of a string is its double-quoted Go literal, exactly
what AppendQuote writes; the literal segments are re-emitted via strconv.Quote of
their decoded text, denoting the identical runtime strings. Verified against
fmt.Appendf over bools and adversarial strings (embedded quotes, control bytes,
invalid UTF-8).

The match is deliberately narrow — it is the whole safety story:
  - the callee is the package-level fmt.Appendf with exactly three arguments and
    no spread;
  - the format is a string literal containing EXACTLY ONE percent sign that is a
    bare %t or %q (a flag, width, %%, or any other verb disqualifies it), with
    at least one non-empty literal segment (the bare verb is PS5041's for %q);
  - the operand's default type is an UNNAMED predeclared bool (for %t) or string
    (for %q) — a named type is excluded, since its Format method could hijack the
    verb, and %q over a rune or []byte is a different encoding;
  - the destination is a []byte-underlying expression (a literal nil is silent —
    append cannot take an untyped nil; a named []byte stays advisory, the fix
    needs an unnamed []byte), and, when the format has a LEADING literal, the
    chain appends it before the operand is evaluated, so the operand must be
    side-effect-free;
  - strconv must be importable, and dropping this call orphans the fmt import
    only in a cgo file (advisory there; elsewhere the fix pipeline prunes it).
A comment inside the rewritten scaffolding withholds the fix.`,
		Before: `buf = fmt.Appendf(buf, "ok=%t;", b)`,
		After:  `buf = append(strconv.AppendBool(append(buf, "ok="...), b), ";"...)`,
		MeasuredWin: `On "ok=%t;" with a bool (Apple M2 Pro, go1.26): fmt.Appendf ~35 ns/op ` +
			`vs the append/strconv.AppendBool chain a few ns/op (0 allocs/op either way when buf ` +
			`is reused) — the format parse and the interface box disappear.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5060",
		Doc:  "fmt.Appendf of one %t or %q verb spliced into literal text instead of a nested append/strconv chain",
		Run:  runPS5060,
	},
})

func runPS5060(pass *analysis.Pass) (any, error) {
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
			verb := format[pct+1]
			if verb != 't' && verb != 'q' {
				return true
			}
			prefix, suffix := format[:pct], format[pct+2:]
			if prefix == "" && suffix == "" {
				return true
			}
			operand := call.Args[2]
			appendName, ok := ps5060VerbArg(pass, verb, operand)
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
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "strconv", "strconv")
				if usable && !(needImport && ps2107ImportsC(f)) {
					sel := call.Fun.(*ast.SelectorExpr)
					hasPre, hasSuf := prefix != "", suffix != ""
					open := useName + "." + appendName + "("
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
					trail := ")"
					if hasSuf {
						trail = "), " + strconv.Quote(suffix) + "...)"
					}
					edits := []analysis.TextEdit{
						{Pos: sel.Pos(), End: buf.Pos(), NewText: []byte(open)},
						{Pos: buf.End(), End: operand.Pos(), NewText: []byte(mid)},
						{Pos: operand.End(), End: call.End(), NewText: []byte(trail)},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "strconv"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "replace fmt.Appendf with a nested append/strconv." + appendName + " chain",
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
				Message: "fmt.Appendf splicing one %t or %q verb into literal text parses the format and boxes the operand; a nested append/strconv chain writes the identical bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5060VerbArg returns the strconv appender for a %t or %q verb over an
// operand of the required UNNAMED predeclared type (bool for %t, string for
// %q). ok is false for named types (whose Format method could hijack the verb),
// a %q over a non-string (a rune or []byte is a different encoding), or a
// non-basic operand.
func ps5060VerbArg(pass *analysis.Pass, verb byte, arg ast.Expr) (appendName string, ok bool) {
	t := pass.TypesInfo.TypeOf(arg)
	if t == nil {
		return "", false
	}
	basic, isB := types.Default(t).(*types.Basic)
	if !isB {
		return "", false
	}
	switch verb {
	case 't':
		if basic.Kind() == types.Bool {
			return "AppendBool", true
		}
	case 'q':
		if basic.Info()&types.IsString != 0 {
			return "AppendQuote", true
		}
	}
	return "", false
}
