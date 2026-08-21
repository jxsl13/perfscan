package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS5047 reports fmt.Appendf(buf, "%b", n) — a lone "%b" verb
// binary-formatting one integer into a byte buffer — where strconv.AppendInt /
// strconv.AppendUint appends the identical binary digits without fmt's
// format parse or interface boxing. The integer "%b" sibling of PS5042 ("%d") and PS5043 ("%x"), and
// PS2035 ("%v") / PS2141 / PS5040 / PS5041.
var PS5047 = register(&lint.Check{
	ID:       "PS5047",
	Category: "alloc",
	Slug:     "appendf-b-int",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Appendf(buf, "%b", n) runs fmt's formatter to binary-print one integer; strconv.AppendInt/AppendUint writes the base-2 digits directly`,
		Text: `fmt.Appendf(buf, "%b", n) parses the format string, boxes n into an
interface and drives fmt's formatter through a pooled buffer — all to append
the base-2 (binary) digits of one integer. strconv.AppendInt(buf, int64(n), 2) (or
AppendUint for an unsigned operand) writes those exact binary digits directly into
buf's backing array, with no format parse and no boxing.

The rewrite is BIT-IDENTICAL: "%b" prints an integer in base 2, a leading '-'
for negatives included, which is exactly what AppendInt/AppendUint produce in base 2
(FormatInt works in uint64 magnitude space, so MinInt64 needs no special case).
Verified equal across every width and the full extremes.

The match is deliberately narrow — it is the whole safety story:
  - the callee is pinned by type information to the package-level fmt.Appendf
    (a shadowed fmt or a method named Appendf never matches) and must not
    spread its arguments;
  - the format is a string literal that is EXACTLY "%b" — any width, flag,
    or other verb disqualifies it;
  - the operand's default type is an UNNAMED predeclared INTEGER. A named
    integer type is excluded: "%b" consults fmt.Formatter for the operand, so a
    named type with a Format method formats via that method, not the plain
    value — while AppendInt/AppendUint always print the digits. An unnamed
    predeclared integer carries no methods, so the two are provably identical.
    A string, []byte, bool or float operand is not an integer (fmt %b of a float is the binary-exponent form), so none match, so they
    never match either;
  - the destination is an unnamed []byte, so the strconv result matches
    Appendf's exactly (a named byte-slice destination would change the
    expression's static type — advisory), and the fix is withheld unless the
    file keeps another fmt reference (so dropping this call never orphans the
    fmt import) and strconv is importable (no cgo file, no shadow).
A comment inside the rewritten scaffolding keeps the report advisory. Named
destinations, named or non-integer operands, and non-"%b" formats are reported
without a fix.`,
		Before: `buf = fmt.Appendf(buf, "%b", n)`,
		After:  `buf = strconv.AppendInt(buf, int64(n), 2)`,
		MeasuredWin: `fmt.Appendf(buf, "%b", n) ~32 ns/op vs strconv.AppendInt(buf, int64(n), 2) ` +
			`~13 ns/op (~2.5x, Apple M2 Pro, go1.26) — the win is the eliminated format parse ` +
			`and interface dispatch; both are 0 allocs when buf is reused, and the fmt path ` +
			`additionally boxes n (a heap allocation when the call escapes).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5047",
		Doc:  "fmt.Appendf(buf, \"%x\", n) hex of an integer instead of strconv.AppendInt/AppendUint(buf, ..., 2)",
		Run:  runPS5047,
	},
})

func runPS5047(pass *analysis.Pass) (any, error) {
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
			if v, err := strconv.Unquote(lit.Value); err != nil || v != "%b" {
				return true
			}
			appendName, prefix, suffix, ok := ps5047IntCase(pass, call.Args[2])
			if !ok {
				return true
			}
			var fix *analysis.SuggestedFix
			if ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(call.Args[0])) &&
				!ps2111CommentIn(f, call.Args[0].End(), call.Args[2].Pos()) &&
				!ps2111CommentIn(f, call.Args[2].End(), call.End()) {
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "strconv", "strconv")
				if usable && !(needImport && ps2107ImportsC(f)) {
					sel := call.Fun.(*ast.SelectorExpr)
					edits := []analysis.TextEdit{
						{Pos: sel.Pos(), End: sel.End(), NewText: []byte(useName + "." + appendName)},
						{Pos: call.Args[0].End(), End: call.Args[2].Pos(), NewText: []byte(prefix)},
						{Pos: call.Args[2].End(), End: call.End(), NewText: []byte(suffix)},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "strconv"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "replace fmt.Appendf(buf, \"%b\", n) with " + useName + "." + appendName,
						TextEdits: edits,
					}
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
				Message: "fmt.Appendf(buf, \"%b\", n) parses the format and boxes n to binary-print one integer; strconv.AppendInt/AppendUint appends the base-2 digits directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5047IntCase classifies an unnamed predeclared integer operand into the
// strconv.Append{Int,Uint} call scaffolding: the callee name and the prefix
// (replacing `, "%b", `) and suffix (replacing the closing `)`) spliced around
// the untouched operand. ok is false for named types, bool, float, and
// non-basic operands — types.Default yields a *types.Basic only for the
// unnamed predeclared type; a named integer survives as *types.Named and is
// skipped (its Format method could hijack "%b").
func ps5047IntCase(pass *analysis.Pass, arg ast.Expr) (appendName, prefix, suffix string, ok bool) {
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
			return "AppendUint", ", ", ", 2)", true
		}
		// uint64(u) is value-preserving for every narrower unsigned width
		// (uintptr included).
		return "AppendUint", ", uint64(", "), 2)", true
	}
	if basic.Kind() == types.Int64 {
		return "AppendInt", ", ", ", 2)", true
	}
	// int64(i) is value-preserving for every narrower signed width.
	return "AppendInt", ", int64(", "), 2)", true
}
