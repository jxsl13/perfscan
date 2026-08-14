package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3107 reports an ascending slices.SortFunc whose comparator is nothing
// but cmp.Compare on the two parameters in source order — slices.Sort
// spelled the slow way — and rewrites it to slices.Sort. Sibling of
// PS3104/PS3105 (legacy sort-package spellings of the same ascending sort);
// PS3107 catches the MODERN slow spelling.
var PS3107 = register(&lint.Check{
	ID:       "PS3107",
	Category: "indirect",
	Slug:     "sortfunc-cmp-compare-to-slices-sort",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.SortFunc with a bare cmp.Compare(a, b) comparator is slices.Sort spelled the slow way",
		Text: `slices.SortFunc(s, func(a, b T) int { return cmp.Compare(a, b) })
sorts ascending — exactly what slices.Sort(s) does — but pays an indirect
call through the comparator func value on EVERY comparison, and that
closure itself calls cmp.Compare, a second call layer. slices.Sort is a
distinct monomorphized entry point that inlines the ordered '<'
comparison directly into pdqsort: same algorithm, zero comparator
indirection. Passing cmp.Compare itself as the comparator
(slices.SortFunc(s, cmp.Compare)) is the same anti-pattern minus one
layer and is matched too.

The result is BIT-IDENTICAL for the element types the fix accepts.
cmp.Compare(a, b) induces exactly the order slices.Sort uses:
slices.Sort is defined via cmp.Less, and cmp.Compare(a, b) < 0 iff
cmp.Less(a, b), so every comparison the shared pdqsort template makes
answers the same on both sides and the permutation is identical. The fix
is offered only when the element type's underlying type is an integer or
string kind — there any two elements that compare equal are
bitwise-identical values, so even the unstable sort's freedom to arrange
ties is unobservable and the output slice is byte-for-byte the same.
Named element types (type Celsius int) are fixed too: sorting orders by
the ordered value, a String()/Error() method on the element is never
consulted, and equal values remain identical bytes.

FLOAT elements are reported ADVISORY only, never auto-fixed — the same
policy as the whole PS3002/PS3104/PS3105 family: -0.0 and +0.0 compare
equal but differ in bits, as do NaNs with distinct payloads, so an
unstable sort's tie arrangement is observable at the bit level and
byte-identity cannot be contractually guaranteed. (The ORDER is still
identical — cmp.Compare and slices.Sort both put NaNs first — which is
why the report still fires as advice.) A TYPE-PARAMETER element is
likewise advisory: its instantiations may include floats.

The match is deliberately EXACT. The comparator must be a func literal
whose body is a single return of cmp.Compare(p0, p1) with the two
parameters in SOURCE ORDER — a swapped cmp.Compare(b, a) is a descending
sort and is never matched (that spelling is what PS3105's Reverse fix
produces). The arguments must be the bare parameters, resolved by object
identity: a field selector (cmp.Compare(a.f, b.f)), a captured outer
variable, any extra statement or arithmetic in the body, or a comparator
that is anything but a fresh literal / cmp.Compare itself all fail the
match silently. Both cmp and slices are resolved with type information —
only the stdlib packages match, never a shadowed local or a same-named
method — and an aliased import matches naturally.

The fix replaces only the SortFunc selector name with Sort and deletes
the comparator argument: the target slice expression is kept VERBATIM in
place (single evaluation preserved), the package qualifier keeps
whatever alias the file used, and an explicit instantiation
slices.SortFunc[S, E](...) keeps its brackets — slices.Sort has the same
two type parameters, so slices.Sort[S, E](s) still compiles. Deleting
the comparator removes the file's cmp reference, so when the rewrites
remove the file's LAST cmp references the fix also drops the cmp import
(alias included); a cgo file (whose import block must not be edited) or
a comment inside the deleted span keeps the report advisory.

The report only fires when the effective language version is at least
go1.21 (slices.SortFunc, slices.Sort and cmp.Compare all appeared
there) — the same gate PS3104/PS3105 apply; in practice code containing
this pattern already compiles only on go1.21+.`,
		Before: `slices.SortFunc(s, func(a, b int) int { return cmp.Compare(a, b) })`,
		After:  `slices.Sort(s)`,
		MeasuredWin: `BenchmarkPS3107 (a shuffled 4096-element []int copied and
sorted per op, Apple M2 Pro, go1.26): 238 µs/op before vs 148 µs/op
after — about 1.6x — 0 allocs either way, so the delta is pure
comparison overhead: the indirect comparator call (plus the cmp.Compare
hop inside it) per comparison that the monomorphized slices.Sort
inlines away.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3107",
		Doc:  "slices.SortFunc with a bare cmp.Compare(a, b) comparator instead of slices.Sort",
		Run:  runPS3107,
	},
})

func runPS3107(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.Sort exists only from go1.21 on; below that the
			// advice is moot, so stay silent entirely (same gate as
			// PS3104/PS3105).
			continue
		}
		// Collect first, decide the cmp-import edit once per file: every
		// fixable call deletes exactly ONE cmp reference (the comparator's
		// cmp.Compare selector), and whether the cmp import is orphaned
		// depends on ALL of them together (same per-file site collection
		// as PS3104/PS3105).
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
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			// The callee must be the receiver-less package function
			// slices.SortFunc, resolved through type info — never a
			// shadowed slices or a same-named method. An explicit
			// instantiation slices.SortFunc[S, E] is unwrapped; its
			// brackets survive the fix (slices.Sort has the same two type
			// parameters).
			sel, ok := ps3107PkgFunc(pass, call.Fun, "slices", "SortFunc")
			if !ok {
				return true
			}
			// The comparator must BE cmp.Compare on the parameters in
			// source order — a fresh literal wrapping it, or cmp.Compare
			// itself — or the call is not an ascending slices.Sort and is
			// never reported.
			if !ps3107BareCompare(pass, call.Args[1]) {
				return true
			}
			elem, fixableElem, why := ps3107Elem(pass, call.Args[1])
			var fix *analysis.SuggestedFix
			if fixableElem {
				fix = ps3107Fix(f, call, sel)
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
		// Each fixable call deletes exactly one cmp reference; when those
		// are ALL of the file's cmp references, the rewrites orphan the
		// import and the fix must drop it (the runner never prunes imports
		// itself). The slices import always survives: the rewritten call
		// still spells slices.Sort through the same qualifier.
		dropCmp := fixable > 0 && pkgRefCount(pass, f, "cmp") == fixable
		importEdits, importsOK := ps3107ImportEdits(f, dropCmp)
		if !importsOK {
			// cgo file needing import surgery, or a cmp spec we could not
			// locate: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edit (same convention as
			// PS3104/PS3105).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			msg := "slices.SortFunc with a bare cmp.Compare(a, b) comparator pays an indirect comparator call per comparison; slices.Sort sorts the " + st.elem + " elements with the identical ascending order and the comparison inlined" + st.why
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

// ps3107PkgFunc matches fun (parens and explicit instantiation brackets
// unwrapped) as a selector denoting the receiver-less function pkg.name of
// the stdlib package with path pkgPath, and returns that selector.
func ps3107PkgFunc(pass *analysis.Pass, fun ast.Expr, pkgPath, name string) (*ast.SelectorExpr, bool) {
	e := ps2110Unparen(fun)
	switch x := e.(type) {
	case *ast.IndexExpr:
		e = ps2110Unparen(x.X)
	case *ast.IndexListExpr:
		e = ps2110Unparen(x.X)
	}
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != pkgPath || fn.Name() != name {
		return nil, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, false
	}
	return sel, true
}

// ps3107BareCompare reports whether the comparator argument is EXACTLY the
// ascending cmp.Compare order: either cmp.Compare itself passed as the func
// value, or a func literal whose whole body is a single
// `return cmp.Compare(p0, p1)` with the two parameters in SOURCE ORDER,
// resolved by object identity. A swapped (p1, p0) — descending — a selector
// suffix, a captured variable, or any extra computation fails the match.
func ps3107BareCompare(pass *analysis.Pass, comparator ast.Expr) bool {
	if _, ok := ps3107PkgFunc(pass, comparator, "cmp", "Compare"); ok {
		return true
	}
	lit, ok := ps2110Unparen(comparator).(*ast.FuncLit)
	if !ok {
		return false
	}
	// Exactly two named parameters, in source order across the fields
	// (func(a, b T) and func(a T, b T) both). A blank _ parameter cannot
	// be referenced in the body, so requiring resolvable names loses
	// nothing.
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
	// The whole body is a single `return cmp.Compare(p0, p1)`.
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
	if _, ok := ps3107PkgFunc(pass, inner.Fun, "cmp", "Compare"); !ok {
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

// ps3107Elem inspects the comparator's element type and classifies the
// site: fixable when the underlying type is an integer or string kind
// (equal elements are bitwise-identical, so the unstable rewrite is
// byte-for-byte identical), advisory with a reason suffix for floats
// (distinguishable ties) and type parameters (instantiations may include
// floats).
func ps3107Elem(pass *analysis.Pass, comparator ast.Expr) (elem string, fixable bool, why string) {
	sig, ok := pass.TypesInfo.TypeOf(comparator).(*types.Signature)
	if !ok || sig.Params().Len() != 2 {
		return "", false, " (no auto-fix: comparator type unresolved)"
	}
	t := sig.Params().At(0).Type()
	elem = types.TypeString(t, types.RelativeTo(pass.Pkg))
	if _, isParam := t.(*types.TypeParam); isParam {
		return elem, false, " (no auto-fix: type-parameter element, instantiations may include floats whose ties are equal-but-distinguishable)"
	}
	b, isBasic := t.Underlying().(*types.Basic)
	if !isBasic {
		return elem, false, " (no auto-fix: element type unresolved)"
	}
	switch {
	case b.Info()&(types.IsInteger|types.IsString) != 0:
		return elem, true, ""
	case b.Info()&types.IsFloat != 0:
		return elem, false, " (no auto-fix: float ties -0.0/+0.0 and NaN payloads are equal-but-distinguishable under an unstable sort)"
	default:
		return elem, false, " (no auto-fix: element type unresolved)"
	}
}

// ps3107Fix builds the slices.Sort(s) rewrite for one call, or nil when a
// guard fails and the report must stay advisory. Only the SortFunc selector
// name and the text after the slice argument are replaced: the slice
// expression stays untouched in place (text and single evaluation
// preserved), the package qualifier keeps the file's alias, and explicit
// instantiation brackets survive — slices.Sort takes the same two type
// parameters. The cmp-import edit is appended later, once per file.
func ps3107Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	// The span from the slice argument's end to the call's end — the
	// comma, the whole comparator and the closing parenthesis — is
	// deleted; a comment there would be silently destroyed — advisory
	// then. (The other replaced span is the SortFunc identifier itself,
	// which cannot contain a comment.)
	if ps2111CommentIn(f, call.Args[0].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + ps3107Qualifier(sel) + "Sort(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Sort")},
			{Pos: call.Args[0].End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

// ps3107Qualifier renders the package qualifier of sel ("slices." for
// slices.SortFunc, the alias for an aliased import) for the fix message.
func ps3107Qualifier(sel *ast.SelectorExpr) string {
	if id, ok := sel.X.(*ast.Ident); ok {
		return id.Name + "."
	}
	return ""
}

// ps3107ImportEdits returns the import-block edit a file's combined fixes
// need: dropping the file's cmp import (alias included) when the rewrites
// orphan it. There is never an import to add — the rewritten call still
// references slices. ok is false when the import block must not or cannot
// be edited (cgo file, missing cmp spec).
func ps3107ImportEdits(f *ast.File, dropCmp bool) (edits []analysis.TextEdit, ok bool) {
	if !dropCmp {
		return nil, true
	}
	if ps2110ImportsC(f) {
		return nil, false
	}
	edit, found := ps3107CmpImportEdit(f)
	if !found {
		return nil, false
	}
	return []analysis.TextEdit{edit}, true
}

// ps3107CmpImportEdit returns the edit removing the file's cmp import spec
// (same spec-removal shapes as ps3104SortImportEdit: sole spec drops the
// whole declaration, otherwise the spec plus one separating boundary).
func ps3107CmpImportEdit(f *ast.File) (analysis.TextEdit, bool) {
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for i, spec := range gd.Specs {
			is, ok := spec.(*ast.ImportSpec)
			if !ok || is.Path == nil || is.Path.Value != `"cmp"` {
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
