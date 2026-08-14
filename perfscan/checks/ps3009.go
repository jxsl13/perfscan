package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3009 reports a slices.SortFunc or slices.SortStableFunc whose comparator
// is nothing but strings.Compare on the two parameters in source order —
// slices.Sort spelled the slow way for string elements — and rewrites it to
// slices.Sort. Sibling of PS3107/PS3006 (the same shape with a cmp.Compare
// comparator): PS3009 catches the strings-package spelling, which PS3107's
// matcher deliberately leaves out of scope.
var PS3009 = register(&lint.Check{
	ID:       "PS3009",
	Category: "indirect",
	Slug:     "sortfunc-strings-compare-to-slices-sort",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.SortFunc/SortStableFunc with a bare strings.Compare(a, b) comparator is slices.Sort spelled the slow way",
		Text: `slices.SortFunc(s, func(a, b string) int { return strings.Compare(a, b) })
sorts ascending byte-lexicographically — exactly what slices.Sort(s) does
on strings — but pays an indirect call through the comparator func value
on EVERY comparison, and inside that closure strings.Compare is itself
another call doing a full three-way compare. slices.Sort is a distinct
monomorphized entry point that inlines the ordered '<' comparison
directly into pdqsort: same order, zero comparator indirection. Passing
strings.Compare itself as the comparator (slices.SortFunc(s,
strings.Compare)) is the same anti-pattern minus one layer and is
matched too. The STABLE spelling slices.SortStableFunc(s, ...) with the
same comparator additionally pays the stable algorithm's insertion-run /
symMerge machinery, which buys a tie-order guarantee that is
unobservable for strings — it is matched and rewritten the same way.

The result is IDENTICAL, and the proof is strictly simpler than
PS3107/PS3006's: the element type is always string (strings.Compare's
parameters are string, so a comparator built from it only type-checks
over string elements — a defined type with underlying string does not
compile without conversions, which the matcher never accepts). ORDER:
strings.Compare(a, b) < 0 iff a < b byte-lexicographically, which is
precisely cmp.Less on string — the order slices.Sort is defined by — so
every comparison answers the same on both sides and the non-tie
permutation is identical. TIES: strings that compare equal are
byte-equal and therefore interchangeable in every safe-Go operation, so
the unstable pdqsort's freedom to arrange ties (and SortStableFunc's
guarantee to keep them) is unobservable and the output slice is
elementwise identical. Invalid UTF-8 is irrelevant — both sides order by
raw bytes, never runes. The float/NaN advisory carve-outs of the
cmp.Compare family cannot arise here at all.

The match is deliberately EXACT — the same shape as PS3107. The
comparator must be a func literal whose body is a single return of
strings.Compare(p0, p1) with the two parameters in SOURCE ORDER —
a swapped strings.Compare(b, a) is a descending sort and is never
matched. The arguments must be the bare parameters, resolved by object
identity: a conversion (strings.Compare(string(a), string(b))), a field
selector, a captured outer variable, any extra statement or arithmetic
in the body, or a comparator that is anything but a fresh literal /
strings.Compare itself all fail the match silently. Both strings and
slices are resolved with type information — only the stdlib packages
match, never a shadowed local or a same-named method — and an aliased
import matches naturally.

The fix replaces only the SortFunc/SortStableFunc selector name with
Sort and deletes the comparator argument: the target slice expression is
kept VERBATIM in place (single evaluation preserved), the package
qualifier keeps whatever alias the file used, and an explicit
instantiation slices.SortFunc[S, E](...) keeps its brackets —
slices.Sort has the same two type parameters and string satisfies
cmp.Ordered, so slices.Sort[S, E](s) still compiles. Deleting the
comparator removes the file's strings reference, so when the rewrites
remove the file's LAST strings references the fix also drops the
strings import (alias included); a cgo file (whose import block must not
be edited) or a comment inside the deleted span keeps the report
advisory.

The report only fires when the effective language version is at least
go1.21 (slices.SortFunc, slices.SortStableFunc and slices.Sort all
appeared there) — the same gate PS3104/PS3105/PS3107/PS3006 apply; in
practice code containing this pattern already compiles only on go1.21+.`,
		Before: `slices.SortFunc(s, func(a, b string) int { return strings.Compare(a, b) })`,
		After:  `slices.Sort(s)`,
		MeasuredWin: `BenchmarkPS3009 (a shuffled tie-heavy 4096-element
[]string copied and sorted per op, Apple M2 Pro, go1.26): 435 µs/op
before (SortFunc) and 750 µs/op before (SortStableFunc) vs 377 µs/op
after — about 1.15x and 2.0x — 0 allocs on every side. The delta is
smaller than the []int siblings' (PS3107/PS3006) because a string
comparison is a runtime cmpstring call on BOTH sides; what the rewrite
removes is the indirect comparator hop around it (the 1.15x) and, for
the stable spelling, the insertion-run/symMerge machinery pdqsort skips
(the 2x).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3009",
		Doc:  "slices.SortFunc/SortStableFunc with a bare strings.Compare(a, b) comparator instead of slices.Sort",
		Run:  runPS3009,
	},
})

func runPS3009(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.Sort exists only from go1.21 on; below that the
			// advice is moot, so stay silent entirely (same gate as
			// PS3104/PS3105/PS3107/PS3006).
			continue
		}
		// Collect first, decide the strings-import edit once per file:
		// every fixable call deletes exactly ONE strings reference (the
		// comparator's strings.Compare selector), and whether the strings
		// import is orphaned depends on ALL of them together (same
		// per-file site collection as PS3107/PS3006).
		type site struct {
			call   *ast.CallExpr
			callee string // "SortFunc" or "SortStableFunc", for the message
			elem   string // rendered element type, for the message
			why    string // non-empty: advisory-by-design reason suffix
			fix    *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the receiver-less package function
			// slices.SortFunc or slices.SortStableFunc, resolved through
			// type info — never a shadowed slices or a same-named method.
			// An explicit instantiation is unwrapped; its brackets survive
			// the fix (slices.Sort has the same two type parameters).
			sel, callee, ok := ps3009SortCallee(pass, call.Fun)
			if !ok {
				return true
			}
			// The comparator must BE strings.Compare on the parameters in
			// source order — a fresh literal wrapping it, or
			// strings.Compare itself — or the call is not an ascending
			// slices.Sort and is never reported.
			if !ps3009BareCompare(pass, call.Args[1]) {
				return true
			}
			elem, fixableElem, why := ps3009Elem(pass, call.Args[1])
			var fix *analysis.SuggestedFix
			if fixableElem {
				fix = ps3107Fix(f, call, sel)
				if fix != nil {
					fixable++
				}
			}
			sites = append(sites, site{call, callee, elem, why, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call deletes exactly one strings reference; when
		// those are ALL of the file's strings references, the rewrites
		// orphan the import and the fix must drop it (the runner never
		// prunes imports itself). The slices import always survives: the
		// rewritten call still spells slices.Sort through the same
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
			// PS3104/PS3105/PS3107/PS3006).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			var msg string
			if st.callee == "SortFunc" {
				msg = "slices.SortFunc with a bare strings.Compare(a, b) comparator pays an indirect comparator call plus a strings.Compare call per comparison; slices.Sort sorts the " + st.elem + " elements in the identical byte-lexicographic order with the comparison inlined" + st.why
			} else {
				msg = "slices.SortStableFunc with a bare strings.Compare(a, b) comparator pays an indirect comparator call plus a strings.Compare call per comparison and the stable sort's merge overhead; slices.Sort sorts the " + st.elem + " elements in the identical byte-lexicographic order, the comparison inlined and no stability cost" + st.why
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

// ps3009SortCallee matches fun as the stdlib slices.SortFunc or
// slices.SortStableFunc (parens and explicit instantiation brackets
// unwrapped, resolved through type info) and returns the selector plus the
// matched name.
func ps3009SortCallee(pass *analysis.Pass, fun ast.Expr) (*ast.SelectorExpr, string, bool) {
	if sel, ok := ps3107PkgFunc(pass, fun, "slices", "SortFunc"); ok {
		return sel, "SortFunc", true
	}
	if sel, ok := ps3107PkgFunc(pass, fun, "slices", "SortStableFunc"); ok {
		return sel, "SortStableFunc", true
	}
	return nil, "", false
}

// ps3009BareCompare reports whether the comparator argument is EXACTLY the
// ascending strings.Compare order: either strings.Compare itself passed as
// the func value, or a func literal whose whole body is a single
// `return strings.Compare(p0, p1)` with the two parameters in SOURCE ORDER,
// resolved by object identity. A swapped (p1, p0) — descending — a
// conversion or selector around a parameter, a captured variable, or any
// extra computation fails the match. (Same shape as ps3107BareCompare, with
// strings.Compare in place of cmp.Compare.)
func ps3009BareCompare(pass *analysis.Pass, comparator ast.Expr) bool {
	if _, ok := ps3107PkgFunc(pass, comparator, "strings", "Compare"); ok {
		return true
	}
	lit, ok := ps2110Unparen(comparator).(*ast.FuncLit)
	if !ok {
		return false
	}
	// Exactly two named parameters, in source order across the fields
	// (func(a, b string) and func(a string, b string) both). A blank _
	// parameter cannot be referenced in the body, so requiring resolvable
	// names loses nothing.
	var params []*types.Var
	for _, field := range lit.Type.Params.List {
		for _, name := range field.Names {
			if name.Name == "_" {
				return false
			}
			v, isVar := pass.TypesInfo.Defs[name].(*types.Var)
			if !isVar {
				return false
			}
			params = append(params, v)
		}
	}
	if len(params) != 2 {
		return false
	}
	// The whole body is a single `return strings.Compare(p0, p1)`.
	if len(lit.Body.List) != 1 {
		return false
	}
	ret, ok := lit.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	inner, ok := ps2110Unparen(ret.Results[0]).(*ast.CallExpr)
	if !ok || len(inner.Args) != 2 || inner.Ellipsis.IsValid() {
		return false
	}
	if _, ok := ps3107PkgFunc(pass, inner.Fun, "strings", "Compare"); !ok {
		return false
	}
	for i, want := range params {
		id, isIdent := ps2110Unparen(inner.Args[i]).(*ast.Ident)
		if !isIdent || pass.TypesInfo.Uses[id] != want {
			return false
		}
	}
	return true
}

// ps3009Elem inspects the comparator's element type and classifies the
// site. Because strings.Compare's parameters are plain string, a matched
// comparator only type-checks over elements whose type IS string (or an
// alias of it) — a defined type with underlying string, or a type
// parameter, cannot be passed to strings.Compare without a conversion the
// matcher rejects — so a matched site is fixable whenever its type info
// resolved; the fallback stays advisory purely defensively.
func ps3009Elem(pass *analysis.Pass, comparator ast.Expr) (elem string, fixable bool, why string) {
	sig, ok := pass.TypesInfo.TypeOf(comparator).(*types.Signature)
	if !ok || sig.Params().Len() != 2 {
		return "", false, " (no auto-fix: comparator type unresolved)"
	}
	t := sig.Params().At(0).Type()
	elem = types.TypeString(t, types.RelativeTo(pass.Pkg))
	if _, isParam := t.(*types.TypeParam); isParam {
		return elem, false, " (no auto-fix: type-parameter element)"
	}
	if b, isBasic := t.Underlying().(*types.Basic); isBasic && b.Info()&types.IsString != 0 {
		return elem, true, ""
	}
	return elem, false, " (no auto-fix: element type unresolved)"
}

// ps3009ImportEdits returns the import-block edit a file's combined fixes
// need: dropping the file's strings import (alias included) when the
// rewrites orphan it. There is never an import to add — the rewritten call
// still references slices. ok is false when the import block must not or
// cannot be edited (cgo file, missing strings spec).
func ps3009ImportEdits(f *ast.File, dropStrings bool) (edits []analysis.TextEdit, ok bool) {
	if !dropStrings {
		return nil, true
	}
	if ps2110ImportsC(f) {
		return nil, false
	}
	edit, found := ps3009StringsImportEdit(f)
	if !found {
		return nil, false
	}
	return []analysis.TextEdit{edit}, true
}

// ps3009StringsImportEdit returns the edit removing the file's strings
// import spec (same spec-removal shapes as ps3107CmpImportEdit: sole spec
// drops the whole declaration, otherwise the spec plus one separating
// boundary).
func ps3009StringsImportEdit(f *ast.File) (analysis.TextEdit, bool) {
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for i, spec := range gd.Specs {
			is, ok := spec.(*ast.ImportSpec)
			if !ok || is.Path == nil || is.Path.Value != `"strings"` {
				continue
			}
			switch {
			case len(gd.Specs) == 1:
				// Sole spec: drop the whole declaration (parenthesized or
				// not); gofmt normalizes the leftover blank line.
				return analysis.TextEdit{Pos: gd.Pos(), End: gd.End()}, true
			case i+1 < len(gd.Specs):
				return analysis.TextEdit{Pos: is.Pos(), End: gd.Specs[i+1].Pos()}, true
			default:
				return analysis.TextEdit{Pos: gd.Specs[i-1].End(), End: is.End()}, true
			}
		}
	}
	return analysis.TextEdit{}, false
}
