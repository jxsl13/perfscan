package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3111 reports a slices.MaxFunc / slices.MinFunc whose comparator is nothing
// but cmp.Compare on the two parameters in source order — slices.Max /
// slices.Min spelled the slow way — and rewrites it to the direct form. Sibling
// of PS3107 (SortFunc with a bare cmp.Compare comparator): the same "generic
// Func variant fed cmp.Compare" anti-pattern, on the single-pass extremum.
var PS3111 = register(&lint.Check{
	ID:       "PS3111",
	Category: "indirect",
	Slug:     "maxfunc-cmp-compare-to-slices-max",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.MaxFunc/MinFunc with a bare cmp.Compare comparator is slices.Max/Min spelled the slow way",
		Text: `slices.MaxFunc(s, func(a, b T) int { return cmp.Compare(a, b) })
walks s once and keeps the greatest element — exactly what slices.Max(s) does —
but pays an indirect call through the comparator func value on EVERY element, and
that closure itself calls cmp.Compare, a second call layer. slices.Max is a
distinct monomorphized entry point that inlines the ordered comparison (via the
builtin max). slices.MinFunc/slices.Min are the symmetric pair. Passing
cmp.Compare itself as the comparator is the same anti-pattern minus one layer and
is matched too.

The fix is offered only when the element type's underlying type is an INTEGER
kind. There any two elements that compare equal are bitwise-identical values, so
which of a tie the scan keeps is unobservable and slices.Max is byte-for-byte
identical (both panic on an empty slice, and cmp.Compare and the builtin max
induce the same order), AND the builtin max inlines to a CMP/CSEL that is at
least as fast as MaxFunc. Named element types (type Priority int) are fixed too.

STRING elements are bit-identical too but reported ADVISORY only, never
auto-fixed: slices.Max/Min on a []string fold via an outlined runtime.strmax/
strmin call per element, which benchmarks ~10-25% SLOWER on gc than the
devirtualized MaxFunc(cmp.Compare) loop (verified on go1.26). Auto-fixing it
would trade a hand-recognizable idiom for provably slower code, so the report
stays advisory and the human decides.

FLOAT elements are reported ADVISORY only, never auto-fixed — the same policy as
PS3107. slices.Max is defined via the builtin max, which PROPAGATES NaN (any NaN
forces a NaN result), while MaxFunc(cmp.Compare) treats a NaN as the SMALLEST
value (cmp.Compare orders NaN first) and so never selects it: the two disagree on
a slice containing NaN, so byte-identity cannot be guaranteed. A TYPE-PARAMETER
element is likewise advisory (its instantiations may include floats).

The match is deliberately EXACT (the shared PS3107 comparator matcher): the
comparator must be a func literal whose body is a single return cmp.Compare(p0,
p1) with the two parameters in SOURCE ORDER — a swapped cmp.Compare(b, a) is the
OPPOSITE extremum and is never matched — or cmp.Compare itself. Both cmp and
slices are resolved with type information; an aliased import matches naturally.
The fix replaces the Func selector with the base name and deletes the comparator;
the target slice is kept verbatim, and deleting the comparator drops the file's
cmp import when it removes the last cmp reference (a cgo file or a comment in the
deleted span keeps the report advisory). Fires only from go1.21 on (slices.Max,
slices.Min and cmp.Compare all appeared there).`,
		Before: `hi := slices.MaxFunc(xs, func(a, b int) int { return cmp.Compare(a, b) })`,
		After:  `hi := slices.Max(xs)`,
		MeasuredWin: `BenchmarkPS3111 (a scattered 4096-element []int scanned per
op, Apple M2 Pro, gc 1.26): ~2.42 µs/op before vs ~2.38 µs/op after — parity, 0
allocs either way. gc inlines slices.MaxFunc and devirtualizes the literal
closure, so both become the same direct-comparison scan; the win is source-level
robustness (the closure and its cmp.Compare scaffolding go away, and slices.Max
cannot fall off the devirtualization path a hoisted or grown callback can), plus
a real per-element indirect call removed on toolchains without closure
devirtualization (gccgo, tinygo, older gc) — the same class as PS1008/PS3110.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3111",
		Doc:  "slices.MaxFunc/MinFunc with a bare cmp.Compare comparator instead of slices.Max/Min",
		Run:  runPS3111,
	},
})

// ps3111Base maps the matched Func variant to its direct spelling.
var ps3111Base = map[string]string{"MaxFunc": "Max", "MinFunc": "Min"}

func runPS3111(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			continue // slices.Max/Min exist only from go1.21 on (same gate as PS3107).
		}
		type site struct {
			call *ast.CallExpr
			base string
			elem string
			why  string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			base, wanted := ps3111Base[sel.Sel.Name]
			if !wanted {
				return true
			}
			s, ok := ps3107PkgFunc(pass, call.Fun, "slices", sel.Sel.Name)
			if !ok {
				return true
			}
			// The comparator must BE cmp.Compare on the parameters in source
			// order (reuses PS3107's exact matcher).
			if !ps3107BareCompare(pass, call.Args[1]) {
				return true
			}
			// int/string fixable (equal values are bitwise-identical, ties
			// unobservable), float and type-parameter advisory (NaN diverges
			// between the builtin max and cmp.Compare).
			elem, fixableElem, why := ps3111Elem(pass, call.Args[1])
			var fix *analysis.SuggestedFix
			if fixableElem {
				fix = ps3111Fix(f, call, s, base)
				if fix != nil {
					fixable++
				}
			}
			sites = append(sites, site{call, base, elem, why, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
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
				Message: "slices." + st.base + "Func with a bare cmp.Compare comparator pays an indirect comparator call per element; slices." + st.base + " computes the extremal " + st.elem + " value with the identical result and the comparison inlined" + st.why,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3111Elem classifies the extremum's element type (the comparator's first
// parameter type): fixable only for an integer kind (equal values are
// bitwise-identical, so tie selection is unobservable and slices.Max/Min match
// byte-for-byte, AND the builtin max/min inlines to a CMP/CSEL that is at least
// as fast). String is bit-identical too but stays ADVISORY: slices.Max/Min on a
// string slice fold via an outlined runtime.strmax/strmin call per element,
// which benchmarks ~10-25% SLOWER on gc than the devirtualized
// MaxFunc(cmp.Compare) loop — so auto-fixing it would recommend slower code.
// Floats are advisory (slices.Max/Min use the builtin max/min, which PROPAGATE
// NaN, whereas cmp.Compare orders NaN first and never selects it) and so are
// type parameters (instantiations may include floats).
func ps3111Elem(pass *analysis.Pass, comparator ast.Expr) (elem string, fixable bool, why string) {
	sig, ok := pass.TypesInfo.TypeOf(comparator).(*types.Signature)
	if !ok || sig.Params().Len() != 2 {
		return "", false, " (no auto-fix: comparator type unresolved)"
	}
	t := sig.Params().At(0).Type()
	elem = types.TypeString(t, types.RelativeTo(pass.Pkg))
	if _, isParam := t.(*types.TypeParam); isParam {
		return elem, false, " (no auto-fix: type-parameter element, instantiations may include floats whose NaN handling differs)"
	}
	b, isBasic := t.Underlying().(*types.Basic)
	if !isBasic {
		return elem, false, " (no auto-fix: element type unresolved)"
	}
	switch {
	case b.Info()&types.IsInteger != 0:
		return elem, true, ""
	case b.Info()&types.IsString != 0:
		return elem, false, " (no auto-fix: slices.Max/Min on a string slice fold to an outlined runtime.strmax/strmin call per element, ~10-25% slower on gc than the devirtualized MaxFunc(cmp.Compare); bit-identical but a measured perf regression)"
	case b.Info()&types.IsFloat != 0:
		return elem, false, " (no auto-fix: slices.Max/Min propagate NaN via the builtin max/min while cmp.Compare orders NaN first, so they differ on a NaN element)"
	default:
		return elem, false, " (no auto-fix: element type unresolved)"
	}
}

// ps3111Fix builds the slices.Max(s) / slices.Min(s) rewrite: the Func selector
// becomes the base name and the text after the slice argument (the comma, the
// comparator and the closing paren) becomes ")". The slice stays verbatim in
// place and the package qualifier keeps its alias. The cmp-import edit is
// appended later, once per file. A comment in the deleted span keeps the report
// advisory.
func ps3111Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr, base string) *analysis.SuggestedFix {
	if ps2111CommentIn(f, call.Args[0].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + base,
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte(base)},
			{Pos: call.Args[0].End(), End: call.End(), NewText: []byte(")")},
		},
	}
}
