package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3023 reports a slices.CompareFunc whose comparator is nothing but
// cmp.Compare on the two parameters in source order — slices.Compare spelled
// the slow way — and rewrites it to the direct form. Sibling of PS3107
// (SortFunc with a bare cmp.Compare comparator) and PS3111 (MaxFunc/MinFunc):
// the same "generic Func variant fed cmp.Compare" anti-pattern, on the
// lexicographic two-slice comparison.
var PS3023 = register(&lint.Check{
	ID:       "PS3023",
	Category: "indirect",
	Slug:     "comparefunc-cmp-compare-to-slices-compare",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.CompareFunc with a bare cmp.Compare comparator is slices.Compare spelled the slow way",
		Text: `slices.CompareFunc(a, b, func(x, y T) int { return cmp.Compare(x, y) })
compares the two slices lexicographically — exactly what slices.Compare(a, b)
does — but pays an indirect call through the comparator func value on EVERY
element pair, and that closure itself calls cmp.Compare, a second call layer.
slices.Compare is a distinct monomorphized entry point whose body IS the same
loop with the comparator fixed to an inlined cmp.Compare(s1[i], s2[i]): same
early exit, same ±1 length tie-breaks, zero comparator indirection. Passing
cmp.Compare itself as the comparator is the same anti-pattern minus one layer
and is matched too.

The result is BYTE-IDENTICAL for EVERY element type the match can see —
including floats. Unlike slices.Max/Min (defined via the NaN-propagating
builtin max/min, which forced PS3111's float exclusion), slices.Compare calls
cmp.Compare per element pair INTERNALLY, so NaN ordering, NaN payloads and
-0.0/+0.0 behave the same on both sides by construction: every returned int
and the early-exit index agree on any input. Both slices are read, never
written, so there is no tie-arrangement freedom to observe; comparison count
and (side-effect-free) argument evaluation are unchanged. Integer, string and
float element kinds are all auto-fixed, named element types (type Rank int)
included, and so is a type-parameter element — cmp.Compare already forced the
constraint to satisfy cmp.Ordered, which is exactly what slices.Compare
requires, and the equivalence argument is structural, independent of the
instantiation.

The fix is withheld (report stays ADVISORY) in two shape corners. MIXED SLICE
TYPES: slices.CompareFunc takes two independently-typed slices (S1, S2), while
slices.Compare takes one S for both — comparing a []int against a named
IntSlice compiles only in the Func spelling, so the fix requires the two slice
arguments to have IDENTICAL types. EXPLICIT INSTANTIATION:
slices.CompareFunc[S1, S2, E1, E2](...) has four type arguments where
slices.Compare has two, so the brackets cannot survive the rewrite and the
site is left to the human.

The match is deliberately EXACT (the shared PS3107 comparator matcher): the
comparator must be a func literal whose body is a single return cmp.Compare(x,
y) with the two parameters in SOURCE ORDER — a swapped cmp.Compare(y, x) is
slices.Compare(b, a), a different result, and is never matched — or
cmp.Compare itself. Both cmp and slices are resolved with type information; an
aliased import matches naturally. The fix replaces the CompareFunc selector
with Compare and deletes the comparator; both slice arguments are kept
verbatim in place, and deleting the comparator drops the file's cmp import
when it removes the last cmp reference (a cgo file or a comment in the deleted
span keeps the report advisory). Fires only from go1.21 on (slices.Compare,
slices.CompareFunc and cmp.Compare all appeared there).`,
		Before: `if slices.CompareFunc(a, b, cmp.Compare) < 0 { ... }`,
		After:  `if slices.Compare(a, b) < 0 { ... }`,
		MeasuredWin: `BenchmarkPS3023 (two 4096-element []int slices differing only
in the last element — a full-length scan — per op, Apple M2 Pro, gc 1.26):
~3.56 µs/op before vs ~3.55 µs/op after — parity, 0 allocs either way. gc
inlines slices.CompareFunc and devirtualizes the known comparator (literal or
cmp.Compare itself), so both become the same direct-comparison scan; the win
is source-level robustness (the closure and its cmp.Compare scaffolding go
away, and slices.Compare cannot fall off the devirtualization path a hoisted
or grown callback can), plus a real per-pair indirect call removed on
toolchains without closure devirtualization (gccgo, tinygo, older gc) — the
same class as PS1008/PS3110/PS3111.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3023",
		Doc:  "slices.CompareFunc with a bare cmp.Compare comparator instead of slices.Compare",
		Run:  runPS3023,
	},
})

func runPS3023(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			continue // slices.Compare exists only from go1.21 on (same gate as PS3107).
		}
		type site struct {
			call *ast.CallExpr
			elem string
			why  string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "CompareFunc")
			if !ok {
				return true
			}
			// The comparator must BE cmp.Compare on the parameters in source
			// order (reuses PS3107's exact matcher). Swapped operands would be
			// slices.Compare(b, a) — never matched.
			if !ps3107BareCompare(pass, call.Args[2]) {
				return true
			}
			elem, fixableSite, why := ps3023Site(pass, call)
			var fix *analysis.SuggestedFix
			if fixableSite {
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
		// Each fixable call deletes exactly one cmp reference (the
		// comparator's cmp.Compare selector); when those are ALL of the
		// file's cmp references the fix must also drop the cmp import (same
		// per-file convention as PS3107/PS3111).
		dropCmp := fixable > 0 && pkgRefCount(pass, f, "cmp") == fixable
		importEdits, importsOK := ps3107ImportEdits(f, dropCmp)
		if !importsOK {
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
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
				Message: "slices.CompareFunc with a bare cmp.Compare comparator pays an indirect comparator call per element pair; slices.Compare compares the " + st.elem + " elements with the identical lexicographic result and the comparison inlined" + st.why,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3023Site classifies one matched call: fixable when the rewrite provably
// typechecks AND is byte-identical. Byte-identity holds for EVERY element type
// (slices.Compare calls cmp.Compare per pair internally, floats included, and
// a type-parameter element already satisfies cmp.Ordered because cmp.Compare
// required it), so the only advisory corners are shape: an explicitly
// instantiated call (CompareFunc has four type parameters, Compare two, so the
// brackets cannot survive) and two slice arguments of DIFFERENT types
// (CompareFunc takes independent S1/S2, Compare one S for both).
func ps3023Site(pass *analysis.Pass, call *ast.CallExpr) (elem string, fixable bool, why string) {
	sig, ok := pass.TypesInfo.TypeOf(call.Args[2]).(*types.Signature)
	if !ok || sig.Params().Len() != 2 {
		return "", false, " (no auto-fix: comparator type unresolved)"
	}
	elem = types.TypeString(sig.Params().At(0).Type(), types.RelativeTo(pass.Pkg))
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

// ps3023Fix builds the slices.Compare(a, b) rewrite: the CompareFunc selector
// becomes Compare and the text after the second slice argument (the comma, the
// comparator and the closing paren) becomes ")". Both slice expressions stay
// verbatim in place (single evaluation, source order preserved) and the
// package qualifier keeps its alias. The cmp-import edit is appended later,
// once per file. A comment in the deleted span keeps the report advisory.
func ps3023Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	if ps2111CommentIn(f, call.Args[1].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + "Compare(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Compare")},
			{Pos: call.Args[1].End(), End: call.End(), NewText: []byte(")")},
		},
	}
}
