package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3037 reports sort.SearchInts(a, x) and sort.SearchStrings(a, x) — a binary
// search driven through a per-probe closure — where slices.BinarySearch(a, x)
// runs the identical search without the closure indirection. It is the
// package-level-search sibling of PS3028/PS3109 (the BinarySearchFunc forms).
// sort.SearchFloat64s is deliberately excluded: it disagrees with
// slices.BinarySearch on NaN.
var PS3037 = register(&lint.Check{
	ID:       "PS3037",
	Category: "indirect",
	Slug:     "sortsearch-to-slices-binarysearch",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "sort.SearchInts/SearchStrings run a binary search through a per-probe closure; slices.BinarySearch is the same search without the indirection",
		Text: `sort.SearchInts(a, x) is defined as sort.Search(len(a), func(i int) bool
{ return a[i] >= x }): every probe of the binary search calls through a closure.
sort.SearchStrings is the same for []string. slices.BinarySearch(a, x) performs
the identical binary search over the ordered element type directly, with no
closure call per probe, and returns (index, found) — the index is exactly what
the sort.Search* function returns (the position of x, or where it would be
inserted), so binding the index and discarding found reproduces the original
value.

The rewrite is BIT-IDENTICAL for ints and strings: a[i] >= x and
cmp.Compare(a[i], x) >= 0 (BinarySearch's own ordering) agree for every pair of
integers and every pair of strings, so the returned insertion index is equal for
all inputs, verified exhaustively. sort.SearchFloat64s is NOT included: for a NaN
target, a[i] >= NaN is always false so SearchFloat64s returns len(a), while
slices.BinarySearch orders NaN first via cmp.Compare and returns 0 — a divergence
floats can hit, so that form is left alone.

The match is deliberately narrow — it is the whole safety story:
  - the callee is the package-level sort.SearchInts or sort.SearchStrings,
    pinned by type information (a shadowed sort never matches);
  - the fix fires only where the call is the SOLE right-hand side of a
    single-target assignment (i := sort.SearchInts(a, x) or i = ...), which it
    rewrites to i, _ := slices.BinarySearch(a, x) — binding the index and
    discarding found. In any other position (a bare expression, an index, a
    condition, a call argument) a two-result call cannot be spliced, so the
    report is advisory;
  - slices must be importable at the site, and because the rewrite drops these
    sort references the fix is withheld unless sort retains another use afterward
    (never orphaning the sort import — that residual case is advisory).
A comment inside the renamed selector or at the assignment target keeps the
report advisory.`,
		Before: `i := sort.SearchInts(a, x)`,
		After:  `i, _ := slices.BinarySearch(a, x)`,
		MeasuredWin: `On a 4096-element []int (Apple M2 Pro, go1.26): sort.SearchInts(a, x) ` +
			`~35 ns/op vs slices.BinarySearch(a, x) ~26 ns/op (~1.33x, 0 allocs/op either way) — ` +
			`the per-probe closure call is removed; the win grows with log2(len).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3037",
		Doc:  "sort.SearchInts/SearchStrings instead of slices.BinarySearch",
		Run:  runPS3037,
	},
})

func runPS3037(pass *analysis.Pass) (any, error) {
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
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "sort", map[string]bool{"SearchInts": true, "SearchStrings": true}); !ok {
				return true
			}
			sel := call.Fun.(*ast.SelectorExpr)

			// Fixable only where the call is the sole RHS of a single-target
			// assignment; the parent is the top of the ancestor stack.
			var lhs ast.Expr
			if len(stack) >= 1 {
				if as, ok := stack[len(stack)-1].(*ast.AssignStmt); ok &&
					len(as.Rhs) == 1 && as.Rhs[0] == call && len(as.Lhs) == 1 {
					lhs = as.Lhs[0]
				}
			}

			var fix *analysis.SuggestedFix
			if lhs != nil &&
				!ps2111CommentIn(f, sel.Pos(), sel.End()) &&
				!ps2111CommentIn(f, lhs.Pos(), lhs.End()) {
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "slices", "slices")
				if usable && !(needImport && ps2107ImportsC(f)) {
					edits := []analysis.TextEdit{
						{Pos: sel.Pos(), End: sel.End(), NewText: []byte(useName + ".BinarySearch")},
						{Pos: lhs.End(), End: lhs.End(), NewText: []byte(", _")},
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "slices"))
						importAdded = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "bind the index and discard found: " + useName + ".BinarySearch",
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, site{call, fix})
			return true
		})
		// Each fixable call removes ONE sort reference; withhold all fixes if
		// that would orphan the sort import.
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "sort") > fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "sort.Search* runs a binary search through a per-probe closure; slices.BinarySearch is the same search without the indirection (bind the index, discard found)",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}
