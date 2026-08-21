package checks

import (
	"go/ast"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3105 reports sort.Sort / sort.Stable over a sort.IntSlice / sort.StringSlice
// adapter — the wrapper-adapter spelling of an ascending sort — and rewrites
// them to the generic slices.Sort(x). Sibling of PS3104, which handles the
// sort.Ints/sort.Strings helper spelling of the same operation.
var PS3105 = register(&lint.Check{
	ID:       "PS3105",
	Category: "indirect",
	Slug:     "sortsort-slice-to-slices-sort",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "sort.Sort/sort.Stable over a sort.IntSlice/StringSlice adapter is slices.Sort spelled the long way; call the generic sort directly",
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

The stable spelling is matched too. sort.Stable(sort.IntSlice(x)) and
sort.Stable(sort.StringSlice(x)) run a STABLE sort through the same
adapter, and they rewrite to the very same slices.Sort(x) — which is
UNSTABLE — because stability is UNOBSERVABLE for these element types. A
stable sort differs from an unstable one only in the relative order it
gives to elements that compare EQUAL; but for []int and []string, any
two elements that compare equal are bitwise-identical values, so no
observer can tell which equal element came first. slices.Sort(x)
therefore produces byte-for-byte the same slice as sort.Stable over the
adapter — bit-identical. (The slices package has no comparator-free
SortStable, and none is needed here: there is no observable stability to
preserve. If a future element type had distinguishable ties this
equivalence would fail, which is exactly why sort.Float64Slice is
excluded below for BOTH sort.Sort and sort.Stable.)

The result is BIT-IDENTICAL. Both spellings sort ascending (< on int;
lexicographic byte order on string), and for int and string any two
elements that compare equal are bitwise-identical values — so the sorted
output is the unique ascending arrangement of the input multiset, the
same bytes no matter which algorithm produced it, stability included.
sort.Sort and sort.Stable return nothing, so only a call statement is
rewritten.

The DESCENDING idiom through sort.Reverse is matched and fixed too:

	sort.Sort(sort.Reverse(sort.IntSlice(x)))

(and the StringSlice and sort.Stable spellings) rewrites to

	slices.SortFunc(x, func(a, b int) int { return cmp.Compare(b, a) })

This is likewise BIT-IDENTICAL. sort.Reverse(sort.IntSlice(x)) has
Less(i, j) = IntSlice.Less(j, i) = x[j] < x[i] — descending.
slices.SortFunc with cmp.Compare(b, a) orders a before b iff
cmp.Compare(b, a) < 0 iff b < a iff a > b — the identical descending
predicate. Both are unstable pdqsort, and for []int/[]string any two
equal elements are bitwise-identical, so the descending arrangement of
the input multiset is unique and byte-for-byte the same. The
sort.Stable(sort.Reverse(...)) spelling maps to the same UNSTABLE
slices.SortFunc for the same reason the ascending sort.Stable does:
stability is unobservable when every tie is bitwise-identical. Only the
fresh sort.Reverse CALL wrapping a fresh conversion is matched — a
pre-built sort.Interface value passed to sort.Reverse is opaque. The
descending fix additionally needs the cmp package usable by name at the
call site (an existing import alias is reused, the import added when
missing); a dot- or blank-imported or shadowed cmp keeps the report
advisory.

sort.Float64Slice is deliberately EXCLUDED and never flagged — under
either sort.Sort or sort.Stable, and equally under sort.Reverse — for
the same reason PS3104 excludes
sort.Float64s: Float64Slice.Less orders NaNs first, and float64 has
DISTINGUISHABLE ties (-0.0 and +0.0 compare equal but differ in bits, as
do NaNs with different payloads). For sort.Sort(sort.Float64Slice(x))
both sorts are unstable, so two correct implementations may arrange
those ties differently. For sort.Stable(sort.Float64Slice(x)) the source
is stable but slices.Sort is not, so the distinguishable ties could be
reordered. Either way the uniqueness argument above collapses and the
rewrite is not guaranteed bit-identical, so perfscan does not offer it —
and sort.Sort(sort.Reverse(sort.Float64Slice(x))) inherits the exact
same collapse (NaN ordering plus distinguishable float ties), so the
Reverse form of Float64Slice is excluded too.

Both selectors are resolved with type information: the callee must be
the package function sort.Sort or sort.Stable — never a shadowed sort, a
same-named local, or a method — and its single argument must be a fresh
CONVERSION to the named type sort.IntSlice or sort.StringSlice. A
pre-built value of those types (a variable, a function result) is never
matched: only the direct conversion form guarantees the operand is the
underlying []int/[]string, kept verbatim in place by the fix. An untyped
operand (sort.Sort(sort.IntSlice(nil))) is skipped — slices.Sort cannot
infer its type parameters from nil. Only a plain call statement is
rewritten.

The fix edits imports as needed: the slices import is added when missing
(reusing an existing alias when the file imports slices under another
name), the cmp import is added when a descending rewrite needs it, and
the sort import is dropped when the rewrites remove the file's LAST sort
references — each rewritten ascending call gives up TWO of them (the
sort.Sort/sort.Stable callee and the sort.IntSlice/StringSlice
conversion), each rewritten descending call THREE (those two plus
sort.Reverse), and the import is dropped only when the file holds none
besides those. When the
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
the modern API. The sort.Stable spelling mirrors these numbers (it drives
the SAME adapter through the same interface, only via a stable pdqsort
variant) and rewrites to the same unstable slices.Sort.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3105",
		Doc:  "sort.Sort/sort.Stable over a sort.IntSlice/StringSlice adapter instead of the generic slices.Sort",
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

// ps3105Elems spells the element type the descending fix's comparator
// closure declares: func(a, b ELEM) int.
var ps3105Elems = map[string]string{
	"IntSlice":    "int",
	"StringSlice": "string",
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
		// ascending call removes TWO sort references (sort.Sort and the
		// inner conversion), every fixable descending call THREE (plus
		// sort.Reverse), and whether the sort import is orphaned depends on
		// ALL of them together (same per-file site collection as PS3104).
		type site struct {
			call  *ast.CallExpr
			outer string // the outer sort.* callee: "Sort" or "Stable"
			name  string // the inner adapter type: "IntSlice" or "StringSlice"
			desc  bool   // the sort.Reverse descending form
			fix   *analysis.SuggestedFix
		}
		var sites []site
		ascFixable, descFixable := 0, 0
		needCmp := false
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
			// Type info pins the callee to the receiver-less package
			// functions sort.Sort or sort.Stable: a shadowed sort resolves
			// sel.Sel to some other object, and a method carries a
			// receiver. Both spellings drive the sort.Interface adapter,
			// and for the []int/[]string element types below the stable
			// arrangement is unobservable (equal elements are bitwise
			// identical), so both map to slices.Sort.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "sort" || (fn.Name() != "Sort" && fn.Name() != "Stable") {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// The single argument is either a fresh adapter CONVERSION
			// (ascending) or a fresh sort.Reverse CALL wrapping one
			// (descending). Anything else — a pre-built adapter value, a
			// custom sort.Interface, sort.Float64Slice — is never matched.
			name, x, matched := ps3105Adapter(pass, call.Args[0])
			desc := false
			if !matched {
				rev, isCall := call.Args[0].(*ast.CallExpr)
				if !isCall || len(rev.Args) != 1 || rev.Ellipsis.IsValid() {
					return true
				}
				rsel, isSel := ps2110Unparen(rev.Fun).(*ast.SelectorExpr)
				if !isSel {
					return true
				}
				// Same type-checked pinning as the outer callee: the
				// receiver-less package function sort.Reverse, never a
				// shadowed sort or a same-named method.
				rfn, isFn := pass.TypesInfo.Uses[rsel.Sel].(*types.Func)
				if !isFn || rfn.Pkg() == nil || rfn.Pkg().Path() != "sort" || rfn.Name() != "Reverse" {
					return true
				}
				if sig, isSig := rfn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
					return true
				}
				name, x, matched = ps3105Adapter(pass, rev.Args[0])
				if !matched {
					return true
				}
				desc = true
			}
			var fix *analysis.SuggestedFix
			if desc {
				var cmpAdded bool
				fix, cmpAdded = ps3105ReverseFix(pass, f, call, x, ps3105Elems[name])
				if fix != nil {
					descFixable++
					needCmp = needCmp || cmpAdded
				}
			} else {
				fix = ps3105Fix(pass, f, call, x)
				if fix != nil {
					ascFixable++
				}
			}
			sites = append(sites, site{call, fn.Name(), name, desc, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable ascending call holds exactly TWO sort references —
		// the sort.Sort selector's package identifier and the inner
		// sort.IntSlice/StringSlice conversion's — and each fixable
		// descending call THREE (those two plus sort.Reverse), so the
		// rewrites orphan the import only when the file's sort reference
		// count equals exactly the sum consumed across all fixable sites
		// (the operand kept verbatim can itself mention sort, in which
		// case the count is higher and the import stays). The slices
		// import is needed when the FILE lacks one — a site-local shadow
		// only suppresses that site's fix, never the file's import edit —
		// and the cmp import when a descending fix resolved it as missing.
		fixable := ascFixable + descFixable
		slicesImported := false
		for _, imp := range f.Imports {
			if imp.Path.Value == `"slices"` {
				slicesImported = true
				break
			}
		}
		needImport := fixable > 0 && !slicesImported
		consumed := 2*ascFixable + 3*descFixable
		orphansSort := fixable > 0 && pkgRefCount(pass, f, "sort") == consumed
		importEdits, importsOK := ps3105ImportEdits(f, needImport, needCmp, orphansSort)
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
			msg := "sort." + st.outer + "(sort." + st.name + "(...)) sorts through the sort.Interface adapter (an interface dispatch per comparison and swap); slices.Sort sorts the concrete " + elem + " directly with the identical ascending order"
			if st.desc {
				msg = "sort." + st.outer + "(sort.Reverse(sort." + st.name + "(...))) sorts descending through the sort.Interface adapter (an interface dispatch per comparison and swap); slices.SortFunc with cmp.Compare(b, a) sorts the concrete " + elem + " directly with the identical descending order"
			}
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: msg,
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
	// sort.Sort(/sort.Stable( plus the inner sort.IntSlice( on the left,
	// the )) on the right; a comment there would be silently destroyed —
	// advisory then.
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

// ps3105Adapter matches a fresh CONVERSION to sort.IntSlice or
// sort.StringSlice — a CallExpr whose Fun denotes the named type, resolved
// through type info — and returns the adapter name plus the conversion
// operand. A pre-built value of those types is not matched: only the
// conversion form hands us the underlying []int/[]string operand to keep
// verbatim. The allowlist keeps sort.Float64Slice (NaN ordering, float
// ties) and every custom sort.Interface implementation out. The operand
// must be a typed value assignable to the plain []int/[]string that
// slices.Sort / slices.SortFunc will receive; an untyped nil is excluded —
// sort.Sort(sort.IntSlice(nil)) is legal, but slices.Sort(nil) cannot
// infer its type parameters and would not compile.
func ps3105Adapter(pass *analysis.Pass, arg ast.Expr) (name string, x ast.Expr, ok bool) {
	inner, isCall := arg.(*ast.CallExpr)
	if !isCall || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
		return "", nil, false
	}
	isel, isSel := ps2110Unparen(inner.Fun).(*ast.SelectorExpr)
	if !isSel {
		return "", nil, false
	}
	tn, isType := pass.TypesInfo.Uses[isel.Sel].(*types.TypeName)
	if !isType || tn.Pkg() == nil || tn.Pkg().Path() != "sort" {
		return "", nil, false
	}
	if _, wanted := ps3105Targets[tn.Name()]; !wanted {
		return "", nil, false
	}
	if tv, has := pass.TypesInfo.Types[inner.Fun]; !has || !tv.IsType() {
		return "", nil, false
	}
	x = inner.Args[0]
	xt := pass.TypesInfo.TypeOf(x)
	if xt == nil {
		return "", nil, false
	}
	if b, isBasic := xt.(*types.Basic); isBasic && b.Info()&types.IsUntyped != 0 {
		return "", nil, false
	}
	if !types.AssignableTo(xt, ps3105SliceTypes[tn.Name()]) {
		return "", nil, false
	}
	return tn.Name(), x, true
}

// ps3105ReverseFix builds the descending rewrite for one
// sort.Sort(sort.Reverse(sort.IntSlice(x))) call:
//
//	slices.SortFunc(x, func(a, b ELEM) int { return cmp.Compare(b, a) })
//
// or nil when a guard fails and the report must stay advisory. The operand
// expression stays untouched in place — only the punctuation and wrappers
// around it are replaced — preserving its text and single evaluation (same
// technique as the ascending ps3105Fix). cmpAdded reports that the fix
// spelled cmp as a NEW import the file must add (an existing import alias
// is reused instead when present); the import edit itself is appended
// later, once per file. A dot- or blank-imported or shadowed cmp (or
// slices) yields nil.
func ps3105ReverseFix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, x ast.Expr, elem string) (fix *analysis.SuggestedFix, cmpAdded bool) {
	slicesName, _, usable := ps3104SlicesName(pass, f, call.Pos())
	if !usable {
		return nil, false
	}
	cmpName, cmpNeedImport, cmpUsable := ps3002CmpName(pass, f, call.Pos())
	if !cmpUsable {
		return nil, false
	}
	// The replaced spans are the call text around the operand — the outer
	// sort.Sort(sort.Reverse(sort.IntSlice( on the left, the ))) on the
	// right; a comment there would be silently destroyed — advisory then.
	if ps2111CommentIn(f, call.Pos(), x.Pos()) || ps2111CommentIn(f, x.End(), call.End()) {
		return nil, false
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + slicesName + ".SortFunc(..., " + cmpName + ".Compare(b, a))",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: x.Pos(), NewText: []byte(slicesName + ".SortFunc(")},
			{Pos: x.End(), End: call.End(), NewText: []byte(", func(a, b " + elem + ") int { return " + cmpName + ".Compare(b, a) })")},
		},
	}, cmpNeedImport
}

// ps3105ImportEdits composes the file's combined import edits: the
// slices-add/sort-drop pair PS3104's helper already builds, plus the cmp
// import a descending fix needs. When the cmp insertion lands at the very
// position of a zero-width slices insertion (e.g. both belong before the
// same spec), the two are merged into ONE edit — cmp first, then slices,
// which is their gofmt order — so the applied fix never depends on how
// same-position edits happen to be ordered. ok is false when the import
// block must not or cannot be edited (cgo file, missing sort spec).
func ps3105ImportEdits(f *ast.File, needSlices, needCmp, dropSort bool) (edits []analysis.TextEdit, ok bool) {
	edits, ok = ps3104ImportEdits(f, needSlices, dropSort)
	if !ok || !needCmp {
		return edits, ok
	}
	if ps2110ImportsC(f) {
		// ps3104ImportEdits only vetoes cgo when it has edits of its own;
		// a cmp-only addition must respect the same rule.
		return nil, false
	}
	cmpEdit := ps2110ImportEdit(f, "cmp")
	for i := range edits {
		if edits[i].Pos == cmpEdit.Pos && edits[i].End == edits[i].Pos && cmpEdit.End == cmpEdit.Pos {
			edits[i].NewText = slices.Concat(cmpEdit.NewText, edits[i].NewText)
			return edits, true
		}
	}
	return append([]analysis.TextEdit{cmpEdit}, edits...), true
}
