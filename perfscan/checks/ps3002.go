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

The automatic fix (L2) handles the shape where the sorted value is a
SIDE-EFFECT-FREE PATH xs — a plain identifier or a selector chain of
identifiers (c.Kinds, s.inner.items; field access through a pointer
included, since Go auto-derefs a selector) — and the comparator body is a
run of ordered comparisons "xs[i]<CHAIN> < xs[j]<CHAIN>" (ascending) or
"xs[i]<CHAIN> > xs[j]<CHAIN>" (descending) with the same selector chain on
both sides of each comparison and an ordered basic element/field type. The
comparator must index the very path being sorted (same root variable, same
fields); a target containing a call, an index expression, a dereference or
parentheses stays advisory — those could re-evaluate with side effects or
yield a different slice, whereas a pure path evaluates to the identical
slice header every time, so dropping the per-comparison re-evaluation is
observably free. Three forms:

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

Two further spellings of the tie-break chain are accepted, both exactly
equivalent: a guard may be written as a TWO-RETURN pair "if xs[i].f <
xs[j].f { return true }" / "if xs[i].f > xs[j].f { return false }" — when
the fields differ one of the two ifs returns the ordering verdict, when
they are equal both fall through, which is precisely the "!=" guard (the
'>'-returns-true flip is the descending form; any other operator/boolean
combination stays advisory) — and the chain may END in a bare "return
false" ("all fields equal → i does not sort before j"), which becomes
"return 0" after the emitted guards, cmp semantics for "equal". Pairs and
"!=" guards mix freely within one chain.

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
			name, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "sort", sortSliceFuncs)
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
// where xs is a SIDE-EFFECT-FREE PATH of slice type — a plain identifier or
// a selector chain of identifiers (c.Kinds, s.inner.items; see pathExpr) —
// and the comparator body is a
// run of zero or more tie-break guard UNITS — either a single `if xs[i]<CHAIN>
// != xs[j]<CHAIN> { return xs[i]<CHAIN> OP xs[j]<CHAIN> }` guard (each guard
// ordering the very field it tests) or the equivalent TWO-RETURN pair
// `if A < B { return true }; if A > B { return false }` (see
// ps3002TwoReturnPair) — closed by a final `return A OP B`, or, after at
// least one guard unit, a bare `return false` (the "all fields equal" tail,
// emitted as `return 0`), where each OP is independently
// '<' (ascending) or '>' (descending); an ascending field becomes
// cmp.Compare(a.f, b.f) and a descending field cmp.Compare(b.f, a.f) with the
// operands swapped. Every comparison's operands must be identical selector
// chains rooted at xs[i] (left) and xs[j] (right) over an ordered non-float
// basic type, and the packages slices and cmp must be usable by name at the
// call site: an existing import is reused (alias included), a missing one is
// ADDED by the per-file import edits runPS3002 builds — only a dot/blank
// import or a shadowing local keeps the site advisory, as does a cgo file,
// whose import block must never be edited. The path target is safe because it
// is evaluated exactly once as the first argument in BOTH spellings; the
// rewrite only drops the per-comparison E[i]/E[j] re-evaluations, which for a
// pure path yield the identical slice header every time — a call or index in
// the target could re-evaluate with side effects or produce a different
// slice, so those stay advisory, as does a comparator that indexes a
// DIFFERENT path than the one being sorted. Same-field guards only: both
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
	xsRoot, xsFields, ok := pathExpr(call.Args[0])
	if !ok {
		return nil, false
	}
	xsObj := pass.TypesInfo.ObjectOf(xsRoot)
	if xsObj == nil {
		return nil, false
	}
	// The comparator's index bases must spell this exact path: same root
	// object AND same field-name sequence (see ps3002SamePath).
	target := ps3002Path{obj: xsObj, fields: xsFields}
	sliceType, ok := underlyingSlice(pass.TypesInfo.TypeOf(call.Args[0]))
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
	// Body: an INDEX-based walk over a run of tie-break guard UNITS closed by
	// one final statement. A guard unit is either
	//   - a single `if xs[i].f != xs[j].f { return xs[i].f OP xs[j].f }`
	//     guard (one statement), or
	//   - a TWO-RETURN pair `if xs[i].f < xs[j].f { return true };
	//     if xs[i].f > xs[j].f { return false }` (two consecutive
	//     statements) — the `!=` guard in disguise, see ps3002TwoReturnPair.
	// Units of either kind mix freely. The final statement is `return A < B`
	// / `return A > B` (field or whole-element compare) or — only after at
	// least one guard unit — a bare `return false`, the canonical "all
	// fields equal → i does not sort before j" tail, which becomes
	// `return 0` in the cmp chain: sort.Slice treats a false comparator
	// result between equal elements exactly as SortFunc treats 0.
	// Direction is INDEPENDENT per field.
	body := fl.Body.List
	if len(body) == 0 {
		return nil, false
	}
	type fieldCmp struct {
		suffix     string
		descending bool
	}
	fields := make([]fieldCmp, 0, len(body))
	tailZero := false // body ends in a bare `return false` → emit `return 0`
	sawFinal := false
	for i := 0; i < len(body); {
		// (a) A two-return guard pair — tried first, but unambiguous either
		// way: its ifs test '<'/'>' and return bool LITERALS, a `!=` guard's
		// if tests '!=' and returns an ordering comparison.
		if i+1 < len(body) {
			if suffix, desc, ok := ps3002TwoReturnPair(pass, body[i], body[i+1], target, params[0], params[1]); ok {
				fields = append(fields, fieldCmp{suffix, desc})
				i += 2
				continue
			}
		}
		// (b) A single `!=` tie-break guard.
		if ifs, ok := body[i].(*ast.IfStmt); ok {
			if i == len(body)-1 {
				// A guard as the LAST statement leaves no final return.
				return nil, false
			}
			if ifs.Init != nil || ifs.Else != nil || len(ifs.Body.List) != 1 {
				return nil, false
			}
			condSuffix, ok := neqField(pass, ifs.Cond, target, params[0], params[1])
			if !ok {
				return nil, false
			}
			ret, ok := ifs.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				return nil, false
			}
			cmpSuffix, cmpDesc, ok := orderingCompare(pass, ret.Results[0], target, params[0], params[1])
			// The guard must test the very field it then orders (an empty
			// chain — a whole-element compare — never belongs in a guard):
			// otherwise the bool chain and the cmp chain diverge on ties. The
			// return's direction does not affect the guard: `!=` is symmetric.
			if !ok || cmpSuffix == "" || cmpSuffix != condSuffix {
				return nil, false
			}
			fields = append(fields, fieldCmp{cmpSuffix, cmpDesc})
			i++
			continue
		}
		// (c) The final statement — anything here must be the LAST one.
		if i != len(body)-1 {
			return nil, false
		}
		final, ok := body[i].(*ast.ReturnStmt)
		if !ok || len(final.Results) != 1 {
			return nil, false
		}
		if val, isLit := ps3002BoolLiteral(pass, final.Results[0]); isLit {
			// A bare `return false` after ≥1 guard unit is the "all fields
			// equal" tail → `return 0`. A `return true` tail (never a valid
			// strict-order comparator on ties) or a bool literal as the SOLE
			// statement (degenerate constant comparator) stays advisory.
			if val || len(fields) == 0 {
				return nil, false
			}
			tailZero = true
		} else {
			finalSuffix, finalDesc, ok := orderingCompare(pass, final.Results[0], target, params[0], params[1])
			if !ok {
				return nil, false
			}
			// A whole-element compare is only valid as the sole statement AND
			// ascending (→ slices.Sort, which has no comparator to swap
			// operands in); as the tail of a tie-break chain it compares the
			// whole element, which cmp.Compare cannot express.
			if finalSuffix == "" && (len(fields) > 0 || finalDesc) {
				return nil, false
			}
			fields = append(fields, fieldCmp{finalSuffix, finalDesc})
		}
		sawFinal = true
		i++
	}
	if !sawFinal {
		// All guard units, no final statement — cannot even compile as a
		// bool comparator, but never trust the input.
		return nil, false
	}
	// slices must be usable by name at the call site — required by BOTH
	// rewrites below. An existing import is reused under its local name
	// (alias included); a missing one is fine, runPS3002 adds the import
	// edit per file. Only a dot/blank import or a shadowing local keeps
	// the site advisory.
	slicesName, _, usable := ps3104SlicesName(pass, f, call.Pos())
	if !usable {
		return nil, false
	}

	// The rewrite splices the target's ORIGINAL source spelling back in as the
	// first argument — a pure ident/selector path renders identically to its
	// source text (no parens, no calls, no line breaks to lose).
	targetText := exprTextRendered(call.Args[0])

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
				{Pos: call.Pos(), End: call.End(), NewText: fmt.Appendf(nil, "%s.Sort(%s)", slicesName, targetText)},
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
	fmt.Fprintf(&b, "%s.%s(%s, func(a, b %s) int { ", slicesName, fn, targetText, elemStr)
	if tailZero {
		// The bare `return false` tail: EVERY collected field is a guard and
		// the tail becomes `return 0` — both say "equal", and sort.Slice's
		// false-on-equal is exactly SortFunc's 0-on-equal, so ties keep the
		// identical order under the shared pdqsort.
		for _, f := range fields {
			fmt.Fprintf(&b, "if a%s != b%s { return %s }; ", f.suffix, f.suffix, compare(f))
		}
		b.WriteString("return 0 })")
	} else {
		for _, f := range fields[:len(fields)-1] {
			fmt.Fprintf(&b, "if a%s != b%s { return %s }; ", f.suffix, f.suffix, compare(f))
		}
		fmt.Fprintf(&b, "return %s })", compare(fields[len(fields)-1]))
	}
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
func orderingCompare(pass *analysis.Pass, expr ast.Expr, target ps3002Path, iName, jName string) (suffix string, descending bool, ok bool) {
	bin, isBin := expr.(*ast.BinaryExpr)
	if !isBin || (bin.Op != token.LSS && bin.Op != token.GTR) {
		return "", false, false
	}
	suffix, ok = comparisonSuffix(pass, bin, target, iName, jName)
	return suffix, bin.Op == token.GTR, ok
}

// ps3002BoolLiteral reports whether e is the predeclared boolean constant
// true or false — the universe identifier itself, not a same-named local
// shadowing it — and which one. Both the two-return guard pairs and the bare
// `return false` tail hinge on the literal really being the untyped bool
// constant: a shadowed `false` could hold anything.
func ps3002BoolLiteral(pass *analysis.Pass, e ast.Expr) (val, ok bool) {
	id, isID := e.(*ast.Ident)
	if !isID || (id.Name != "true" && id.Name != "false") {
		return false, false
	}
	if pass.TypesInfo.ObjectOf(id) != types.Universe.Lookup(id.Name) {
		return false, false
	}
	return id.Name == "true", true
}

// ps3002PairHalf matches one half of a two-return guard pair: `if xs[i]<CHAIN>
// OP xs[j]<CHAIN> { return BOOL }` with OP '<' or '>' over a NON-EMPTY chain
// (a whole-element pair is rejected the same way an empty `!=` guard is:
// guards must name the field they order) and BOOL a bool literal, with no
// Init, no Else and a single-statement body. greater reports OP == '>' and
// retTrue the literal's value.
func ps3002PairHalf(pass *analysis.Pass, stmt ast.Stmt, target ps3002Path, iName, jName string) (suffix string, greater, retTrue, ok bool) {
	ifs, isIf := stmt.(*ast.IfStmt)
	if !isIf || ifs.Init != nil || ifs.Else != nil || len(ifs.Body.List) != 1 {
		return "", false, false, false
	}
	suffix, greater, condOK := orderingCompare(pass, ifs.Cond, target, iName, jName)
	if !condOK || suffix == "" {
		return "", false, false, false
	}
	ret, isRet := ifs.Body.List[0].(*ast.ReturnStmt)
	if !isRet || len(ret.Results) != 1 {
		return "", false, false, false
	}
	val, isLit := ps3002BoolLiteral(pass, ret.Results[0])
	if !isLit {
		return "", false, false, false
	}
	return suffix, greater, val, true
}

// ps3002TwoReturnPair matches the TWO-RETURN guard pair
//
//	if xs[i].f < xs[j].f { return true }
//	if xs[i].f > xs[j].f { return false }
//
// which is the `!=` tie-break guard in disguise: when the fields differ,
// exactly one of the two ifs fires and returns the '<' verdict; when they are
// equal, both fall through — precisely `if a.f != b.f { return a.f < b.f }`.
// The GTR-returns-true flip (`>` → true, `<` → false) is the descending form.
// A valid pair needs the SAME non-empty field chain in both conditions,
// exactly one '<' and one '>', and exactly one `return true` and one
// `return false`, with the operator of the true-returning if setting the
// direction (LSS+true → ascending, GTR+true → descending). Anything else —
// same operator twice, same boolean twice, `<=`/`>=`, mismatched fields — is
// a DIFFERENT predicate and keeps the site advisory.
func ps3002TwoReturnPair(pass *analysis.Pass, first, second ast.Stmt, target ps3002Path, iName, jName string) (suffix string, descending, ok bool) {
	s1, gt1, true1, ok1 := ps3002PairHalf(pass, first, target, iName, jName)
	if !ok1 {
		return "", false, false
	}
	s2, gt2, true2, ok2 := ps3002PairHalf(pass, second, target, iName, jName)
	if !ok2 || s1 != s2 || gt1 == gt2 || true1 == true2 {
		return "", false, false
	}
	// The if that returns true carries the direction; its partner is forced
	// to be the opposite operator returning false by the checks above.
	if true1 {
		return s1, gt1, true
	}
	return s1, gt2, true
}

// neqField validates a tie-break guard condition `xs[i]<CHAIN> !=
// xs[j]<CHAIN>` under the same operand rules as orderingCompare and
// returns the same "."-prefixed selector suffix. `!=` is symmetric, so the
// guard is direction-agnostic: it pairs equally with an ascending or a
// descending return comparison on the same field.
func neqField(pass *analysis.Pass, expr ast.Expr, target ps3002Path, iName, jName string) (suffix string, ok bool) {
	bin, isBin := expr.(*ast.BinaryExpr)
	if !isBin || bin.Op != token.NEQ {
		return "", false
	}
	return comparisonSuffix(pass, bin, target, iName, jName)
}

// comparisonSuffix validates a binary comparison (the operator itself was
// already vetted by the caller) whose operands are identical selector chains
// rooted at target[iName] (left) and target[jName] (right) over an ordered
// basic type, and returns the selector chain as a "."-prefixed suffix (""
// for the empty whole-element chain). Floats are rejected: cmp.Compare and
// slices.Sort order NaN as the smallest value, whereas the '<'/'>'
// comparators treat NaN as incomparable, so the two disagree on any slice
// that can contain a NaN — the same float exclusion PS3104/PS3105 apply for
// exactly this reason.
func comparisonSuffix(pass *analysis.Pass, bin *ast.BinaryExpr, target ps3002Path, iName, jName string) (string, bool) {
	fieldsA, baseA, idxA, ok := indexSelectorChain(bin.X)
	if !ok || idxA.Name != iName {
		return "", false
	}
	fieldsB, baseB, idxB, ok := indexSelectorChain(bin.Y)
	if !ok || idxB.Name != jName {
		return "", false
	}
	// Both chains index the SAME path as the first argument — same root
	// object AND same field-name sequence: a comparator indexing a different
	// slice (or a different field of the same struct) than the one being
	// sorted would be dropped by the rewrite, so it must stay advisory.
	if !ps3002SamePath(pass, baseA, target) || !ps3002SamePath(pass, baseB, target) {
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
// returning the selector names AFTER the index (outermost last), the indexed
// base expression (validated as a path by the caller via ps3002SamePath) and
// the index identifier. Anything else — parens, calls, a non-identifier
// index, nested indexes after the base — does not match.
func indexSelectorChain(e ast.Expr) (fields []string, base ast.Expr, index *ast.Ident, ok bool) {
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
	index, okIdx := ix.Index.(*ast.Ident)
	if !okIdx {
		return nil, nil, nil, false
	}
	return fields, ix.X, index, true
}

// ps3002Path identifies the sorted target: the root identifier's object plus
// the selector field names (outermost last) leading from it to the slice —
// `c.Kinds` is {obj(c), ["Kinds"]}, a plain `xs` is {obj(xs), nil}.
type ps3002Path struct {
	obj    types.Object
	fields []string
}

// ps3002SamePath reports whether e spells exactly the target path: a
// side-effect-free ident/selector chain whose root resolves to the same
// types.Object and whose field-name sequence is identical. Resolving the
// root through the type checker (not by name) keeps a shadowed name from
// matching.
func ps3002SamePath(pass *analysis.Pass, e ast.Expr, target ps3002Path) bool {
	root, fields, ok := pathExpr(e)
	if !ok || pass.TypesInfo.ObjectOf(root) != target.obj {
		return false
	}
	if len(fields) != len(target.fields) {
		return false
	}
	for i := range fields {
		if fields[i] != target.fields[i] {
			return false
		}
	}
	return true
}

// pathExpr matches a SIDE-EFFECT-FREE PATH — a plain identifier or a selector
// chain of identifiers rooted at one (`xs`, `c.Kinds`, `s.inner.items`) —
// and returns the root identifier plus the field names (outermost last, the
// same convention as indexSelectorChain). Field access through a pointer is
// fine: it is still just an *ast.SelectorExpr (Go auto-derefs) with no side
// effect and no re-evaluation hazard. Any other node — a call, an index
// expression, an explicit `*` dereference, parentheses — could re-evaluate
// with side effects or yield a different value, so ok is false.
func pathExpr(e ast.Expr) (root *ast.Ident, fields []string, ok bool) {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x, fields, true
		case *ast.SelectorExpr:
			fields = append([]string{x.Sel.Name}, fields...)
			e = x.X
		default:
			return nil, nil, false
		}
	}
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
