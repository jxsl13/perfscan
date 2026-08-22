package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5072 reports binary.LittleEndian.Uint64(h.Sum(nil)) for a
// hash/maphash.Hash. Hash.Sum materializes Sum64 in little-endian form solely
// to satisfy hash.Hash; decoding those bytes immediately reconstructs the
// value Hash.Sum64 already returned.
var PS5072 = register(&lint.Check{
	ID:       "PS5072",
	Category: "alloc",
	Slug:     "maphash-sum-decode-to-sum64",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "binary.LittleEndian.Uint64(h.Sum(nil)) rebuilds the value hash/maphash.Hash.Sum64 already computed",
		Text: `hash/maphash.Hash.Sum exists to implement hash.Hash. It first calls
Hash.Sum64, then appends that uint64 to a byte slice in little-endian order.
Passing the resulting slice straight to binary.LittleEndian.Uint64 reverses
that encoding and reconstructs the original uint64:

  binary.LittleEndian.Uint64(h.Sum(nil)) -> h.Sum64()

The direct method avoids the temporary eight-byte slice, eight byte stores,
and the matching decode. It is also the API the hash/maphash documentation
explicitly recommends: "For direct calls, it is more efficient to use
Hash.Sum64."

The rewrite is BIT-IDENTICAL for every hash state. Hash.Sum(nil) is defined by
the current standard-library implementation as append(nil, byte(x>>0), ...,
byte(x>>56)), where x is the result of h.Sum64. LittleEndian.Uint64 of those
eight bytes is exactly x. Both forms evaluate the receiver once, do not mutate
the hash, return the same uint64 type, and call Sum64 exactly once. A nil
*maphash.Hash panics inside Sum64 in both forms.

The match is deliberately exact. The outer call must be the Uint64 method on
the standard library's encoding/binary.LittleEndian value, the inner call must
be the Sum method declared on the standard library's hash/maphash.Hash, and its
sole argument must be the predeclared nil. Type information rejects BigEndian,
a runtime binary.ByteOrder, another hash.Hash implementation, an appended
prefix, shadowed package names, and same-named user methods. The receiver's
static type must be exactly hash/maphash.Hash or *hash/maphash.Hash so that the
replacement cannot be intercepted by a wrapper's own Sum64 method.

The fix preserves the receiver expression byte-for-byte and changes only the
surrounding call chain to .Sum64(). A comment inside removed scaffolding keeps
the report advisory. If the rewrite removes the file's last encoding/binary
reference, one file-wide suggestion rewrites every such site and removes that
import atomically. This keeps each editor quick-fix independently compilable.
In a cgo file, whose import layout is left untouched, those final-reference
fixes remain advisory.`,
		Before: `sum := binary.LittleEndian.Uint64(h.Sum(nil))`,
		After:  `sum := h.Sum64()`,
		MeasuredWin: `BenchmarkPS5072 (Apple M2 Pro, go1.26; five runs):
binary.LittleEndian.Uint64(h.Sum(nil)) median 16.35 ns/op, 8 B/op,
1 alloc/op -> h.Sum64() median 3.34 ns/op, 0 B/op, 0 allocs/op (~4.9x).
The direct call removes the temporary little-endian byte append, its heap
allocation, and the matching Uint64 decode.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5072",
		Doc:  "binary.LittleEndian.Uint64 over hash/maphash.Hash.Sum(nil) instead of direct Sum64",
		Run:  runPS5072,
	},
})

var ps5072Chain = []typedCallStep{
	{PkgPath: "encoding/binary", Name: "Uint64", Kind: typedMethod, Arity: 1, NextArg: 0},
	{PkgPath: "hash/maphash", Name: "Sum", Kind: typedMethod, Arity: 1, NextArg: -1},
}

func runPS5072(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		type site struct {
			outer *ast.CallExpr
			fix   *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0

		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			calls, ok := matchTypedCallChain(pass, outer, ps5072Chain...)
			if !ok || !ps5072IsLittleEndian(pass, calls[0]) {
				return true
			}
			inner := calls[1]
			if !ps5072Nil(pass, inner.Args[0]) {
				return true
			}
			recv, ok := ps5072MaphashReceiver(pass, inner)
			if !ok {
				return true
			}

			var fix *analysis.SuggestedFix
			if !ps2111CommentIn(file, outer.Pos(), recv.Pos()) &&
				!ps2111CommentIn(file, recv.End(), outer.End()) {
				fix = &analysis.SuggestedFix{
					Message: "call hash/maphash.Hash.Sum64 directly",
					TextEdits: []analysis.TextEdit{
						{Pos: outer.Pos(), End: recv.Pos()},
						{Pos: recv.End(), End: outer.End(), NewText: []byte(".Sum64()")},
					},
				}
				fixable++
			}
			sites = append(sites, site{outer: outer, fix: fix})
			return true
		})

		if len(sites) == 0 {
			continue
		}
		dropBinary := fixable > 0 && pkgRefCount(pass, file, "encoding/binary") == fixable
		if dropBinary {
			if ps2110ImportsC(file) {
				for i := range sites {
					sites[i].fix = nil
				}
			} else if edit, ok := dropImportEdit(file, "encoding/binary"); ok {
				grouped := analysis.SuggestedFix{
					Message: "call hash/maphash.Hash.Sum64 directly at every site and remove the unused encoding/binary import",
				}
				firstFixable := -1
				for i := range sites {
					if sites[i].fix != nil {
						if firstFixable < 0 {
							firstFixable = i
						}
						grouped.TextEdits = append(grouped.TextEdits, sites[i].fix.TextEdits...)
						sites[i].fix = nil
					}
				}
				grouped.TextEdits = append(grouped.TextEdits, edit)
				sites[firstFixable].fix = &grouped
			} else {
				for i := range sites {
					sites[i].fix = nil
				}
			}
		}

		for _, matched := range sites {
			diagnostic := analysis.Diagnostic{
				Pos:     matched.outer.Pos(),
				End:     matched.outer.End(),
				Message: "binary.LittleEndian.Uint64(h.Sum(nil)) makes and decodes an 8-byte representation of the hash/maphash value that h.Sum64() returns directly",
			}
			if matched.fix != nil {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{*matched.fix}
			}
			pass.Report(diagnostic)
		}
	}
	return nil, nil
}

// ps5072IsLittleEndian pins the receiver of the outer Uint64 method to the
// package variable encoding/binary.LittleEndian. Merely matching a ByteOrder
// method would be unsafe because a runtime value could hold BigEndian.
func ps5072IsLittleEndian(pass *analysis.Pass, call *ast.CallExpr) bool {
	fun, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	recv, ok := ps2110Unparen(fun.X).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	obj, ok := pass.TypesInfo.Uses[recv.Sel].(*types.Var)
	return ok && obj.Pkg() != nil && obj.Pkg().Path() == "encoding/binary" && obj.Name() == "LittleEndian"
}

func ps5072Nil(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[ps2110Unparen(expr)]
	if !ok || tv.Type == nil {
		return false
	}
	basic, ok := tv.Type.(*types.Basic)
	return ok && basic.Kind() == types.UntypedNil
}

// ps5072MaphashReceiver returns the receiver of Hash.Sum only when its static
// type is exactly hash/maphash.Hash or *hash/maphash.Hash. Excluding embedded
// wrappers prevents a wrapper-defined Sum64 from intercepting the replacement.
func ps5072MaphashReceiver(pass *analysis.Pass, call *ast.CallExpr) (ast.Expr, bool) {
	fun, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	// Keep the receiver's original source extent for the fix. Parentheses may
	// be required for selector syntax, as in (*p).Sum64().
	recv := fun.X
	t := types.Unalias(pass.TypesInfo.TypeOf(ps2110Unparen(recv)))
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "hash/maphash" || named.Obj().Name() != "Hash" {
		return nil, false
	}
	return recv, true
}
