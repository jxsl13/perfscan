package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3036 reports a slices.CompareFunc whose comparator is nothing but
// strings.Compare on the two parameters in source order — slices.Compare
// spelled the slow way for string elements — and rewrites it to the direct
// form. Sibling of PS3023 (the same CompareFunc shape with a cmp.Compare
// comparator, which that matcher deliberately limits itself to) and the
// strings.Compare arm of the family PS3009 (SortFunc/SortStableFunc),
// PS3011 (BinarySearchFunc), PS3014 (IsSortedFunc) and PS3015/PS3018
// (SortedFunc/SortedStableFunc) already cover for their shapes.
var PS3036 = register(&lint.Check{
	ID:       "PS3036",
	Category: "indirect",
	Slug:     "comparefunc-strings-compare-to-slices-compare",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.CompareFunc with a bare strings.Compare comparator is slices.Compare spelled the slow way",
		Text: `slices.CompareFunc(a, b, func(x, y string) int { return strings.Compare(x, y) })
compares the two string slices lexicographically — exactly what
slices.Compare(a, b) does — but pays an indirect call through the comparator
func value on EVERY element pair, and inside that closure strings.Compare is
itself another call doing the three-way byte comparison. slices.Compare is a
distinct monomorphized entry point whose body IS the same loop with the
comparator fixed to an inlined cmp.Compare(s1[i], s2[i]): same early exit,
same ±1 length tie-breaks, zero comparator indirection. Passing
strings.Compare itself as the comparator is the same anti-pattern minus one
layer and is matched too.

The result is BYTE-IDENTICAL, and the proof is strictly simpler than
PS3023's: strings.Compare's parameters are plain string, so a matched
comparator only type-checks over elements whose type IS string (or an alias
of it) — a defined type with underlying string does not compile against it
without conversions, which the matcher never accepts — and the float/NaN
carve-outs of the cmp.Compare family cannot arise at all. On strings,
strings.Compare(x, y) returns exactly -1/0/+1 by byte-lexicographic order,
which is identically cmp.Compare(x, y) on string (its isNaN branches are dead
for strings, leaving x<y → -1, x>y → +1, else 0). Both walks visit the same
pairs left to right, return the first non-zero comparison — and because
strings.Compare only ever yields -1/0/+1, no non-canonical magnitude can leak
through CompareFunc's "return the comparator's int" path — then break length
ties with the same ±1. Both slices are read, never written, so there is no
tie-arrangement freedom to observe; comparison count and (side-effect-free)
argument evaluation are unchanged. Invalid UTF-8 is irrelevant — both sides
order by raw bytes, never runes.

The fix is withheld (report stays ADVISORY) in the same two shape corners as
PS3023. MIXED SLICE TYPES: slices.CompareFunc takes two independently-typed
slices (S1, S2), while slices.Compare takes one S for both — comparing a
[]string against a named StringSlice compiles only in the Func spelling, so
the fix requires the two slice arguments to have IDENTICAL types. EXPLICIT
INSTANTIATION: slices.CompareFunc[S1, S2, E1, E2](...) has four type
arguments where slices.Compare has two, so the brackets cannot survive the
rewrite and the site is left to the human.

The match is deliberately EXACT — the same comparator matcher as
PS3009/PS3011. The comparator must be strings.Compare itself, or a func
literal whose whole body is a single return of strings.Compare(p0, p1) with
the two parameters in SOURCE ORDER, resolved by object identity — a swapped
strings.Compare(y, x) is slices.Compare(b, a), a DIFFERENT result, and is
never matched; a conversion, a field selector, a captured outer variable or
any extra computation fails the match silently. Both strings and slices are
resolved with type information — only the stdlib packages match, never a
shadowed local or a same-named method — and an aliased import matches
naturally.

The fix replaces the CompareFunc selector with Compare and deletes the
comparator; both slice arguments are kept VERBATIM in place (single
evaluation, source order preserved) and the package qualifier keeps the
file's alias. Deleting the comparator removes the file's strings reference,
so when the rewrites remove the file's LAST strings references the fix also
drops the strings import (alias included); a cgo file (whose import block
must not be edited) or a comment inside the deleted span keeps the report
advisory. Fires only from go1.21 on (slices.Compare and slices.CompareFunc
both appeared there).`,
		Before: `if slices.CompareFunc(a, b, strings.Compare) < 0 { ... }`,
		After:  `if slices.Compare(a, b) < 0 { ... }`,
		MeasuredWin: `BenchmarkPS3036 (two 4096-element []string of 8-byte keys
sharing a 4-byte prefix, differing only in the last element — a full-length
lexicographic scan — per op, Apple M2 Pro, gc 1.26): ~30.0 µs/op before vs
~29.9 µs/op after — parity, 0 allocs either way. gc inlines
slices.CompareFunc and devirtualizes the known comparator (literal or
strings.Compare itself), and the byte comparison is a runtime cmpstring call
on BOTH sides, so both become the same scan; the win is source-level
robustness (the closure and its strings.Compare scaffolding go away, and
slices.Compare cannot fall off the devirtualization path a hoisted or grown
callback can), plus a real per-pair indirect call removed on toolchains
without closure devirtualization (gccgo, tinygo, older gc) — the same class
as PS1008/PS3110/PS3111/PS3023.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3036",
		Doc:  "slices.CompareFunc with a bare strings.Compare comparator instead of slices.Compare",
		Run:  runPS3036,
	},
})

func runPS3036(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			continue // slices.Compare exists only from go1.21 on (same gate as PS3107/PS3023).
		}
		// Collect first, decide the strings-import edit once per file: every
		// fixable call deletes exactly ONE strings reference (the
		// comparator's strings.Compare selector), and whether the strings
		// import is orphaned depends on ALL of them together (same per-file
		// collection as PS3009/PS3011).
		type site struct {
			call *ast.CallExpr
			elem string // rendered element type, for the message
			why  string // non-empty: advisory-by-design reason suffix
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the receiver-less package function
			// slices.CompareFunc, resolved through type info — never a
			// shadowed slices or a same-named method.
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "CompareFunc")
			if !ok {
				return true
			}
			// The comparator must BE strings.Compare on the parameters in
			// source order (reuses PS3009's exact matcher). Swapped operands
			// would be slices.Compare(b, a) — never matched.
			if !ps3009BareCompare(pass, call.Args[2]) {
				return true
			}
			elem, fixableSite, why := ps3036Site(pass, call)
			var fix *analysis.SuggestedFix
			if fixableSite {
				// The rewrite itself is PS3023's: only the selector name and
				// the text after the second slice argument change.
				fix = ps3023Fix(f, call, sel)
				if fix != nil {
					fixable++
				}
			}
			sites = append(sites, site{call, elem, why, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call deletes exactly one strings reference; when
		// those are ALL of the file's strings references, the rewrites
		// orphan the import and the fix must drop it (the runner never
		// prunes imports itself). The slices import always survives: the
		// rewritten call still spells slices.Compare through the same
		// qualifier.
		dropStrings := fixable > 0 && pkgRefCount(pass, f, "strings") == fixable
		importEdits, importsOK := ps3009ImportEdits(f, dropStrings)
		if !importsOK {
			// cgo file needing import surgery, or a strings spec we could
			// not locate: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edit (same convention as
			// PS3009/PS3011/PS3023).
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
				Message: "slices.CompareFunc with a bare strings.Compare comparator pays an indirect comparator call plus a strings.Compare call per element pair; slices.Compare compares the " + st.elem + " elements with the identical lexicographic result and the comparison inlined" + st.why,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3036Site classifies one matched call: fixable when the rewrite provably
// typechecks AND is byte-identical. Because strings.Compare's parameters are
// plain string, a matched comparator only type-checks over string (or
// string-alias) elements — where strings.Compare and cmp.Compare agree
// exactly, value for value — so, like PS3023, the only advisory corners are
// shape: an explicitly instantiated call (CompareFunc has four type
// parameters, Compare two, so the brackets cannot survive) and two slice
// arguments of DIFFERENT types (CompareFunc takes independent S1/S2, Compare
// one S for both).
func ps3036Site(pass *analysis.Pass, call *ast.CallExpr) (elem string, fixable bool, why string) {
	elem, elemOK, why := ps3009Elem(pass, call.Args[2])
	if !elemOK {
		return elem, false, why
	}
	switch ps2110Unparen(call.Fun).(type) {
	case *ast.IndexExpr, *ast.IndexListExpr:
		return elem, false, " (no auto-fix: explicit instantiation — slices.CompareFunc has four type parameters, slices.Compare two, so the brackets cannot survive the rewrite)"
	}
	t1, t2 := pass.TypesInfo.TypeOf(call.Args[0]), pass.TypesInfo.TypeOf(call.Args[1])
	if t1 == nil || t2 == nil {
		return elem, false, " (no auto-fix: slice types unresolved)"
	}
	if !types.Identical(t1, t2) {
		return elem, false, " (no auto-fix: the slice types " + types.TypeString(t1, types.RelativeTo(pass.Pkg)) + " and " + types.TypeString(t2, types.RelativeTo(pass.Pkg)) + " differ — slices.Compare takes one slice type for both sides)"
	}
	return elem, true, ""
}
