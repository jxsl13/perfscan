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
identifier xs and the comparator body is a run of ordered comparisons
"xs[i]<CHAIN> < xs[j]<CHAIN>" (ascending) or "xs[i]<CHAIN> > xs[j]<CHAIN>"
(descending) with the same selector chain on both sides of each comparison
and an ordered basic element/field type. Three forms:

  - Whole element (empty chain, "return xs[i] < xs[j]") → slices.Sort(xs),
    which drops the comparator entirely. Needs only the "slices" package.
    Ascending only — slices.Sort has no comparator to reverse.
  - A single field ("return xs[i].f < xs[j].f") → slices.SortFunc(xs,
    func(a, b T) int { return cmp.Compare(a.f, b.f) }); a descending
    "return xs[i].f > xs[j].f" swaps the operands to cmp.Compare(b.f, a.f).
    Needs "slices" and "cmp" and a locally spellable element type T.
  - A multi-field tie-break chain of "if xs[i].f != xs[j].f { return
    xs[i].f < xs[j].f }" guards (each guard testing the very field it then
    orders) closed by a final "return xs[i].g < xs[j].g" → the equivalent
    cmp.Compare chain inside slices.SortFunc. Each field's direction is
    independent — a chain may mix '<' and '>' (e.g. date descending, then
    name ascending), each emitted with its own operand order. Both sorts
    run the same pdqsort, and the int chain returns the sign of the first
    differing field exactly where the bool chain returns its '<' or '>'
    (cmp.Compare(b.f, a.f) < 0 iff a.f > b.f), so the permutation is
    identical. Same "slices"+"cmp" requirements as the single field.

The fix edits imports as needed: a missing "slices" (and, for the field
forms, "cmp") import is ADDED at its sorted position, and an existing
import is reused under whatever alias the file gave it. The check itself
never touches the sort import — when the rewrites remove the file's last
sort reference, the fix pipeline prunes the orphaned import after
applying the edits. A dot- or blank-imported slices/cmp, a local name
shadowing the package at the call site, or a cgo file (whose import
block must not be edited) keeps the report advisory. The check only
fires when the effective language version is at least go1.21, where
slices.Sort/SortFunc and cmp.Compare exist — the same gate as PS3104.

FLOAT is excluded from the fix (advisory only): cmp.Compare and slices.Sort
order NaN as the smallest value while the '<' comparator treats NaN as
incomparable, so the rewrite is not bit-identical for a slice that may hold a
NaN — the same exclusion PS3104/PS3105 apply. A SliceStable over an ordered
basic type collapses to the unstable sort because equal elements are
indistinguishable. Every other comparator stays advisory.`,
		Before: `sort.Slice(xs, func(i, j int) bool {
	if xs[i].Group != xs[j].Group {
		return xs[i].Group < xs[j].Group
	}
	return xs[i].Kind < xs[j].Kind
})`,
		After: `slices.SortFunc(xs, func(a, b GVK) int {
	if a.Group != b.Group {
		return cmp.Compare(a.Group, b.Group)
	}
	return cmp.Compare(a.Kind, b.Kind)
})`,
		MeasuredWin: `BenchmarkPS3002 (a shuffled 10k copy sorted per op, Apple
M2 Pro, go1.26). Field compare -> slices.SortFunc: 1787 µs/op, 3 allocs ->
997 µs/op, 0 allocs (~1.8x, reflect-based struct swaps eliminated).
Whole-element compare -> slices.Sort: 652 µs/op, 2 allocs -> 389 µs/op, 0
allocs (~1.7x). The win is the reflect element swaps plus the per-comparison
interface dispatch that the compile-time-specialized generic sorts avoid.`,
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
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.Sort/SortFunc and cmp.Compare exist only from go1.21
			// on; below that the advice is moot, so stay silent entirely
			// (same gate as PS3104/PS2119).
			continue
		}
		// Two passes per file: collect first, then decide the import edits
		// once for the whole file — every fixable site needs "slices" and
		// the field-compare sites need "cmp" as well (same per-file site
		// collection as PS3104).
		type site struct {
			call *ast.CallExpr
			name string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable, cmpFixable := 0, 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := astutil.PkgFuncCall(call.Fun, "sort", sortSliceFuncs)
			if !ok {
				return true
			}
			fix, needsCmp := sortFuncFix(pass, f, call, name)
			if fix != nil {
				fixable++
				if needsCmp {
					cmpFixable++
				}
			}
			sites = append(sites, site{call, name, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// The fix ADDS the imports its rewrites need and leaves the sort
		// import alone: when the rewrites remove the file's last sort
		// reference, the fix pipeline prunes the orphaned import after
		// applying the edits (pure INSERTS never collide with the call
		// rewrites; a drop/swap of the sort spec could). The file-level
		// scan uses the import paths — an aliased slices/cmp import is
		// still an import, and sortFuncFix already resolved the alias.
		// A cgo file never gets here with a fix: sortFuncFix returns nil
		// for it, so no import edit is ever built for one.
		needSlicesFile := fixable > 0 && !ps3002FileImports(f, "slices")
		needCmpFile := cmpFixable > 0 && !ps3002FileImports(f, "cmp")
		var importEdits []analysis.TextEdit
		if needSlicesFile {
			importEdits = append(importEdits, ps2110ImportEdit(f, "slices"))
		}
		if needCmpFile {
			importEdits = append(importEdits, ps2110ImportEdit(f, "cmp"))
		}
		if len(importEdits) > 0 {
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
		emitFixes := fixable > 0
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

// ps3002FileImports reports whether f imports the given path (under any
// name, aliases included).
func ps3002FileImports(f *ast.File, path string) bool {
	for _, imp := range f.Imports {
		if imp.Path != nil && imp.Path.Value == `"`+path+`"` {
			return true
		}
	}
	return false
}

// sortFuncFix builds the slices.SortFunc rewrite for the provably safe
// shapes:
//
//	sort.Slice(xs, func(i, j int) bool { return xs[i].f.g < xs[j].f.g })
//
//	sort.Slice(xs, func(i, j int) bool {
//		if xs[i].f != xs[j].f {
//			return xs[i].f < xs[j].f
//		}
//		return xs[i].g < xs[j].g
//	})
//
// where xs is a plain identifier of slice type and the comparator body is a
// run of zero or more `if xs[i]<CHAIN> != xs[j]<CHAIN> { return xs[i]<CHAIN>
// OP xs[j]<CHAIN> }` tie-break guards — each guard ordering the very field it
// tests — closed by a final `return A OP B`, where each OP is independently
// '<' (ascending) or '>' (descending); an ascending field becomes
// cmp.Compare(a.f, b.f) and a descending field cmp.Compare(b.f, a.f) with the
// operands swapped. Every comparison's operands must be identical selector
// chains rooted at xs[i] (left) and xs[j] (right) over an ordered non-float
// basic type, and the packages slices and cmp must be usable by name at the
// call site: an existing import is reused (alias included), a missing one is
// ADDED by the per-file import edits runPS3002 builds — only a dot/blank
// import or a shadowing local keeps the site advisory, as does a cgo file,
// whose import block must never be edited. Same-field guards only: both
// sorts share the same pdqsort, and under these guards the bool chain and
// the cmp.Compare chain induce the identical total order —
// cmp.Compare(b.f, a.f) < 0 ⟺ a.f > b.f — so the resulting permutation is
// bit-identical (including ties). The whole-element form stays ascending
// only: slices.Sort has no comparator to swap operands in. All other shapes
// get the advisory report only.
//
// needsCmp reports whether the returned fix rewrites a field compare and so
// relies on the cmp package (the whole-element slices.Sort form does not) —
// the caller uses it to decide the file's cmp import edit.
func sortFuncFix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, name string) (fix *analysis.SuggestedFix, needsCmp bool) {
	// A cgo file's import block must not be edited, and whether an import
	// edit is needed is a per-file decision made after all sites are
	// collected — so no cgo site ever offers a fix.
	if ps2110ImportsC(f) {
		return nil, false
	}
	// The rewrite targets the standard library sort package only: a
	// same-named third-party package could give Slice other semantics.
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	pkgID, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, false
	}
	if pn, ok := pass.TypesInfo.Uses[pkgID].(*types.PkgName); !ok || pn.Imported().Path() != "sort" {
		return nil, false
	}
	if len(call.Args) != 2 {
		return nil, false
	}
	xs, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return nil, false
	}
	xsObj := pass.TypesInfo.ObjectOf(xs)
	if xsObj == nil {
		return nil, false
	}
	sliceType, ok := underlyingSlice(pass.TypesInfo.TypeOf(xs))
	if !ok {
		return nil, false
	}
	fl, ok := call.Args[1].(*ast.FuncLit)
	if !ok {
		return nil, false
	}
	// Exactly two int parameters and a single (bool) result.
	var params []string
	for _, field := range fl.Type.Params.List {
		if !types.Identical(pass.TypesInfo.TypeOf(field.Type), types.Typ[types.Int]) {
			return nil, false
		}
		for _, pn := range field.Names {
			params = append(params, pn.Name)
		}
	}
	if len(params) != 2 || params[0] == "_" || params[1] == "_" {
		return nil, false
	}
	if fl.Type.Results == nil || fl.Type.Results.NumFields() != 1 {
		return nil, false
	}
	// Body: zero or more `if xs[i].f != xs[j].f { return xs[i].f < xs[j].f }`
	// (or `> xs[j].f`) tie-break guards followed by exactly one final
	// `return A < B` / `return A > B`. Direction is INDEPENDENT per field.
	body := fl.Body.List
	if len(body) == 0 {
		return nil, false
	}
	type fieldCmp struct {
		suffix     string
		descending bool
	}
	fields := make([]fieldCmp, 0, len(body))
	for _, stmt := range body[:len(body)-1] {
		ifs, ok := stmt.(*ast.IfStmt)
		if !ok || ifs.Init != nil || ifs.Else != nil || len(ifs.Body.List) != 1 {
			return nil, false
		}
		condSuffix, ok := neqField(pass, ifs.Cond, xsObj, params[0], params[1])
		if !ok {
			return nil, false
		}
		ret, ok := ifs.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return nil, false
		}
		cmpSuffix, cmpDesc, ok := orderingCompare(pass, ret.Results[0], xsObj, params[0], params[1])
		// The guard must test the very field it then orders (an empty
		// chain — a whole-element compare — never belongs in a guard):
		// otherwise the bool chain and the cmp chain diverge on ties. The
		// return's direction does not affect the guard: `!=` is symmetric.
		if !ok || cmpSuffix == "" || cmpSuffix != condSuffix {
			return nil, false
		}
		fields = append(fields, fieldCmp{cmpSuffix, cmpDesc})
	}
	final, ok := body[len(body)-1].(*ast.ReturnStmt)
	if !ok || len(final.Results) != 1 {
		return nil, false
	}
	finalSuffix, finalDesc, ok := orderingCompare(pass, final.Results[0], xsObj, params[0], params[1])
	if !ok {
		return nil, false
	}
	// A whole-element compare is only valid as the sole statement AND
	// ascending (→ slices.Sort, which has no comparator to swap operands
	// in); as the tail of a tie-break chain it compares the whole element,
	// which cmp.Compare cannot express.
	if finalSuffix == "" && (len(fields) > 0 || finalDesc) {
		return nil, false
	}
	fields = append(fields, fieldCmp{finalSuffix, finalDesc})
	// slices must be usable by name at the call site — required by BOTH
	// rewrites below. An existing import is reused under its local name
	// (alias included); a missing one is fine, runPS3002 adds the import
	// edit per file. Only a dot/blank import or a shadowing local keeps
	// the site advisory.
	slicesName, _, usable := ps3104SlicesName(pass, f, call.Pos())
	if !usable {
		return nil, false
	}

	// Whole-element ascending compare — the body is exactly `xs[i] < xs[j]`
	// (empty selector chain) — becomes slices.Sort(xs), dropping the comparator
	// entirely. For an ordered non-float element '<' is the total order
	// slices.Sort already uses, and equal elements are indistinguishable so a
	// SliceStable collapses to the same result. No cmp package, no element
	// spelling needed.
	if fields[0].suffix == "" {
		return &analysis.SuggestedFix{
			Message: fmt.Sprintf("replace sort.%s with %s.Sort", name, slicesName),
			TextEdits: []analysis.TextEdit{
				{Pos: call.Pos(), End: call.End(), NewText: fmt.Appendf(nil, "%s.Sort(%s)", slicesName, xs.Name)},
			},
		}, false
	}

	// Field compare(s) — `xs[i].f < xs[j].f` or `xs[i].f > xs[j].f`, possibly
	// behind `!=` tie-break guards — become slices.SortFunc with one
	// cmp.Compare per field (operands SWAPPED for a descending field:
	// cmp.Compare(b.f, a.f) < 0 ⟺ b.f < a.f ⟺ a.f > b.f, the bool
	// comparator's exact result), so cmp must also be usable by name (same
	// resolution as slices: reuse or add) and the element type spellable
	// without a new import.
	cmpName, _, usable := ps3002CmpName(pass, f, call.Pos())
	if !usable {
		return nil, false
	}
	elem := types.Unalias(sliceType.Elem())
	if !locallySpellable(elem, pass.Pkg) {
		return nil, false
	}
	elemStr := types.TypeString(elem, types.RelativeTo(pass.Pkg))

	fn := "SortFunc"
	if name == "SliceStable" {
		fn = "SortStableFunc"
	}
	// One cmp.Compare per field, its operand order set by that field's own
	// direction: ascending → cmp.Compare(a.f, b.f), descending →
	// cmp.Compare(b.f, a.f).
	compare := func(f fieldCmp) string {
		if f.descending {
			return fmt.Sprintf("%s.Compare(b%s, a%s)", cmpName, f.suffix, f.suffix)
		}
		return fmt.Sprintf("%s.Compare(a%s, b%s)", cmpName, f.suffix, f.suffix)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s.%s(%s, func(a, b %s) int { ", slicesName, fn, xs.Name, elemStr)
	for _, f := range fields[:len(fields)-1] {
		fmt.Fprintf(&b, "if a%s != b%s { return %s }; ", f.suffix, f.suffix, compare(f))
	}
	fmt.Fprintf(&b, "return %s })", compare(fields[len(fields)-1]))
	return &analysis.SuggestedFix{
		Message: fmt.Sprintf("replace sort.%s with %s.%s", name, slicesName, fn),
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: call.End(), NewText: []byte(b.String())},
		},
	}, true
}

// ps3002CmpName resolves how the fix must spell the cmp package at pos: the
// file's existing import name (alias included) when cmp is already imported,
// or the bare name "cmp" with needImport set when the file must add the
// import. usable is false when the name cannot be used — a dot or blank cmp
// import, or another object owning the name at pos (the mirror of
// ps3104SlicesName for the "cmp" path).
func ps3002CmpName(pass *analysis.Pass, f *ast.File, pos token.Pos) (name string, needImport, usable bool) {
	for _, imp := range f.Imports {
		if imp.Path.Value != `"cmp"` {
			continue
		}
		local := "cmp"
		if imp.Name != nil {
			local = imp.Name.Name
		}
		if local == "_" || local == "." {
			// Blank import gives no usable name; a dot import puts Compare
			// in scope unqualified but rewriting to a bare Compare(x, y) is
			// too fragile — advisory in both cases.
			return "", false, false
		}
		nf, ok := ps2110PkgUsable(pass, pos, local, "cmp")
		return local, false, ok && !nf
	}
	needImport, usable = ps2110PkgUsable(pass, pos, "cmp", "cmp")
	return "cmp", needImport, usable && needImport
}

// orderingCompare validates a single ordering comparison `xs[i]<CHAIN> <
// xs[j]<CHAIN>` (ascending) or `xs[i]<CHAIN> > xs[j]<CHAIN>` (descending) —
// identical chains, xs = xsObj, i/j in that order, ordered non-float leaf
// type — and returns the "."-prefixed selector suffix (empty for a
// whole-element compare) plus which direction it saw.
func orderingCompare(pass *analysis.Pass, expr ast.Expr, xsObj types.Object, iName, jName string) (suffix string, descending bool, ok bool) {
	bin, isBin := expr.(*ast.BinaryExpr)
	if !isBin || (bin.Op != token.LSS && bin.Op != token.GTR) {
		return "", false, false
	}
	suffix, ok = comparisonSuffix(pass, bin, xsObj, iName, jName)
	return suffix, bin.Op == token.GTR, ok
}

// neqField validates a tie-break guard condition `xs[i]<CHAIN> !=
// xs[j]<CHAIN>` under the same operand rules as orderingCompare and
// returns the same "."-prefixed selector suffix. `!=` is symmetric, so the
// guard is direction-agnostic: it pairs equally with an ascending or a
// descending return comparison on the same field.
func neqField(pass *analysis.Pass, expr ast.Expr, xsObj types.Object, iName, jName string) (suffix string, ok bool) {
	bin, isBin := expr.(*ast.BinaryExpr)
	if !isBin || bin.Op != token.NEQ {
		return "", false
	}
	return comparisonSuffix(pass, bin, xsObj, iName, jName)
}

// comparisonSuffix validates a binary comparison (the operator itself was
// already vetted by the caller) whose operands are identical selector chains
// rooted at xsObj[iName] (left) and xsObj[jName] (right) over an ordered
// basic type, and returns the selector chain as a "."-prefixed suffix (""
// for the empty whole-element chain). Floats are rejected: cmp.Compare and
// slices.Sort order NaN as the smallest value, whereas the '<'/'>'
// comparators treat NaN as incomparable, so the two disagree on any slice
// that can contain a NaN — the same float exclusion PS3104/PS3105 apply for
// exactly this reason.
func comparisonSuffix(pass *analysis.Pass, bin *ast.BinaryExpr, xsObj types.Object, iName, jName string) (string, bool) {
	fieldsA, baseA, idxA, ok := indexSelectorChain(bin.X)
	if !ok || idxA.Name != iName {
		return "", false
	}
	fieldsB, baseB, idxB, ok := indexSelectorChain(bin.Y)
	if !ok || idxB.Name != jName {
		return "", false
	}
	// Both chains index the SAME variable as the first argument, and walk
	// the same fields.
	if pass.TypesInfo.ObjectOf(baseA) != xsObj || pass.TypesInfo.ObjectOf(baseB) != xsObj {
		return "", false
	}
	if len(fieldsA) != len(fieldsB) {
		return "", false
	}
	for i := range fieldsA {
		if fieldsA[i] != fieldsB[i] {
			return "", false
		}
	}
	// The compared type must be an ordered, non-float basic type — the safe
	// domain of cmp.Compare and slices.Sort (see NaN note above).
	cmpType := pass.TypesInfo.TypeOf(bin.X)
	if cmpType == nil {
		return "", false
	}
	basic, ok := cmpType.Underlying().(*types.Basic)
	if !ok || basic.Info()&types.IsOrdered == 0 || basic.Info()&types.IsFloat != 0 {
		return "", false
	}
	if len(fieldsA) == 0 {
		return "", true
	}
	return "." + strings.Join(fieldsA, "."), true
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
