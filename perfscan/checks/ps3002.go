package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3002 reports sort.Slice/sort.SliceStable with a comparator closure.
var PS3002 = register(&lint.Check{
	ID:       "PS3002",
	Category: "indirect",
	Slug:     "closure-comparator-sort",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a package sort (sort.Slice/SliceStable) with a comparator closure",
		Text: `sort.Slice swaps elements through reflection and calls the
comparator closure through an interface — two indirections per comparison.
The generic slices.SortFunc (Go 1.21+) sorts the concrete slice type with a
direct call, and a sort.Sort on a concrete sort.Interface implementation
avoids the reflect-based swaps too.

The automatic fix (L2) handles the shape where the sorted value is a plain
identifier xs and the comparator body is exactly "return xs[i]<CHAIN> <
xs[j]<CHAIN>" with the same (possibly empty) selector chain on both sides and
an ordered basic element/field type. Two forms:

  - Whole element (empty chain, "xs[i] < xs[j]") → slices.Sort(xs), which
    drops the comparator entirely. Needs only "slices" in scope.
  - A field ("xs[i].f < xs[j].f") → slices.SortFunc(xs, func(a, b T) int {
    return cmp.Compare(a.f, b.f) }). Needs "slices" and "cmp" in scope and a
    locally spellable element type T.

FLOAT is excluded from the fix (advisory only): cmp.Compare and slices.Sort
order NaN as the smallest value while the '<' comparator treats NaN as
incomparable, so the rewrite is not bit-identical for a slice that may hold a
NaN — the same exclusion PS3104/PS3105 apply. A SliceStable over an ordered
basic type collapses to the unstable sort because equal elements are
indistinguishable. Every other comparator stays advisory.`,
		Before: `sort.Slice(xs, func(i, j int) bool { return xs[i].Key < xs[j].Key })`,
		After:  `slices.SortFunc(xs, func(a, b Item) int { return cmp.Compare(a.Key, b.Key) })`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3002",
		Doc:  "sort.Slice with comparator closure",
		Run:  runPS3002,
	},
})

var sortSliceFuncs = map[string]bool{
	"Slice":       true,
	"SliceStable": true,
}

func runPS3002(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Two passes per file: collect first, so the fixes can be
		// suppressed when applying ALL of them would rewrite the file's
		// last sort.* reference and orphan the import (the runner never
		// prunes imports; same guard as PS3077's math handling).
		type site struct {
			call *ast.CallExpr
			name string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := astutil.PkgFuncCall(call.Fun, "sort", sortSliceFuncs)
			if !ok {
				return true
			}
			fix := sortFuncFix(pass, call, name)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, name, fix})
			return true
		})
		// Each fixable call holds exactly one sort reference (its
		// selector); if those are ALL of the file's sort references, the
		// fixes would orphan the import — advisory only then.
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "sort") > fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "sort." + st.name + " swaps through reflection and calls its comparator indirectly; slices.SortFunc sorts the concrete type directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// sortFuncFix builds the slices.SortFunc rewrite for the one provably safe
// shape:
//
//	sort.Slice(xs, func(i, j int) bool { return xs[i].f.g < xs[j].f.g })
//
// where xs is a plain identifier of slice type, the comparator body is a
// single return of a '<' comparison whose operands are identical selector
// chains rooted at xs[i] (left) and xs[j] (right), the compared type is an
// ordered basic type, and the packages slices and cmp are importable by
// name at the call site (already imported, un-renamed, not shadowed). All
// other shapes get the advisory report only.
func sortFuncFix(pass *analysis.Pass, call *ast.CallExpr, name string) *analysis.SuggestedFix {
	// The rewrite targets the standard library sort package only: a
	// same-named third-party package could give Slice other semantics.
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	pkgID, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if pn, ok := pass.TypesInfo.Uses[pkgID].(*types.PkgName); !ok || pn.Imported().Path() != "sort" {
		return nil
	}
	if len(call.Args) != 2 {
		return nil
	}
	xs, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return nil
	}
	xsObj := pass.TypesInfo.ObjectOf(xs)
	if xsObj == nil {
		return nil
	}
	sliceType, ok := underlyingSlice(pass.TypesInfo.TypeOf(xs))
	if !ok {
		return nil
	}
	fl, ok := call.Args[1].(*ast.FuncLit)
	if !ok {
		return nil
	}
	// Exactly two int parameters and a single (bool) result.
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
	// Body: exactly `return A < B`.
	if len(fl.Body.List) != 1 {
		return nil
	}
	ret, ok := fl.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil
	}
	bin, ok := ret.Results[0].(*ast.BinaryExpr)
	if !ok || bin.Op != token.LSS {
		return nil
	}
	fieldsA, baseA, idxA, ok := indexSelectorChain(bin.X)
	if !ok || idxA.Name != params[0] {
		return nil
	}
	fieldsB, baseB, idxB, ok := indexSelectorChain(bin.Y)
	if !ok || idxB.Name != params[1] {
		return nil
	}
	// Both chains index the SAME variable as the first argument, and walk
	// the same fields.
	if pass.TypesInfo.ObjectOf(baseA) != xsObj || pass.TypesInfo.ObjectOf(baseB) != xsObj {
		return nil
	}
	if len(fieldsA) != len(fieldsB) {
		return nil
	}
	for i := range fieldsA {
		if fieldsA[i] != fieldsB[i] {
			return nil
		}
	}
	// The compared type must be an ordered basic type — the domain of
	// cmp.Compare and slices.Sort.
	cmpType := pass.TypesInfo.TypeOf(bin.X)
	if cmpType == nil {
		return nil
	}
	basic, ok := cmpType.Underlying().(*types.Basic)
	if !ok || basic.Info()&types.IsOrdered == 0 {
		return nil
	}
	// Floats are ordered but NOT safe: cmp.Compare and slices.Sort order NaN as
	// the smallest value, whereas the '<' comparator treats NaN as incomparable,
	// so the two disagree on any slice that can contain a NaN. Advisory only —
	// the same float exclusion PS3104/PS3105 apply for exactly this reason.
	if basic.Info()&types.IsFloat != 0 {
		return nil
	}
	// slices must resolve to the std package at the call site (imported here,
	// un-renamed, not shadowed) — required by BOTH rewrites below.
	if !stdPkgInScope(pass, call.Pos(), "slices") {
		return nil
	}

	// Whole-element ascending compare — the body is exactly `xs[i] < xs[j]`
	// (empty selector chain) — becomes slices.Sort(xs), dropping the comparator
	// entirely. For an ordered non-float element '<' is the total order
	// slices.Sort already uses, and equal elements are indistinguishable so a
	// SliceStable collapses to the same result. No cmp import, no element
	// spelling needed.
	if len(fieldsA) == 0 {
		return &analysis.SuggestedFix{
			Message: fmt.Sprintf("replace sort.%s with slices.Sort", name),
			TextEdits: []analysis.TextEdit{
				{Pos: call.Pos(), End: call.End(), NewText: fmt.Appendf(nil, "slices.Sort(%s)", xs.Name)},
			},
		}
	}

	// Field compare — `xs[i].f < xs[j].f` — becomes slices.SortFunc with
	// cmp.Compare, so cmp must also be importable and the element type spellable
	// without a new import.
	if !stdPkgInScope(pass, call.Pos(), "cmp") {
		return nil
	}
	elem := types.Unalias(sliceType.Elem())
	if !locallySpellable(elem, pass.Pkg) {
		return nil
	}
	elemStr := types.TypeString(elem, types.RelativeTo(pass.Pkg))

	fn := "SortFunc"
	if name == "SliceStable" {
		fn = "SortStableFunc"
	}
	suffix := "." + strings.Join(fieldsA, ".")
	newText := fmt.Sprintf("slices.%s(%s, func(a, b %s) int { return cmp.Compare(a%s, b%s) })",
		fn, xs.Name, elemStr, suffix, suffix)
	return &analysis.SuggestedFix{
		Message: fmt.Sprintf("replace sort.%s with slices.%s", name, fn),
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: call.End(), NewText: []byte(newText)},
		},
	}
}

// indexSelectorChain matches base[idx], base[idx].f, base[idx].f.g, ...
// returning the selector names (outermost last), the indexed identifier and
// the index identifier. Anything else — parens, calls, nested indexes —
// does not match.
func indexSelectorChain(e ast.Expr) (fields []string, base, index *ast.Ident, ok bool) {
	for {
		sel, isSel := e.(*ast.SelectorExpr)
		if !isSel {
			break
		}
		fields = append([]string{sel.Sel.Name}, fields...)
		e = sel.X
	}
	ix, isIx := e.(*ast.IndexExpr)
	if !isIx {
		return nil, nil, nil, false
	}
	base, okBase := ix.X.(*ast.Ident)
	index, okIdx := ix.Index.(*ast.Ident)
	if !okBase || !okIdx {
		return nil, nil, nil, false
	}
	return fields, base, index, true
}

// stdPkgInScope reports whether name resolves, at pos, to the imported
// standard-library package with that import path — i.e. the file imports it
// un-renamed and no local declaration shadows it.
func stdPkgInScope(pass *analysis.Pass, pos token.Pos, name string) bool {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		return false
	}
	_, obj := scope.LookupParent(name, pos)
	pn, ok := obj.(*types.PkgName)
	return ok && pn.Imported().Path() == name
}

// locallySpellable reports whether t can be rendered in the current package
// without importing anything: basic types, non-generic named types declared
// in pkg (or universe), and pointers to those.
func locallySpellable(t types.Type, pkg *types.Package) bool {
	switch tt := types.Unalias(t).(type) {
	case *types.Basic:
		return tt.Info()&types.IsUntyped == 0
	case *types.Pointer:
		return locallySpellable(tt.Elem(), pkg)
	case *types.Named:
		if tt.TypeArgs().Len() > 0 {
			return false
		}
		obj := tt.Obj()
		return obj.Pkg() == nil || obj.Pkg() == pkg
	}
	return false
}
