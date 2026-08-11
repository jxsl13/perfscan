package checks

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// ps3103Threshold is the element size, in bytes, above which a by-value
// range copy is reported. 128 bytes (two cache lines) matches go-critic's
// rangeValCopy default: below it the per-iteration copy is a handful of
// MOVs the CPU hides behind the loads, and reporting it would be noise.
const ps3103Threshold = 128

// PS3103 reports `for _, v := range xs` over slices and arrays whose
// element type is large: every iteration copies the whole element into v.
// ADVISORY ONLY — the remedy turns the copy into an alias, which is not a
// behavior-preserving mechanical rewrite (see Doc).
var PS3103 = register(&lint.Check{
	ID:       "PS3103",
	Category: "indirect",
	Slug:     "range-value-copy",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "ranging by value over large elements copies each one",
		Text: `for _, v := range xs assigns a fresh copy of xs[i] to v on
every iteration. For a large element type that copy is real work: memmove
traffic that scales with len(xs)*sizeof(elem), competing with the loop
body for memory bandwidth. Elements larger than 128 bytes (two cache
lines, go-critic's rangeValCopy default) are reported; only slices,
arrays, and pointers-to-array are considered, because only they have
addressable elements with an indexable remedy. Map and channel ranges
are excluded: map elements are not addressable and a channel receive
must copy, so no cheaper access exists. A value variable the body never
reads is also excluded (only expressible in the 'for _, v = range'
assignment form; ':=' with an unused v does not compile): such a loop
only defines v's final value, so the per-iteration aliasing remedy does
not apply there.

The manual remedy is to iterate by index and read through a pointer:

	for i := range xs {
		v := &xs[i]
		... v.Field ...
	}

NO AUTOMATIC FIX IS ATTACHED, deliberately. The rewrite changes v from
an independent copy into an alias of xs[i], and that is observable, not
cosmetic: any write to v (or through v's fields) now mutates the slice;
&v (or a closure capturing v) now escapes a pointer into the backing
array, pinning it and seeing later mutations; appending v to another
collection stores an alias where a value was stored before; and with
Go 1.22 per-iteration variables, goroutines capturing v saw stable
copies that would become racy shared pointers. Proving all of v's uses
are pure reads that never let v or its address escape is whole-body
flow analysis, and even then the pointer-chasing form can lose to the
copy for elements near the threshold. The change is a short, easy edit
for a human who can see how v is used — exactly the kind of judgment an
advisory is for.`,
		Before: `for _, v := range rows { // copies sizeof(row) bytes per iteration
	total += v.Sum
}`,
		After: `for i := range rows { // reads through a pointer; v now ALIASES rows[i]
	v := &rows[i]
	total += v.Sum
}`,
		MeasuredWin: `BenchmarkPS3103 (256 elements of a 2056-byte struct,
three fields read per iteration, Apple M2 Pro): 8.89 µs/op -> 0.17 µs/op
(~52x) — the by-value loop memmoves ~514 KiB per pass that the indexed
loop never touches. Zero allocations either way; the win scales with
element size and evaporates for small elements, hence the 128-byte gate.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3103",
		Doc:  "range by value copies large elements every iteration",
		Run:  runPS3103,
	},
})

func runPS3103(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			rng, ok := n.(*ast.RangeStmt)
			if !ok {
				return true
			}
			val, ok := rng.Value.(*ast.Ident)
			if !ok || val.Name == "_" {
				return true
			}
			elem, ok := ps3103IndexableElem(pass.TypesInfo.TypeOf(rng.X))
			if !ok || ps3103HasTypeParam(elem, nil) {
				return true
			}
			size := pass.TypesSizes.Sizeof(elem)
			if size <= ps3103Threshold {
				return true
			}
			// The value variable's object: Defs for `:=`, Uses for the
			// `for _, v = range` assignment form.
			obj := pass.TypesInfo.Defs[val]
			if obj == nil {
				obj = pass.TypesInfo.Uses[val]
			}
			if obj == nil || !ps3103BodyUses(pass, rng.Body, obj) {
				return true // dead copy; remedy is dropping v, not aliasing
			}
			pass.Report(analysis.Diagnostic{
				Pos: val.Pos(),
				End: val.End(),
				Message: fmt.Sprintf(
					"ranging by value copies a %d-byte %s element on every iteration; iterate by index and take a pointer to the element where aliasing is safe",
					size, types.TypeString(elem, types.RelativeTo(pass.Pkg))),
			})
			return true
		})
	}
	return nil, nil
}

// ps3103IndexableElem returns the element type when t is a slice, array,
// or pointer-to-array — the ranges whose elements are addressable, so the
// indexed &xs[i] remedy exists. Maps (elements not addressable), channels
// (a receive must copy), strings, ints, and iterator funcs are excluded.
func ps3103IndexableElem(t types.Type) (types.Type, bool) {
	if t == nil {
		return nil, false
	}
	switch u := t.Underlying().(type) {
	case *types.Slice:
		return u.Elem(), true
	case *types.Array:
		return u.Elem(), true
	case *types.Pointer:
		if a, ok := u.Elem().Underlying().(*types.Array); ok {
			return a.Elem(), true
		}
	}
	return nil, false
}

// ps3103BodyUses reports whether body contains a use of obj. A value
// variable that is never read has no live copy after dead-code
// elimination, so reporting it as a copy cost would be dishonest.
func ps3103BodyUses(pass *analysis.Pass, body *ast.BlockStmt, obj types.Object) bool {
	if body == nil {
		return false
	}
	used := false
	ast.Inspect(body, func(n ast.Node) bool {
		if used {
			return false
		}
		if id, ok := n.(*ast.Ident); ok && pass.TypesInfo.Uses[id] == obj {
			used = true
		}
		return !used
	})
	return used
}

// ps3103HasTypeParam reports whether t contains a free type parameter
// anywhere in its structure. Sizeof over a type parameter is meaningless
// (the size differs per instantiation), so generic element types are
// skipped rather than judged against the threshold.
func ps3103HasTypeParam(t types.Type, seen map[types.Type]bool) bool {
	if t == nil {
		return false
	}
	if seen[t] {
		return false
	}
	if seen == nil {
		seen = map[types.Type]bool{}
	}
	seen[t] = true
	switch u := t.(type) {
	case *types.TypeParam:
		return true
	case *types.Named:
		if args := u.TypeArgs(); args != nil {
			for i := range args.Len() {
				if ps3103HasTypeParam(args.At(i), seen) {
					return true
				}
			}
		}
		return ps3103HasTypeParam(u.Underlying(), seen)
	case *types.Alias:
		return ps3103HasTypeParam(types.Unalias(u), seen)
	case *types.Pointer:
		return ps3103HasTypeParam(u.Elem(), seen)
	case *types.Slice:
		return ps3103HasTypeParam(u.Elem(), seen)
	case *types.Array:
		return ps3103HasTypeParam(u.Elem(), seen)
	case *types.Map:
		return ps3103HasTypeParam(u.Key(), seen) || ps3103HasTypeParam(u.Elem(), seen)
	case *types.Chan:
		return ps3103HasTypeParam(u.Elem(), seen)
	case *types.Struct:
		for i := range u.NumFields() {
			if ps3103HasTypeParam(u.Field(i).Type(), seen) {
				return true
			}
		}
	}
	return false
}
