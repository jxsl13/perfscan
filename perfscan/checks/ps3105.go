package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3105 reports sort.Sort(sort.IntSlice(x)) / sort.Sort(sort.StringSlice(x))
// — the wrapper-adapter spelling of an ascending sort — and rewrites them to
// the generic slices.Sort(x). Sibling of PS3104, which handles the
// sort.Ints/sort.Strings helper spelling of the same operation.
var PS3105 = register(&lint.Check{
	ID:       "PS3105",
	Category: "indirect",
	Slug:     "sortsort-slice-to-slices-sort",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "sort.Sort(sort.IntSlice/StringSlice(x)) is the adapter spelling of slices.Sort; call the generic sort directly",
		Text: `sort.Sort(sort.IntSlice(x)) wraps the slice in the sort.IntSlice
adapter and sorts it through the three-method sort.Interface — an
interface dispatch on every comparison and swap — while the generic
slices.Sort (Go 1.21) is a pdqsort specialized for the concrete element
type at compile time. Unlike sort.Ints (which became a one-line wrapper
over slices.Sort in go1.22, see PS3104), the sort.Sort spelling still
pays the sort.Interface indirection on EVERY toolchain: sort.Sort takes
a sort.Interface and dispatches Less/Swap through it per operation, so
here the rewrite is not merely directness — it measures a real win on
modern toolchains too (plus one small allocation saved: boxing the
adapter into the interface). This check is PS3104's sibling: PS3104
matches the helper spelling sort.Ints(x)/sort.Strings(x); PS3105 matches
the equivalent adapter spelling sort.Sort(sort.IntSlice(x)) /
sort.Sort(sort.StringSlice(x)) that PS3104 does not.

The result is BIT-IDENTICAL. Both spellings sort ascending (< on int;
lexicographic byte order on string), and for int and string any two
elements that compare equal are bitwise-identical values — so the sorted
output is the unique ascending arrangement of the input multiset, the
same bytes no matter which algorithm produced it, stability included.
sort.Sort returns nothing, so only a call statement is rewritten.

sort.Float64Slice is deliberately EXCLUDED and never flagged, for the
same reason PS3104 excludes sort.Float64s: Float64Slice.Less orders NaNs
first, and float64 has DISTINGUISHABLE ties (-0.0 and +0.0 compare equal
but differ in bits, as do NaNs with different payloads) — and both sorts
are unstable, so two correct implementations may arrange those ties
differently. The uniqueness argument above collapses and the rewrite is
not guaranteed bit-identical, so perfscan does not offer it.

Both selectors are resolved with type information: the callee must be
the package function sort.Sort — never a shadowed sort, a same-named
local, or a method — and its single argument must be a fresh CONVERSION
to the named type sort.IntSlice or sort.StringSlice. A pre-built value
of those types (a variable, a function result) is never matched: only
the direct conversion form guarantees the operand is the underlying
[]int/[]string, kept verbatim in place by the fix. An untyped operand
(sort.Sort(sort.IntSlice(nil))) is skipped — slices.Sort cannot infer
its type parameters from nil. Only a plain call statement is rewritten.

The fix edits imports as needed: the slices import is added when missing
(reusing an existing alias when the file imports slices under another
name), and the sort import is dropped when the rewrites remove the
file's LAST sort references — each rewritten call gives up TWO of them,
sort.Sort and the sort.IntSlice/StringSlice conversion, and the import
is dropped only when the file holds none besides those. When the
rewrites replace the whole import, the sort spec is swapped for "slices"
in place. A dot- or blank-imported slices, a comment inside the
rewritten call punctuation, or a cgo file (whose import block must not
be edited) keeps the report advisory.

The report only fires when the effective language version is at least
go1.21 — below that slices.Sort does not exist and the advice would be
moot. Note that per-file version pinning itself only exists from go1.21
on, so go/types clamps a //go:build go1.20 (or older) line to go1.21 and
such files are still flagged; in practice the gate blocks via the
module's go directive (pass.Pkg.GoVersion()) when a whole module still
declares go < 1.21.`,
		Before: `sort.Sort(sort.IntSlice(xs))`,
		After:  `slices.Sort(xs)`,
		MeasuredWin: `BenchmarkPS3105 (a shuffled 10k-element []int copied and
sorted per op, Apple M2 Pro, go1.26): 678 µs/op before vs 389 µs/op
after — about 1.7x — and 1 alloc/op before (boxing the adapter into
sort.Interface, 24 B) vs 0 after. Unlike PS3104's helpers, sort.Sort
never became a wrapper over slices.Sort: it still drives Less and Swap
through the interface on every comparison and swap on every toolchain,
so the dispatch cost is real on go1.26, not just on go1.21. The rewrite
buys the same ascending order, measurably faster, plus the directness of
the modern API.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3105",
		Doc:  "sort.Sort(sort.IntSlice/StringSlice(x)) adapter spelling instead of the generic slices.Sort",
		Run:  runPS3105,
	},
})

// ps3105Targets maps the matched sort adapter type to its element spelling
// (for the message). sort.Float64Slice is deliberately absent: float64 has
// distinguishable ties (-0.0/+0.0, NaN payloads) that an unstable sort may
// arrange differently — and Float64Slice.Less additionally orders NaNs
// first — so that rewrite is not guaranteed bit-identical.
var ps3105Targets = map[string]string{
	"IntSlice":    "[]int",
	"StringSlice": "[]string",
}

// ps3105SliceTypes gives the plain slice type the conversion operand must be
// assignable to — the type slices.Sort will sort after the rewrite.
var ps3105SliceTypes = map[string]*types.Slice{
	"IntSlice":    types.NewSlice(types.Typ[types.Int]),
	"StringSlice": types.NewSlice(types.Typ[types.String]),
}

func runPS3105(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.Sort exists only from go1.21 on; below that the
			// advice is moot, so stay silent entirely (same gate as
			// PS3104).
			continue
		}
		// Collect first, decide import edits once per file: every fixable
		// call removes TWO sort references (sort.Sort and the inner
		// conversion), and whether the sort import is orphaned depends on
		// ALL of them together (same per-file site collection as PS3104).
		type site struct {
			call *ast.CallExpr
			name string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			es, ok := n.(*ast.ExprStmt)
			if !ok {
				return true
			}
			call, ok := es.X.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package function sort.Sort:
			// a shadowed sort resolves sel.Sel to some other object, and a
			// method carries a receiver.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "sort" || fn.Name() != "Sort" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// The single argument must be a fresh CONVERSION to
			// sort.IntSlice or sort.StringSlice — a CallExpr whose Fun
			// denotes the named type. A pre-built value of those types is
			// not matched: only the conversion form hands us the
			// underlying []int/[]string operand to keep verbatim. The
			// allowlist keeps sort.Float64Slice (NaN ordering, float ties)
			// and every custom sort.Interface implementation out.
			inner, ok := call.Args[0].(*ast.CallExpr)
			if !ok || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
				return true
			}
			isel, ok := ps2110Unparen(inner.Fun).(*ast.SelectorExpr)
			if !ok {
				return true
			}
			tn, ok := pass.TypesInfo.Uses[isel.Sel].(*types.TypeName)
			if !ok || tn.Pkg() == nil || tn.Pkg().Path() != "sort" {
				return true
			}
			if _, wanted := ps3105Targets[tn.Name()]; !wanted {
				return true
			}
			if tv, has := pass.TypesInfo.Types[inner.Fun]; !has || !tv.IsType() {
				return true
			}
			// The conversion operand must be a typed value assignable to
			// the plain []int/[]string that slices.Sort will receive. An
			// untyped nil is excluded: sort.Sort(sort.IntSlice(nil)) is
			// legal, but slices.Sort(nil) cannot infer its type parameters
			// and would not compile.
			x := inner.Args[0]
			xt := pass.TypesInfo.TypeOf(x)
			if xt == nil {
				return true
			}
			if b, isBasic := xt.(*types.Basic); isBasic && b.Info()&types.IsUntyped != 0 {
				return true
			}
			if !types.AssignableTo(xt, ps3105SliceTypes[tn.Name()]) {
				return true
			}
			fix := ps3105Fix(pass, f, call, x)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, tn.Name(), fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call holds exactly TWO sort references — the
		// sort.Sort selector's package identifier and the inner
		// sort.IntSlice/StringSlice conversion's — so the rewrites orphan
		// the import only when the file's sort reference count is exactly
		// twice the fixable-site count (the operand kept verbatim can
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
			// PS3104).
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
				Message: "sort.Sort(sort." + st.name + "(...)) sorts through the sort.Interface adapter (an interface dispatch per comparison and swap); slices.Sort sorts the concrete " + elem + " directly with the identical ascending order",
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3105Fix builds the slices.Sort(x) rewrite for one call, or nil when a
// guard fails and the report must stay advisory. Only the punctuation and
// adapter conversion around the operand are replaced; the operand
// expression stays untouched in place, preserving its text and single
// evaluation (same technique as PS3104). Import edits are appended later,
// once per file.
func ps3105Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, x ast.Expr) *analysis.SuggestedFix {
	slicesName, _, usable := ps3104SlicesName(pass, f, call.Pos())
	if !usable {
		return nil
	}
	// The replaced spans are the call text around the operand — the outer
	// sort.Sort( plus the inner sort.IntSlice( on the left, the )) on the
	// right; a comment there would be silently destroyed — advisory then.
	if ps2111CommentIn(f, call.Pos(), x.Pos()) || ps2111CommentIn(f, x.End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + slicesName + ".Sort(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: x.Pos(), NewText: []byte(slicesName + ".Sort(")},
			{Pos: x.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}
