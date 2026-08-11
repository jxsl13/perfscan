package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"go/version"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3104 reports sort.Ints(x) / sort.Strings(x) — the legacy concrete
// sort helpers — and rewrites them to the generic slices.Sort(x).
var PS3104 = register(&lint.Check{
	ID:       "PS3104",
	Category: "indirect",
	Slug:     "sort-helper-to-slices-sort",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "sort.Ints/sort.Strings are the legacy spelling of slices.Sort; call the generic sort directly",
		Text: `On a go1.21 toolchain sort.Ints and sort.Strings wrap the slice
in sort.IntSlice / sort.StringSlice and sort it through the three-method
sort.Interface — an interface dispatch on every comparison and swap —
while the generic slices.Sort (Go 1.21) is a pdqsort specialized for the
concrete element type at compile time. Since go1.22 the standard library
has conceded the point: sort.Ints and sort.Strings ARE one-line wrappers
calling slices.Sort (their doc comments say so), so there the rewrite is
performance parity minus one call hop, and the win is directness —
calling the modern API instead of its legacy shim.

The result is BIT-IDENTICAL. Both spellings sort ascending (< on int;
lexicographic byte order on string), and for int and string any two
elements that compare equal are bitwise-identical values — so the sorted
output is the unique ascending arrangement of the input multiset, the
same bytes no matter which algorithm produced it, stability included.
Both helpers return nothing, so only a call statement is rewritten.

sort.Float64s is deliberately EXCLUDED and never flagged. Its contract
orders NaNs first, and float64 has DISTINGUISHABLE ties: -0.0 and +0.0
compare equal but differ in bits, as do NaNs with different payloads —
and both sorts are unstable, so two correct implementations may arrange
those ties differently. The uniqueness argument above collapses and the
rewrite is not guaranteed bit-identical, so perfscan does not offer it.

The callee is resolved with type information — only the package functions
sort.Ints/sort.Strings match, never a shadowed sort, a same-named local,
or a method — and only a plain call statement is rewritten. The argument
expression is kept verbatim in place. The fix edits imports as needed:
the slices import is added when missing (reusing an existing alias when
the file imports slices under another name), and the sort import is
dropped when the rewrites remove the file's LAST sort reference — when
they replace the whole import, the sort spec is swapped for "slices" in
place. A dot- or blank-imported slices, a comment inside the rewritten
call punctuation, or a cgo file (whose import block must not be edited)
keeps the report advisory.

The report only fires when the effective language version is at least
go1.21 — below that slices.Sort does not exist and the advice would be
moot. Note that per-file version pinning itself only exists from go1.21
on, so go/types clamps a //go:build go1.20 (or older) line to go1.21 and
such files are still flagged; in practice the gate blocks via the
module's go directive (pass.Pkg.GoVersion()) when a whole module still
declares go < 1.21.`,
		Before: `sort.Ints(xs)`,
		After:  `slices.Sort(xs)`,
		MeasuredWin: `BenchmarkPS3104 (a shuffled 10k-element []int copied and
sorted per op, Apple M2 Pro, go1.26): parity — 392 µs/op before vs
392 µs/op after, 0 allocs either way. Expected: since go1.22 sort.Ints IS
a one-line wrapper over slices.Sort, so the modern toolchain pair
measures the same code. The performance win is real only on a go1.21
toolchain, where sort.Ints still paid an interface dispatch per
comparison and swap; from go1.22 on the win is directness — the modern
API without the legacy hop.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3104",
		Doc:  "sort.Ints/sort.Strings legacy helpers instead of the generic slices.Sort",
		Run:  runPS3104,
	},
})

// ps3104Targets maps the matched sort helper to its element spelling (for
// the message). sort.Float64s is deliberately absent: float64 has
// distinguishable ties (-0.0/+0.0, NaN payloads) that an unstable sort may
// arrange differently, so that rewrite is not guaranteed bit-identical.
var ps3104Targets = map[string]string{
	"Ints":    "[]int",
	"Strings": "[]string",
}

func runPS3104(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps3104SlicesSortAvailable(pass, f) {
			// slices.Sort exists only from go1.21 on; below that the
			// advice is moot, so stay silent entirely (same policy as
			// PS2119's SplitSeq gate).
			continue
		}
		// Collect first, decide import edits once per file: every fixable
		// call removes one sort reference, and whether the sort import is
		// orphaned depends on ALL of them together (same per-file site
		// collection as PS2118/PS3002).
		type site struct {
			call *ast.CallExpr
			name string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
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
			// Type info pins the callee to the package function sort.Ints
			// or sort.Strings: a shadowed sort resolves sel.Sel to some
			// other object, and a method carries a receiver. The name
			// allowlist keeps sort.Float64s (NaN ordering differs) and
			// everything else out.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "sort" {
				return true
			}
			if _, wanted := ps3104Targets[fn.Name()]; !wanted {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			fix := ps3104Fix(pass, f, call, call.Args[0])
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fn.Name(), fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call holds exactly one sort reference (its
		// selector's package identifier); when those are ALL of the file's
		// sort references, the rewrites orphan the import and the fix must
		// drop it (the runner never prunes imports itself). The slices
		// import is needed when the FILE lacks one — a site-local shadow
		// only suppresses that site's fix, never the file's import edit.
		slicesImported := false
		for _, imp := range f.Imports {
			if imp.Path.Value == `"slices"` {
				slicesImported = true
				break
			}
		}
		needImport := fixable > 0 && !slicesImported
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
			// PS2110's importAdded map).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			elem := ps3104Targets[st.name]
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "sort." + st.name + " is the legacy spelling of slices.Sort (an interface-dispatch sort on go1.21, a one-line wrapper since go1.22); slices.Sort sorts the concrete " + elem + " directly with the identical ascending order",
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3104Fix builds the slices.Sort(arg) rewrite for one call, or nil when a
// guard fails and the report must stay advisory. Only the punctuation around
// the argument is replaced; the argument expression stays untouched in
// place, preserving its text and single evaluation (same technique as
// PS2110/PS2112). Import edits are appended later, once per file.
func ps3104Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, arg ast.Expr) *analysis.SuggestedFix {
	slicesName, _, usable := ps3104SlicesName(pass, f, call.Pos())
	if !usable {
		return nil
	}
	// The replaced spans are the call text around the argument; a comment
	// there would be silently destroyed — advisory then.
	if ps2111CommentIn(f, call.Pos(), arg.Pos()) || ps2111CommentIn(f, arg.End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + slicesName + ".Sort(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: arg.Pos(), NewText: []byte(slicesName + ".Sort(")},
			{Pos: arg.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

// ps3104SlicesName resolves how the fix must spell the slices package at
// pos: the file's existing import name (alias included) when slices is
// already imported, or the bare name "slices" with needImport set when the
// file must add the import. usable is false when the name cannot be used —
// a dot or blank slices import, or another object owning the name at pos.
func ps3104SlicesName(pass *analysis.Pass, f *ast.File, pos token.Pos) (name string, needImport, usable bool) {
	for _, imp := range f.Imports {
		if imp.Path.Value != `"slices"` {
			continue
		}
		local := "slices"
		if imp.Name != nil {
			local = imp.Name.Name
		}
		if local == "_" || local == "." {
			// Blank import gives no usable name; a dot import puts Sort in
			// scope unqualified but rewriting to a bare Sort(x) is too
			// fragile — advisory in both cases.
			return "", false, false
		}
		nf, ok := ps2110PkgUsable(pass, pos, local, "slices")
		return local, false, ok && !nf
	}
	needImport, usable = ps2110PkgUsable(pass, pos, "slices", "slices")
	return "slices", needImport, usable && needImport
}

// ps3104ImportEdits returns the import-block edits a file's combined fixes
// need: adding "slices" when missing (via the shared sorted-position insert
// PS2110/PS2112 use) and dropping the file's sort import when the rewrites
// orphan it. When both apply, the sort spec is replaced by "slices" in
// place — one edit, no overlap. ok is false when the import block must not
// or cannot be edited (cgo file, missing sort spec).
func ps3104ImportEdits(f *ast.File, needSlices, dropSort bool) (edits []analysis.TextEdit, ok bool) {
	if !needSlices && !dropSort {
		return nil, true
	}
	if ps2110ImportsC(f) {
		return nil, false
	}
	if !dropSort {
		return []analysis.TextEdit{ps2110ImportEdit(f, "slices")}, true
	}
	edit, found := ps3104SortImportEdit(f, needSlices)
	if !found {
		return nil, false
	}
	return []analysis.TextEdit{edit}, true
}

// ps3104SortImportEdit returns the edit removing the file's sort import
// spec — or, when replaceWithSlices is set, swapping the whole spec (alias
// included) for "slices" in place, which keeps a gofmt-sorted group sorted
// since no std path orders between "slices" and "sort".
func ps3104SortImportEdit(f *ast.File, replaceWithSlices bool) (analysis.TextEdit, bool) {
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for i, spec := range gd.Specs {
			is, ok := spec.(*ast.ImportSpec)
			if !ok || is.Path == nil || is.Path.Value != `"sort"` {
				continue
			}
			if replaceWithSlices {
				return analysis.TextEdit{Pos: is.Pos(), End: is.End(), NewText: []byte(`"slices"`)}, true
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

// ps3104SlicesSortAvailable reports whether f's effective language version
// has the generic slices.Sort (go1.21). The per-file version (a //go:build
// go1.N line moves it, up or down) wins over the package's. An unknown or
// unparseable version does not block the check: perfscan itself requires a
// far newer toolchain, so an empty version means "module default", not
// "ancient" — the same policy as PS2119's SplitSeq gate.
func ps3104SlicesSortAvailable(pass *analysis.Pass, f *ast.File) bool {
	v := ""
	if pass.TypesInfo.FileVersions != nil {
		v = pass.TypesInfo.FileVersions[f]
	}
	if v == "" && pass.Pkg != nil {
		v = pass.Pkg.GoVersion()
	}
	if v == "" {
		return true
	}
	lang := version.Lang(v)
	if lang == "" {
		return true
	}
	return version.Compare(lang, "go1.21") >= 0
}
