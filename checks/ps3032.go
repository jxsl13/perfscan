package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3032 reports sort.IsSorted over a sort.IntSlice / sort.StringSlice
// adapter — the wrapper-adapter spelling of an ascending sortedness check —
// and rewrites it to the generic slices.IsSorted(x). Sibling of PS3008,
// which handles the sort.IntsAreSorted/StringsAreSorted helper spelling of
// the same predicate, exactly as PS3105 is PS3104's adapter-form sibling
// for the sort itself.
var PS3032 = register(&lint.Check{
	ID:       "PS3032",
	Category: "indirect",
	Slug:     "sortissorted-adapter-to-slices-issorted",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "sort.IsSorted over a sort.IntSlice/StringSlice adapter is slices.IsSorted spelled the long way; call the generic predicate directly",
		Text: `sort.IsSorted(sort.IntSlice(x)) wraps the slice in the sort.IntSlice
adapter and scans it through sort.Interface — an interface Len() call
plus a Less(i, j) dispatch per adjacent pair, each comparison paying a
virtual call and the bounds checks of the adapter's indexing. The
generic slices.IsSorted (Go 1.21) is monomorphized for the concrete
element type: the comparison inlines to a machine compare with no
interface hop. Unlike the helper spelling sort.IntsAreSorted (which
became a one-line wrapper over slices.IsSorted in go1.22, see PS3008),
the adapter spelling still pays the sort.Interface indirection on EVERY
toolchain: sort.IsSorted takes a sort.Interface and cannot devirtualize
the adapter, so here the rewrite is not merely directness — it measures
a real win on modern toolchains too (plus one small allocation saved:
boxing the adapter into the interface). This check is PS3008's sibling:
PS3008 matches the helper spelling sort.IntsAreSorted(x); PS3032 matches
the equivalent adapter spelling sort.IsSorted(sort.IntSlice(x)) /
sort.IsSorted(sort.StringSlice(x)) that PS3008 does not — the same
division of labor as PS3104 (helpers) versus PS3105 (adapters) for the
sort itself.

The result is BIT-IDENTICAL. The predicate is a pure bool — no elements
move, so every tie/stability hazard that blocks the sort-family float
REORDERING fixes is inapplicable. Both spellings answer "is no adjacent
pair descending under <": sort.IntSlice.Less and sort.StringSlice.Less
are the plain x[i] < x[j], and slices.IsSorted's cmp.Less compiles to
that identical < for int and string (its NaN clause is constant-false
for non-float types). Both scan every adjacent pair with a side-effect-
free comparison, so even the scan order is unobservable (in fact both
stdlib implementations walk high-to-low). Both return true for len < 2,
nil included. The conversion is a zero-copy reinterpretation of the same
slice header, and the operand expression is kept verbatim in place as
the sole argument — single evaluation preserved, nothing duplicated.

sort.Float64Slice is deliberately EXCLUDED and never flagged — the same
bright no-float line the whole sort family draws (PS3104/PS3105/PS3008):
Float64Slice.Less has NaN-ordering semantics, and perfscan does not
special-case float comparisons even where a rewrite could be argued.

The DESCENDING spelling sort.IsSorted(sort.Reverse(sort.IntSlice(x)))
is deliberately NOT matched: slices has no comparator-free descending
IsSorted, and rewriting to slices.IsSortedFunc with a flipped comparator
trades one indirection for another — no useful target, so it stays out.

Both selectors are resolved with type information: the callee must be
the package function sort.IsSorted — never a shadowed sort, a same-named
local, or a method — and its single argument must be a fresh CONVERSION
to the named type sort.IntSlice or sort.StringSlice. A pre-built value
of those types (a variable, a function result) is never matched: only
the direct conversion form guarantees the operand is the underlying
[]int/[]string, kept verbatim in place by the fix. An untyped operand
(sort.IsSorted(sort.IntSlice(nil))) is skipped — slices.IsSorted cannot
infer its type parameters from nil. The call is rewritten wherever the
bool is consumed (condition, assignment, return, argument); both
spellings are primary expressions, so the swap needs no parenthesization
in any parent.

The fix edits imports as needed: the slices import is added when missing
(reusing an existing alias when the file imports slices under another
name), and the sort import is dropped when the rewrites remove the
file's LAST sort references — each rewritten call gives up TWO of them
(the sort.IsSorted callee and the sort.IntSlice/StringSlice conversion),
and the import is dropped only when the file holds none besides those.
When the rewrites replace the whole import, the sort spec is swapped for
"slices" in place. A dot- or blank-imported slices, a comment inside the
rewritten call punctuation, or a cgo file (whose import block must not
be edited) keeps the report advisory.

The report only fires when the effective language version is at least
go1.21 — below that slices.IsSorted does not exist and the advice would
be moot. Note that per-file version pinning itself only exists from
go1.21 on, so go/types clamps a //go:build go1.20 (or older) line to
go1.21 and such files are still flagged; in practice the gate blocks via
the module's go directive (pass.Pkg.GoVersion()) when a whole module
still declares go < 1.21.`,
		Before: `ok := sort.IsSorted(sort.IntSlice(xs))`,
		After:  `ok := slices.IsSorted(xs)`,
		MeasuredWin: `BenchmarkPS3032 (a sorted 10k-element []int scanned per op —
worst case, the full pass over every adjacent pair — Apple M2 Pro,
go1.26): 18.9 µs/op and 1 alloc/op (boxing the adapter into
sort.Interface, 24 B) before vs 2.9 µs/op and 0 allocs after — about
6.5x. Unlike PS3008's helpers, sort.IsSorted never became a wrapper over
slices.IsSorted: it drives Len and Less through the interface on every
adjacent pair on every toolchain, so the dispatch cost is real on
go1.26, not just on go1.21.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3032",
		Doc:  "sort.IsSorted over a sort.IntSlice/StringSlice adapter instead of the generic slices.IsSorted",
		Run:  runPS3032,
	},
})

func runPS3032(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.IsSorted arrived with the slices package in go1.21,
			// the same gate PS3104/PS3105/PS3008 use; below that the
			// advice is moot, so stay silent entirely.
			continue
		}
		// Collect first, decide import edits once per file: every fixable
		// call removes TWO sort references (sort.IsSorted and the inner
		// conversion), and whether the sort import is orphaned depends on
		// ALL of them together (same per-file site collection as PS3105).
		type site struct {
			call *ast.CallExpr
			name string // the inner adapter type: "IntSlice" or "StringSlice"
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the receiver-less package
			// function sort.IsSorted: a shadowed sort resolves sel.Sel to
			// some other object, and a method carries a receiver.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "sort" || fn.Name() != "IsSorted" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// The single argument must be a fresh adapter CONVERSION.
			// Anything else — a pre-built adapter value, a custom
			// sort.Interface, sort.Float64Slice, a sort.Reverse wrapper,
			// an untyped nil operand — is never matched (PS3105's shared
			// adapter matcher enforces all of that).
			name, x, matched := ps3105Adapter(pass, call.Args[0])
			if !matched {
				return true
			}
			fix := ps3032Fix(pass, f, call, x)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, name, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call holds exactly TWO sort references — the
		// sort.IsSorted selector's package identifier and the inner
		// sort.IntSlice/StringSlice conversion's — so the rewrites orphan
		// the import only when the file's sort reference count equals
		// exactly twice the fixable sites (the operand kept verbatim can
		// itself mention sort, in which case the count is higher and the
		// import stays). The slices import is needed when the FILE lacks
		// one — a site-local shadow only suppresses that site's fix, never
		// the file's import edit.
		slicesImported := false
		for _, imp := range f.Imports {
			if imp.Path.Value == `"slices"` {
				slicesImported = true
				break
			}
		}
		needImport := fixable > 0 && !slicesImported
		orphansSort := fixable > 0 && pkgRefCount(pass, f, "sort") == 2*fixable
		importEdits, importsOK := ps3104ImportEdits(f, needImport, orphansSort)
		if !importsOK {
			// cgo file needing import surgery, or a sort spec we could not
			// locate: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edits (same convention as
			// PS3104/PS3105).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			elem := ps3105Targets[st.name]
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "sort.IsSorted(sort." + st.name + "(...)) scans through the sort.Interface adapter (an interface Len plus a Less dispatch per adjacent pair); slices.IsSorted checks the concrete " + elem + " directly with the identical boolean result",
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3032Fix builds the slices.IsSorted(x) rewrite for one call, or nil when
// a guard fails and the report must stay advisory. Only the punctuation and
// adapter conversion around the operand are replaced; the operand expression
// stays untouched in place, preserving its text and single evaluation (same
// technique as PS3105). Both spellings are primary expressions, so the swap
// needs no parenthesization in any parent. Import edits are appended later,
// once per file.
func ps3032Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, x ast.Expr) *analysis.SuggestedFix {
	slicesName, _, usable := ps3104SlicesName(pass, f, call.Pos())
	if !usable {
		return nil
	}
	// The replaced spans are the call text around the operand — the outer
	// sort.IsSorted( plus the inner sort.IntSlice( on the left, the )) on
	// the right; a comment there would be silently destroyed — advisory
	// then.
	if ps2111CommentIn(f, call.Pos(), x.Pos()) || ps2111CommentIn(f, x.End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + slicesName + ".IsSorted(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: x.Pos(), NewText: []byte(slicesName + ".IsSorted(")},
			{Pos: x.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}
