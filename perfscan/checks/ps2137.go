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

// PS2137 reports fmt.Sprint(x) and fmt.Sprintf("%v", x) with x an UNNAMED
// predeclared integer — fmt's format parse, interface boxing and reflection
// paid for the plain decimal string strconv yields directly.
var PS2137 = register(&lint.Check{
	ID:       "PS2137",
	Category: "alloc",
	Slug:     "sprint-int",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Sprint / fmt.Sprintf(\"%v\") of a plain integer has a direct strconv equivalent",
		Text: `fmt.Sprint(x) and fmt.Sprintf("%v", x) with x an integer box the
argument into an interface and dispatch through fmt's reflection-driven
printer just to produce the decimal digits strconv emits directly:

  x is int                             -> strconv.Itoa(x)
  x is int8/int16/int32/int64          -> strconv.FormatInt(int64(x), 10)
  x is uint/uint8/.../uint64/uintptr   -> strconv.FormatUint(uint64(x), 10)

The rewrite is restricted to operands whose static type is an UNNAMED
predeclared integer type (exactly int, int64, uint, ...). This is the
safety guard that makes it bit-identical: unlike %d (PS2107's territory),
%v and fmt.Sprint HONOR fmt.Stringer and fmt.Formatter — a NAMED integer
type with a String() method prints via String(), not as decimal digits, so
rewriting it would change the output. An unnamed predeclared type cannot
have methods, so it cannot be a Stringer or Formatter, and %v/Sprint of a
plain integer prints exactly what %d does — exactly the strconv form,
including MinInt64 (FormatInt works in uint64 magnitude space) and
MaxUint64 at full width.

The match is strict: fmt.Sprint with exactly one non-variadic argument, or
fmt.Sprintf whose format is a string LITERAL that is exactly "%v" with
exactly one further argument. "%d" belongs to PS2107; a string operand
under Sprint/"%v"/"%s" is the identity owned by PS2130 — both are
deliberately out of scope here, so no call is double-reported.

The fix replaces the WHOLE call, so the source fmt qualifier disappears —
an aliased fmt import works too, and when the rewrite removes the file's
last fmt reference the fix pipeline prunes the orphaned import. The
strconv import is added when missing (honoring an existing strconv alias).
The report stays advisory when strconv's name is shadowed at the call
site, when a comment overlaps the replaced range, or when a cgo file would
need its import block edited.`,
		Before: `s := fmt.Sprint(userID)`,
		After:  `s := strconv.Itoa(userID)`,
		MeasuredWin: `BenchmarkPS2137 (int 123456, Apple M2 Pro, go1.26):
fmt.Sprint(n) 45.3 ns/op 16 B/op 2 allocs -> strconv.Itoa(n) 14.8 ns/op
8 B/op 1 alloc (~3x; the interface boxing and fmt's reflection printer
vanish, leaving the one result-string allocation).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2137",
		Doc:  "fmt.Sprint / fmt.Sprintf(\"%v\") of a plain integer has a direct strconv equivalent",
		Run:  runPS2137,
	},
})

func runPS2137(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last
		// fmt reference and orphan the import. The fix pipeline prunes
		// such an orphan afterwards — except in a cgo file (import "C"),
		// whose import block is never edited, so there the fixes are
		// withheld and the reports stay advisory (same per-file collection
		// as PS2107).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		// All fixes of a run are applied together, so only the first fix
		// needing the strconv import carries the import edit (same
		// convention as PS2107).
		importAdded := false
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || call.Ellipsis.IsValid() {
				return true
			}
			arg, ok := ps2137Match(pass, call)
			if !ok {
				return true
			}
			// THE safety guard: the operand's static type must be an UNNAMED
			// predeclared integer. %v and fmt.Sprint honor fmt.Stringer and
			// fmt.Formatter, so a NAMED integer type with a String() method
			// prints via String(), not decimal — the rewrite would change the
			// output. An unnamed predeclared type cannot carry methods, so for
			// it the strconv form is provably bit-identical. types.Default
			// materializes an untyped constant as its default type (an
			// untyped int becomes int), exactly what Sprint's ...any sees;
			// a named type survives Default as *types.Named, never
			// *types.Basic, and is skipped entirely.
			basic, isBasic := types.Default(pass.TypesInfo.TypeOf(arg)).(*types.Basic)
			if !isBasic || basic.Info()&types.IsInteger == 0 {
				return true
			}
			var fix *analysis.SuggestedFix
			if !ps2107CommentsOverlap(f, call) {
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "strconv", "strconv")
				if usable && !(needImport && ps2107ImportsC(f)) {
					if argText, textOK := ps2107ExprText(arg); textOK {
						repl, replName := strconvDecimalRepl(basic, argText)
						// Reuse an existing strconv import's name (e.g. an
						// alias) by rewriting the replacement's leading
						// qualifier; repl always begins with "strconv.".
						if useName != "strconv" {
							repl = strings.Replace(repl, "strconv.", useName+".", 1)
						}
						edits := []analysis.TextEdit{
							{Pos: call.Pos(), End: call.End(), NewText: []byte(repl)},
						}
						if needImport && !importAdded {
							edits = append(edits, ps2107ImportEdit(f, "strconv"))
							importAdded = true
						}
						fix = &analysis.SuggestedFix{
							Message:   "replace the fmt call with " + replName,
							TextEdits: edits,
						}
						fixable++
					}
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
				Message: "fmt.Sprint(i) / fmt.Sprintf(\"%v\", i) on an integer pays fmt's reflection and boxing for a plain decimal; strconv.Itoa/FormatInt/FormatUint converts it directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2137Match matches call as fmt.Sprint(x) with exactly one argument or
// fmt.Sprintf("%v", x) whose format is a string literal that is exactly
// "%v", returning the operand. The callee is resolved by type information
// (astutil.PkgFuncCall), so an aliased fmt import matches and a shadowed
// fmt or same-named third-party package does not. "%d" is PS2107's; a
// string operand is filtered out by the caller's integer guard (PS2130's
// territory).
func ps2137Match(pass *analysis.Pass, call *ast.CallExpr) (ast.Expr, bool) {
	name, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Sprint": true, "Sprintf": true})
	if !ok {
		return nil, false
	}
	switch name {
	case "Sprint":
		// Exactly one operand: with two, Sprint inserts a separating space
		// between non-string operands and the result is no longer one
		// conversion.
		if len(call.Args) != 1 {
			return nil, false
		}
		return call.Args[0], true
	case "Sprintf":
		if len(call.Args) != 2 {
			return nil, false
		}
		lit, isLit := call.Args[0].(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			return nil, false
		}
		if v, err := strconv.Unquote(lit.Value); err != nil || v != "%v" {
			return nil, false
		}
		return call.Args[1], true
	}
	return nil, false
}

// strconvDecimalRepl builds the strconv decimal conversion for an operand
// of an unnamed predeclared integer type (shared by PS2107's %d arm and
// PS2137): Itoa for int, FormatUint with a value-preserving uint64
// widening for the unsigned kinds (uintptr included), FormatInt with an
// int64 widening for the remaining signed widths. FormatInt works in
// uint64 magnitude space, so MinInt64 needs no special case.
func strconvDecimalRepl(basic *types.Basic, argText string) (repl, replName string) {
	switch {
	case basic.Kind() == types.Int:
		return "strconv.Itoa(" + argText + ")", "strconv.Itoa"
	case basic.Info()&types.IsUnsigned != 0:
		return "strconv.FormatUint(uint64(" + argText + "), 10)", "strconv.FormatUint"
	default:
		return "strconv.FormatInt(int64(" + argText + "), 10)", "strconv.FormatInt"
	}
}
