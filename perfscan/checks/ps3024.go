package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3024 reports sort.SliceIsSorted — the reflection-based sortedness
// predicate — and rewrites the whole-element ASCENDING closure shape to the
// generic slices.IsSorted. The sibling of PS3002 (sort.Slice closure →
// slices.Sort/SortFunc) on the predicate side, and of PS3008
// (Ints/StringsAreSorted → slices.IsSorted) on the closure side.
var PS3024 = register(&lint.Check{
	ID:       "PS3024",
	Category: "indirect",
	Slug:     "sliceissorted-closure-to-slices-issorted",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "sort.SliceIsSorted with an ascending whole-element closure is slices.IsSorted spelled through reflection",
		Text: `sort.SliceIsSorted boxes the slice into an "any", reads its length
through reflection (internal/reflectlite), and then calls the less closure
through a function-value indirection on EVERY adjacent pair. The generic
slices.IsSorted (Go 1.21) is monomorphized for the concrete element type:
one direct len(x), and the per-pair comparison inlines to a machine compare
with no closure call — the exact win class of PS3002 (sort.Slice closure)
and PS3008 (IntsAreSorted), of which SliceIsSorted is the missing
reflection-based sibling.

The automatic fix handles the ONE provably identical shape: the sorted
value is a SIDE-EFFECT-FREE PATH xs — a plain identifier or a selector
chain of identifiers (h.ids; field access through a pointer included, Go
auto-derefs a selector; the same precondition as PS3002) — of slice type,
and the predicate is exactly

	func(i, j int) bool { return xs[i] < xs[j] }

indexing the very path under test, i on the left, j on the right, over an
ordered NON-FLOAT element type (integers, strings — named types with such
an underlying type included, since cmp.Ordered is a ~tilde constraint and
cmp.Less compares the underlying values with the identical <).

The result is BIT-IDENTICAL. Both spellings run the same loop: sort's

	for i := n - 1; i > 0; i-- { if less(i, i-1) { return false } }

with less(i, i-1) = xs[i] < xs[i-1], against slices.IsSorted's

	for i := len(x) - 1; i > 0; i-- { if cmp.Less(x[i], x[i-1]) { return false } }

cmp.Less differs from < ONLY on NaN (cmp.Less(NaN, y) is true where
NaN < y is false), so excluding float elements — the same NaN line
PS3002/PS3008/PS3104/PS3105 draw — makes the two predicates decide false at
exactly the same first out-of-order pair and true otherwise, len < 2 (nil
included) returning true on both sides. No element ever moves, so every
tie/stability hazard of the sort-family REORDERING fixes is structurally
absent. The path argument is evaluated exactly once in BOTH spellings (as
the call argument); the rewrite only drops the closure's per-pair
re-evaluations of xs, which for a pure path yield the identical slice
header every time.

Everything else stays advisory (reported without a fix): a DESCENDING
closure (xs[i] > xs[j] — that is slices.IsSortedFunc territory), swapped
operands (xs[j] < xs[i]), a key-extraction closure (xs[i].f < xs[j].f), a
non-strict <= (a broken less contract with DIFFERENT results on ties —
never rewritable), float elements, a closure indexing a different slice
than the one under test, a call/index/deref target (re-evaluation could
have side effects), an untyped nil argument (compiles for sort, not for
the generic), a non-slice argument (sort.SliceIsSorted panics at runtime;
the generic would not compile), extra statements in the closure body, a
comment inside the deleted closure text, and a dot-/blank-imported or
shadowed slices.

The fix edits imports as needed: the slices import is added when missing
(reusing an existing alias when the file imports slices under another
name), and the sort import is dropped when the rewrites remove the file's
LAST sort reference — when they replace the whole import, the sort spec is
swapped for "slices" in place. A cgo file (whose import block must not be
edited) keeps every report advisory when import surgery would be needed.
The report only fires when the effective language version is at least
go1.21, where slices.IsSorted exists — the same gate as PS3104/PS3008.`,
		Before: `sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] < xs[j] })`,
		After:  `slices.IsSorted(xs)`,
		MeasuredWin: `BenchmarkPS3024 (a sorted 10k-element []int scanned per op —
worst case, the full pass — Apple M2 Pro, go1.26): 19.4 µs/op -> 3.0
µs/op, 0 allocs either way (~6.5x). The delta is the reflectlite length
read plus the indirect closure call per adjacent pair, which the
monomorphized generic replaces with an inlined machine compare.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3024",
		Doc:  "sort.SliceIsSorted with a whole-element ascending closure instead of the generic slices.IsSorted",
		Run:  runPS3024,
	},
})

func runPS3024(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.IsSorted arrived with the slices package in go1.21,
			// the same gate PS3104/PS3008 use; below that the advice is
			// moot, so stay silent entirely.
			continue
		}
		// Collect first, decide import edits once per file: every fixable
		// call removes one sort reference (its selector's package
		// qualifier — the closure body never references sort), and whether
		// the sort import is orphaned depends on ALL of them together
		// (same per-file site collection as PS3104/PS3008/PS3002).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package function
			// sort.SliceIsSorted: a shadowed sort resolves the qualifier
			// to some other object and never matches.
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "sort", ps3024Target); !ok {
				return true
			}
			fix := ps3024Fix(pass, f, call)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		needImport := fixable > 0 && !ps3002FileImports(f, "slices")
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
			// PS3104/PS2110/PS3008).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "sort.SliceIsSorted reads the slice through reflection and calls its less closure indirectly per adjacent pair; the generic slices.IsSorted/IsSortedFunc scans the concrete type directly",
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

var ps3024Target = map[string]bool{"SliceIsSorted": true}

// ps3024Fix builds the slices.IsSorted(xs) rewrite for the one provably
// bit-identical shape
//
//	sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] < xs[j] })
//
// where xs is a SIDE-EFFECT-FREE PATH of slice type (pathExpr — PS3002's
// precondition) and the closure body is the single ascending whole-element
// comparison over the SAME path (same root object, same field sequence —
// ps3002SamePath via orderingCompare) with an ordered non-float element
// type. Everything else returns nil and the report stays advisory. Only the
// call punctuation around the first argument is replaced; the argument
// expression stays untouched in place, preserving its text and single
// evaluation (same technique as PS3008/PS3104). Both spellings are primary
// expressions, so the swap needs no parenthesization in any parent. Import
// edits are appended later, once per file.
func ps3024Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr) *analysis.SuggestedFix {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil
	}
	arg := call.Args[0]
	// The target must be a pure ident/selector path: a call, an index, a
	// dereference or parens could re-evaluate with side effects or yield a
	// different slice inside the closure — advisory. This also rejects an
	// untyped nil argument at the type check below (nil boxes fine into
	// sort's any parameter, but the generic slices.IsSorted(nil) cannot
	// infer its type parameters and would not compile).
	xsRoot, xsFields, ok := pathExpr(arg)
	if !ok {
		return nil
	}
	xsObj := pass.TypesInfo.ObjectOf(xsRoot)
	if xsObj == nil {
		return nil
	}
	target := ps3002Path{obj: xsObj, fields: xsFields}
	// sort.SliceIsSorted takes any and panics at runtime on a non-slice;
	// the generic rewrite must see an actual slice type to compile (named
	// slice types included — slices.IsSorted infers through ~[]E).
	if _, ok := underlyingSlice(pass.TypesInfo.TypeOf(arg)); !ok {
		return nil
	}
	fl, ok := call.Args[1].(*ast.FuncLit)
	if !ok {
		return nil
	}
	// Exactly two named int parameters and a single (bool) result — the
	// same closure vetting as PS3002.
	var params []string
	for _, field := range fl.Type.Params.List {
		if !types.Identical(pass.TypesInfo.TypeOf(field.Type), types.Typ[types.Int]) {
			return nil
		}
		for _, pn := range field.Names {
			params = append(params, pn.Name)
		}
	}
	if len(params) != 2 || params[0] == "_" || params[1] == "_" {
		return nil
	}
	if fl.Type.Results == nil || fl.Type.Results.NumFields() != 1 {
		return nil
	}
	// Body: the single statement `return xs[i] < xs[j]` — whole element
	// (empty selector suffix), ASCENDING (token.LSS), i left, j right, over
	// an ordered non-float element (orderingCompare/comparisonSuffix
	// enforce all of it, floats excluded for the NaN divergence of
	// cmp.Less). A descending or field-keyed closure is slices.IsSortedFunc
	// territory and stays advisory.
	if len(fl.Body.List) != 1 {
		return nil
	}
	ret, ok := fl.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil
	}
	suffix, descending, ok := orderingCompare(pass, ret.Results[0], target, params[0], params[1])
	if !ok || suffix != "" || descending {
		return nil
	}
	slicesName, _, usable := ps3104SlicesName(pass, f, call.Pos())
	if !usable {
		return nil
	}
	// The replaced spans are the call text around the first argument —
	// including the whole closure; a comment there would be silently
	// destroyed, so the report stays advisory then.
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
