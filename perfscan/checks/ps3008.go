package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3008 reports sort.IntsAreSorted(x) / sort.StringsAreSorted(x) — the
// legacy concrete sortedness predicates — and rewrites them to the generic
// slices.IsSorted(x).
var PS3008 = register(&lint.Check{
	ID:       "PS3008",
	Category: "indirect",
	Slug:     "aresorted-helper-to-slices-issorted",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "sort.IntsAreSorted/sort.StringsAreSorted are the legacy spelling of slices.IsSorted; call the generic predicate directly",
		Text: `On a go1.21 toolchain sort.IntsAreSorted and sort.StringsAreSorted
wrap the slice in sort.IntSlice / sort.StringSlice and scan it through
sort.IsSorted(sort.Interface) — an interface Len() call plus a Less(i, j)
dispatch per adjacent pair, each comparison paying a virtual call and the
bounds checks of the adapter's indexing. The generic slices.IsSorted
(Go 1.21) is monomorphized for the concrete element type: the comparison
inlines to a machine compare with no interface hop. Since go1.22 the
standard library has conceded the point: sort.IntsAreSorted and
sort.StringsAreSorted ARE one-line wrappers calling slices.IsSorted, so
there the rewrite is performance parity minus one call hop, and the win is
directness — calling the modern API instead of its legacy shim (the exact
sibling of PS3104's sort.Ints → slices.Sort story).

The result is BIT-IDENTICAL. The predicate is a pure bool — no elements
move, so every tie/stability hazard that blocks the sort-family float
REORDERING fixes is inapplicable. Both spellings answer "is no adjacent
pair descending under <": on go1.21 sort.IntSlice.Less / sort.StringSlice.Less
use the identical < the generic cmp.Less compiles to for int and string,
and from go1.22 on both spellings are literally the same function. int and
string carry no distinguishable-equal-value subtleties (no NaN, no ±0.0),
so there is nothing observable to diverge. Both return true for len < 2,
nil included. The argument expression is kept verbatim in place as the sole
argument — single evaluation preserved, nothing duplicated.

sort.Float64sAreSorted is deliberately NOT flagged: it is provably
bool-identical too (no reordering happens), but perfscan's sort family
keeps a bright conservative line — no float rewrites — so it is deferred
rather than special-cased.

The callee is resolved with type information — only the package functions
sort.IntsAreSorted/sort.StringsAreSorted match, never a shadowed sort, a
same-named local, or a method — and the call is rewritten wherever the bool
is consumed (condition, assignment, return, argument). The fix edits
imports as needed: the slices import is added when missing (reusing an
existing alias when the file imports slices under another name), and the
sort import is dropped when the rewrites remove the file's LAST sort
reference — when they replace the whole import, the sort spec is swapped
for "slices" in place. Two shapes stay advisory (reported without a fix):
an untyped nil argument (sort.IntsAreSorted(nil) compiles, but the generic
slices.IsSorted(nil) cannot infer its type parameters), and a comment
inside the rewritten call punctuation; a dot- or blank-imported slices or a
cgo file (whose import block must not be edited) withholds the fix too.

The report only fires when the effective language version is at least
go1.21 — below that slices.IsSorted does not exist and the advice would be
moot. Note that per-file version pinning itself only exists from go1.21 on,
so go/types clamps a //go:build go1.20 (or older) line to go1.21 and such
files are still flagged; in practice the gate blocks via the module's go
directive (pass.Pkg.GoVersion()) when a whole module still declares
go < 1.21.`,
		Before: `sort.IntsAreSorted(xs)`,
		After:  `slices.IsSorted(xs)`,
		MeasuredWin: `BenchmarkPS3008 (a sorted 10k-element []int scanned per op —
worst case, the full pass — Apple M2 Pro, go1.26): parity — 2.9 µs/op
before vs 2.9 µs/op after, 0 allocs either way. Expected: since go1.22
sort.IntsAreSorted IS a one-line wrapper over slices.IsSorted, so the
modern toolchain pair measures the same code. The performance win is real
only on a go1.21 toolchain, where the predicate still paid an interface
dispatch per adjacent pair; from go1.22 on the win is directness — the
modern API without the legacy hop.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3008",
		Doc:  "sort.IntsAreSorted/sort.StringsAreSorted legacy predicates instead of the generic slices.IsSorted",
		Run:  runPS3008,
	},
})

// ps3008Targets maps the matched sort predicate to its element spelling (for
// the message). sort.Float64sAreSorted is deliberately absent: it would be
// bool-identical too (nothing is reordered), but the sort family keeps a
// bright no-float line, so it stays out.
var ps3008Targets = map[string]string{
	"IntsAreSorted":    "[]int",
	"StringsAreSorted": "[]string",
}

func runPS3008(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.IsSorted arrived with the slices package in go1.21,
			// the same gate PS3104 uses for slices.Sort; below that the
			// advice is moot, so stay silent entirely.
			continue
		}
		// Collect first, decide import edits once per file: every fixable
		// call removes one sort reference, and whether the sort import is
		// orphaned depends on ALL of them together (same per-file site
		// collection as PS3104/PS3002).
		type site struct {
			call *ast.CallExpr
			name string
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
			// Type info pins the callee to the package function
			// sort.IntsAreSorted or sort.StringsAreSorted: a shadowed sort
			// resolves sel.Sel to some other object, and a method carries
			// a receiver. The name allowlist keeps sort.Float64sAreSorted
			// (family no-float policy) and everything else out.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "sort" {
				return true
			}
			if _, wanted := ps3008Targets[fn.Name()]; !wanted {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			fix := ps3008Fix(pass, f, call, call.Args[0])
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fn.Name(), fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call holds exactly one sort reference (its
		// selector's package identifier); when those are ALL of the file's
		// sort references, the rewrites orphan the import and the fix must
		// drop it (the runner never prunes imports itself). The slices
		// import is needed when the FILE lacks one — a site-local shadow
		// only suppresses that site's fix, never the file's import edit.
		slicesImported := false
		for _, imp := range f.Imports {
			if imp.Path.Value == `"slices"` {
				slicesImported = true
				break
			}
		}
		needImport := fixable > 0 && !slicesImported
		orphansSort := fixable > 0 && pkgRefCount(pass, f, "sort") == fixable
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
			// PS3104/PS2110).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			elem := ps3008Targets[st.name]
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "sort." + st.name + " is the legacy spelling of slices.IsSorted (an interface-dispatch scan on go1.21, a one-line wrapper since go1.22); slices.IsSorted checks the concrete " + elem + " directly with the identical boolean result",
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3008Fix builds the slices.IsSorted(arg) rewrite for one call, or nil when
// a guard fails and the report must stay advisory. Only the punctuation
// around the argument is replaced; the argument expression stays untouched in
// place, preserving its text and single evaluation (same technique as
// PS3104). Both spellings are primary expressions, so the swap needs no
// parenthesization in any parent. Import edits are appended later, once per
// file.
func ps3008Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, arg ast.Expr) *analysis.SuggestedFix {
	// sort.IntsAreSorted(nil) compiles (nil converts to the []int
	// parameter), but the generic slices.IsSorted(nil) cannot infer its
	// type parameters and would not compile — advisory.
	if tv, ok := pass.TypesInfo.Types[arg]; !ok || tv.IsNil() {
		return nil
	}
	slicesName, _, usable := ps3104SlicesName(pass, f, call.Pos())
	if !usable {
		return nil
	}
	// The replaced spans are the call text around the argument; a comment
	// there would be silently destroyed — advisory then.
	if ps2111CommentIn(f, call.Pos(), arg.Pos()) || ps2111CommentIn(f, arg.End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + slicesName + ".IsSorted(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: arg.Pos(), NewText: []byte(slicesName + ".IsSorted(")},
			{Pos: arg.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}
