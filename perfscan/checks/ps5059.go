package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5059 reports fmt.Appendf(buf, "<lit>%d<lit>", n) — one bare integer verb
// (%d, %x, %b, %o) spliced into literal text over a single integer operand —
// where a nested append / strconv.AppendInt chain writes the identical bytes
// with no format parse and no interface box. The scalar-verb sibling of PS2044
// (the %s-splice) and the interleaved-literal companion of PS5042/PS5043/PS5047
// (the bare integer verbs).
var PS5059 = register(&lint.Check{
	ID:       "PS5059",
	Category: "alloc",
	Slug:     "appendf-int-verb-in-literal",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Appendf splicing one integer verb into literal text runs fmt's formatter; a nested append/strconv.AppendInt chain writes the same bytes directly",
		Text: `fmt.Appendf(buf, "id=%d;", n) parses the format string, boxes n into an
interface and drives fmt's formatter through a pooled buffer to append one
integer between two literal runs. The nested chain
append(strconv.AppendInt(append(buf, "id="...), int64(n), 10), ";"...) writes the
identical bytes — the literal segments as constant-string spreads and the
integer via strconv.AppendInt/AppendUint — straight into buf, with no format
parse and no boxing.

The rewrite is BIT-IDENTICAL: "%d"/"%b"/"%o"/"%x" print an integer in base
10/2/8/16 (lowercase), a leading '-' for negatives included, exactly what
AppendInt/AppendUint produce for that base; the literal segments are re-emitted
via strconv.Quote of their decoded text, denoting the identical runtime strings.
Verified against fmt.Appendf across widths and the base extremes.

The match is deliberately narrow — it is the whole safety story:
  - the callee is the package-level fmt.Appendf with exactly three arguments and
    no spread;
  - the format is a string literal containing EXACTLY ONE percent sign, and it
    is a bare %d, %x, %b, or %o (a flag, width, %%, %X uppercase, or any other
    verb disqualifies it — %X has no lowercase-only AppendInt form), with at
    least one non-empty literal segment (the bare verb is PS5042/PS5043/PS5047's);
  - the operand's default type is an UNNAMED predeclared INTEGER (a named
    integer type is excluded: its Format method could hijack the verb) — a
    string, bool, or float operand is not an integer, so none match;
  - the destination is an unnamed []byte (a named byte-slice destination would
    change the expression's static type — advisory), and, when the format has a
    LEADING literal, the chain appends that literal before the operand is
    evaluated, so the operand must be side-effect-free (a trailing-only literal
    needs no such guard, matching Go's left-to-right argument order);
  - strconv must be importable, and the fix is withheld if dropping this call
    would orphan the fmt import in a cgo file (elsewhere the fix pipeline prunes
    the orphan).
A comment inside the rewritten scaffolding withholds the fix.`,
		Before: `buf = fmt.Appendf(buf, "id=%d;", n)`,
		After:  `buf = append(strconv.AppendInt(append(buf, "id="...), int64(n), 10), ";"...)`,
		MeasuredWin: `On "id=%d;" with a 7-digit int (Apple M2 Pro, go1.26): ` +
			`fmt.Appendf(buf, "id=%d;", n) ~45 ns/op vs the append/strconv chain ~9.7 ns/op ` +
			`(~4.6x, 0 allocs/op either way when buf is reused) — the format parse and the ` +
			`interface box disappear.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5059",
		Doc:  "fmt.Appendf of one integer verb spliced into literal text instead of a nested append/strconv chain",
		Run:  runPS5059,
	},
})

var ps5059VerbBase = map[byte]string{'d': "10", 'b': "2", 'o': "8", 'x': "16"}

func runPS5059(pass *analysis.Pass) (any, error) {
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
			// Exactly one percent sign, a bare integer verb, and at least one
			// non-empty literal segment.
			pct := strings.IndexByte(format, '%')
			if pct < 0 || pct == len(format)-1 || strings.IndexByte(format[pct+1:], '%') >= 0 {
				return true
			}
			base, ok := ps5059VerbBase[format[pct+1]]
			if !ok {
				return true
			}
			prefix, suffix := format[:pct], format[pct+2:]
			if prefix == "" && suffix == "" {
				return true
			}
			operand := call.Args[2]
			appendName, castOpen, castClose, ok := ps5059IntArg(pass, operand)
			if !ok {
				return true
			}
			buf := call.Args[0]
			// The destination must be a []byte-underlying expression for the
			// append chain to be spellable at all. A literal nil is silent (not
			// advisory): append cannot take an untyped nil first argument, so the
			// chain is unspellable for that shape. A named []byte is byte-slice
			// underlying and stays advisory below (the unnamed gate is the fix's).
			if !ps2044ByteSliceUnderlying(pass.TypesInfo.TypeOf(buf)) {
				return true
			}

			var fix *analysis.SuggestedFix
			// Unnamed []byte destination; a leading literal write reorders the
			// operand's evaluation after that write, so the operand must be inert.
			if ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(buf)) &&
				(prefix == "" || ps2033Inert(pass.TypesInfo, operand)) &&
				!ps2111CommentIn(f, call.Pos(), buf.Pos()) &&
				!ps2111CommentIn(f, buf.End(), operand.Pos()) &&
				!ps2111CommentIn(f, operand.End(), call.End()) {
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "strconv", "strconv")
				if usable && !(needImport && ps2107ImportsC(f)) {
					sel := call.Fun.(*ast.SelectorExpr)
					hasPre, hasSuf := prefix != "", suffix != ""
					// Openers replacing "fmt.Appendf(".
					open := useName + "." + appendName + "("
					if hasSuf {
						open = "append(" + open
					}
					if hasPre {
						open += "append("
					}
					// Between buf and the operand: close the prefix append (if
					// any) and open the integer cast (if any).
					mid := ", "
					if hasPre {
						mid = ", " + strconv.Quote(prefix) + "...), "
					}
					mid += castOpen
					// After the operand: close the cast, add the base, and close
					// with the trailing literal append (if any).
					trail := castClose + ", " + base + ")"
					if hasSuf {
						trail += ", " + strconv.Quote(suffix) + "...)"
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
		// Each fixable call drops one fmt reference; if that empties the fmt
		// import the fix pipeline prunes it, except in a cgo file where the
		// import block is never edited (advisory there).
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "fmt") > fixable || !ps2110ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "fmt.Appendf splicing one integer verb into literal text parses the format and boxes the operand; a nested append/strconv.AppendInt chain writes the identical bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5059IntArg classifies an unnamed predeclared integer operand into the
// strconv.Append{Int,Uint} callee and the value-preserving cast (int64/uint64)
// spliced around the untouched operand, base left to the caller. ok is false for
// named types (whose Format method could hijack the verb), bool, float, and
// non-basic operands.
func ps5059IntArg(pass *analysis.Pass, arg ast.Expr) (appendName, castOpen, castClose string, ok bool) {
	t := pass.TypesInfo.TypeOf(arg)
	if t == nil {
		return "", "", "", false
	}
	basic, isB := types.Default(t).(*types.Basic)
	if !isB || basic.Info()&types.IsInteger == 0 {
		return "", "", "", false
	}
	if basic.Info()&types.IsUnsigned != 0 {
		if basic.Kind() == types.Uint64 {
			return "AppendUint", "", "", true
		}
		return "AppendUint", "uint64(", ")", true
	}
	if basic.Kind() == types.Int64 {
		return "AppendInt", "", "", true
	}
	return "AppendInt", "int64(", ")", true
}
