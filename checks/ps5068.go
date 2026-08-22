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

// PS5068 reports fmt.Appendf(buf, "%e", f) — and the %E/%f/%F/%G float verbs —
// formatting one float into a byte buffer, where strconv.AppendFloat appends the
// identical digits without fmt's format parse or interface boxing. The
// fixed-precision float sibling of PS5042/PS5043/PS5047; the shortest %g form is
// PS5015's (which explicitly leaves %e/%f alone because they default to 6
// digits, not the shortest form).
var PS5068 = register(&lint.Check{
	ID:       "PS5068",
	Category: "alloc",
	Slug:     "appendf-float-verb",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `fmt.Appendf(buf, "%e", f) runs fmt's formatter to print one float; strconv.AppendFloat writes the identical digits directly`,
		Text: `fmt.Appendf(buf, "%e", f) parses the format string, boxes f into an
interface and drives fmt's formatter through a pooled buffer — all to append the
%e (or %E/%f/%F/%G) form of one float. strconv.AppendFloat(buf, f, 'e', 6, 64)
writes those exact digits directly into buf's backing array, with no format parse
and no boxing.

The rewrite is BIT-IDENTICAL. fmt's default precision for %e, %E, %f and %F is 6
digits, which is exactly strconv.AppendFloat's prec 6 with format bytes 'e', 'E',
'f', 'f'; %G maps onto 'G' with the shortest precision (-1). The operand's own
bit size (32 or 64) is passed as the bitSize argument so the same rounding
applies — verified equal for -0, NaN, both infinities, subnormals, MaxFloat, and
hundreds of thousands of random float32/float64 values across every verb.

The match is deliberately narrow — it is the whole safety story:
  - the callee is the package-level fmt.Appendf with three arguments and no
    spread;
  - the format is a string literal that is EXACTLY "%e", "%E", "%f", "%F" or "%G"
    — any width, flag, precision, or other verb disqualifies it (the shortest %g
    is PS5015's);
  - the operand's default type is an UNNAMED predeclared FLOAT (float32 or
    float64). A named float type is excluded (its Format method could hijack the
    verb); a float32 operand is widened as float64(f) with bitSize 32, which is
    value-preserving and reproduces fmt's float32 rounding;
  - the destination is an unnamed []byte, so the strconv result matches
    Appendf's exactly, and the fix is withheld unless the file keeps another fmt
    reference (dropping this call never orphans the fmt import) and strconv is
    importable.
A comment inside the rewritten scaffolding keeps the report advisory.`,
		Before: `buf = fmt.Appendf(buf, "%f", f)`,
		After:  `buf = strconv.AppendFloat(buf, f, 'f', 6, 64)`,
		MeasuredWin: `fmt.Appendf(buf, "%f", f) ~50 ns/op vs strconv.AppendFloat(buf, f, 'f', 6, 64) ` +
			`~25 ns/op (~2x, Apple M2 Pro, go1.26) — the eliminated format parse and interface ` +
			`dispatch; both are 0 allocs when buf is reused.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5068",
		Doc:  "fmt.Appendf(buf, \"%e\"/%E/%f/%F/%G, f) instead of strconv.AppendFloat(buf, f, verb, prec, bitSize)",
		Run:  runPS5068,
	},
})

// ps5068Verb maps a bare float format verb to strconv.AppendFloat's format byte
// and precision. %g is omitted: it is PS5015's (shortest form over an integer or
// float).
var ps5068Verb = map[string][2]string{
	"%e": {"'e'", "6"},
	"%E": {"'E'", "6"},
	"%f": {"'f'", "6"},
	"%F": {"'f'", "6"},
	"%G": {"'G'", "-1"},
}

func runPS5068(pass *analysis.Pass) (any, error) {
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
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			vc, ok := ps5068Verb[v]
			if !ok {
				return true
			}
			prefix, suffix, ok := ps5068FloatCase(pass, call.Args[2], vc[0], vc[1])
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
						{Pos: sel.Pos(), End: sel.End(), NewText: []byte(useName + ".AppendFloat")},
						{Pos: call.Args[0].End(), End: call.Args[2].Pos(), NewText: []byte(prefix)},
						{Pos: call.Args[2].End(), End: call.End(), NewText: []byte(suffix)},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "strconv"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "replace fmt.Appendf(buf, \"" + v + "\", f) with " + useName + ".AppendFloat",
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
				Message: "fmt.Appendf(buf, \"%e\"/%f/..., f) parses the format and boxes f to print one float; strconv.AppendFloat appends the identical digits directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5068FloatCase classifies an unnamed predeclared float operand into the
// strconv.AppendFloat call scaffolding: the prefix (replacing `, "%e", `) and
// suffix (replacing the closing `)`) spliced around the untouched operand, given
// the verb's format byte and precision. ok is false for named types, integers,
// and non-basic operands — types.Default yields a *types.Basic only for the
// unnamed predeclared type; a named float survives as *types.Named and is
// skipped (its Format method could hijack the verb).
func ps5068FloatCase(pass *analysis.Pass, arg ast.Expr, fmtByte, prec string) (prefix, suffix string, ok bool) {
	t := pass.TypesInfo.TypeOf(arg)
	if t == nil {
		return "", "", false
	}
	basic, isB := types.Default(t).(*types.Basic)
	if !isB || basic.Info()&types.IsFloat == 0 {
		return "", "", false
	}
	if basic.Kind() == types.Float64 {
		return ", ", ", " + fmtByte + ", " + prec + ", 64)", true
	}
	// float64(f) is value-preserving for a float32 operand, and bitSize 32
	// reproduces fmt's float32 rounding.
	return ", float64(", "), " + fmtByte + ", " + prec + ", 32)", true
}
