package checks

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS5053 reports strconv.Quote(a) == strconv.Quote(b) (and !=) — quoting two
// strings only to compare them — where a == b compares the strings directly,
// with no quoting pass and no throwaway string. The string sibling of PS5048
// (strconv.Itoa compare) and PS5051 (FormatInt/FormatUint compare): strconv.Quote
// is a bijection, so equal quoted forms appear exactly when the strings are equal.
var PS5053 = register(&lint.Check{
	ID:       "PS5053",
	Category: "alloc",
	Slug:     "quote-compare-to-string-compare",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strconv.Quote(a) == strconv.Quote(b) quotes both strings just to compare them; a == b compares the strings directly",
		Text: `strconv.Quote(a) == strconv.Quote(b) converts each string to its
double-quoted Go literal — scanning every byte to escape quotes, backslashes,
control and non-printable runes, into a throwaway string — and then compares the
two literals. Because strconv.Quote is a bijection (strconv.Unquote inverts it,
so distinct strings have distinct quoted forms and equal strings identical ones),
the quoted comparison answers exactly a == b, a single string comparison with no
quoting pass. The == and != forms both collapse: Quote(a) == Quote(b) is a == b,
Quote(a) != Quote(b) is a != b.

The rewrite is BIT-IDENTICAL for equality and inequality only, verified over
adversarial strings including embedded quotes, backslashes, control bytes, and
invalid UTF-8. It is deliberately restricted to == and != — ordering does NOT
carry over (the quoted form's leading '"' and escaping reorder the byte
sequence), so those are left alone. Both operands are evaluated exactly once.

The match is deliberately narrow: an == or != comparison whose BOTH operands are
a single-argument call of the package-level strconv.Quote (a shadowed strconv,
or the different strconv.QuoteToASCII / QuoteToGraphic, never matches). Each Quote
argument is a plain string expression, and every Go operator that can appear in
one (string concatenation with +, and higher) binds tighter than == / !=, so
unwrapping it needs no parentheses. The fix drops both Quote(...) wrappers,
keeping the two arguments and the operator byte-verbatim; it is withheld
(advisory report only) when a comment sits inside a removed wrapper, or when
removing the two strconv references would orphan the strconv import.`,
		Before: `if strconv.Quote(a) == strconv.Quote(b) {`,
		After:  `if a == b {`,
		MeasuredWin: `On two ~19-char strings (Apple M2 Pro, go1.26): strconv.Quote(a) == ` +
			`strconv.Quote(b) ~177 ns/op vs a == b ~0.6 ns/op (~270x) — both quoting passes ` +
			`and the throwaway literals replaced by one direct string comparison (0 allocs when ` +
			`the quoted strings do not escape, and one heap allocation each when they do).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5053",
		Doc:  "strconv.Quote(a) == / != strconv.Quote(b) instead of a == / != b",
		Run:  runPS5053,
	},
})

func runPS5053(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			bin *ast.BinaryExpr
			fix *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
				return true
			}
			left, lok := ps5053QuoteCall(pass, bin.X)
			right, rok := ps5053QuoteCall(pass, bin.Y)
			if !lok || !rok {
				return true
			}
			var fix *analysis.SuggestedFix
			// Drop both Quote(...) wrappers; a comment inside either removed span
			// withholds the fix.
			if !ps2109CommentBetween(f, left.Pos(), left.Args[0].Pos()) &&
				!ps2109CommentBetween(f, left.Args[0].End(), left.End()) &&
				!ps2109CommentBetween(f, right.Pos(), right.Args[0].Pos()) &&
				!ps2109CommentBetween(f, right.Args[0].End(), right.End()) {
				fix = &analysis.SuggestedFix{
					Message: "compare the strings directly",
					TextEdits: []analysis.TextEdit{
						{Pos: left.Pos(), End: left.Args[0].Pos()},
						{Pos: left.Args[0].End(), End: left.End()},
						{Pos: right.Pos(), End: right.Args[0].Pos()},
						{Pos: right.Args[0].End(), End: right.End()},
					},
				}
				fixable++
			}
			sites = append(sites, site{bin, fix})
			return true
		})
		// Each fixable comparison removes TWO strconv references (both Quote
		// selectors); withhold all fixes if that would orphan the import.
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "strconv") > 2*fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.bin.Pos(),
				End:     st.bin.End(),
				Message: "strconv.Quote(a) " + st.bin.Op.String() + " strconv.Quote(b) quotes two strings just to compare them; a " + st.bin.Op.String() + " b compares the strings directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5053QuoteCall reports whether e is a single-argument call of the
// package-level strconv.Quote, returning the call.
func ps5053QuoteCall(pass *analysis.Pass, e ast.Expr) (*ast.CallExpr, bool) {
	call, ok := ps2109Unparen(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil, false
	}
	if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "strconv", map[string]bool{"Quote": true}); !ok {
		return nil, false
	}
	return call, true
}
