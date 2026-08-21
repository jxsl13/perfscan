package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5083 removes standard-library clone chains consumed only by the len
// builtin. All supported Clone functions preserve length exactly.
var PS5083 = register(&lint.Check{
	ID:       "PS5083",
	Category: "alloc",
	Slug:     "len-of-stdlib-clone",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "len is applied to throwaway standard-library clones",
		Text: `bytes.Clone, slices.Clone, maps.Clone, and strings.Clone all
preserve length exactly. Cloning a value only to ask for that length turns an
O(1) header read into an O(n) allocation and copy:

  len(bytes.Clone(data))                           -> len(data)
  len(slices.Clone(values))                        -> len(values)
  len(maps.Clone(index))                           -> len(index)
  len(strings.Clone(text))                         -> len(text)
  len(bytes.Clone(slices.Clone(bytes.Clone(data)))) -> len(data)
  len(bytes.Clone([]byte(strings.Clone(text))))     -> len(text)

This check resolves the outer len builtin and every Clone layer through
go/types. The shared typed unary-chain matcher supports arbitrarily deep,
heterogeneous bytes/slices chains and explicit generic instantiation while
rejecting shadowed len identifiers, dot imports, methods named Clone,
ellipsis, and type-changing wrappers.

The rewrite is BIT-IDENTICAL for race-free Go programs. Every supported Clone
preserves nil length, empty length, string byte length, slice length, and map
entry count. The base expression is still evaluated exactly once in its
original position; only allocation/copy scaffolding disappears. cap is not
matched because slices.Clone and bytes.Clone intentionally change capacity.
Clone results used outside the len call are also untouched.

The terminal reducer also composes the independently safe byte-length
identity len([]byte(s)) == len(s). A byte Clone chain around an unnamed
[]byte conversion, including nested strings.Clone layers, is collapsed to the
original non-constant string in the SAME fix. This avoids overlapping PS5084
and PS2125 diagnostics and reaches the O(1) length read in one pass. A string
constant deliberately keeps the conversion: removing it would turn a runtime
len expression into a compile-time constant and can invalidate a switch with
duplicate cases.

Comments keep the finding advisory. The shared multi-package deletion engine
removes all newly unused clone imports—including adjacent aliases in one import
block—and withholds a fix for cgo imports, commented imports, overlapping
semantic uses, or unsafe local-declaration fallout.`,
		Before: `size := len(bytes.Clone(
	slices.Clone(bytes.Clone(payload)),
))
textSize := len(bytes.Clone([]byte(strings.Clone(text))))`,
		After: `size := len(payload)
textSize := len(text)`,
		MeasuredWin: `On Apple M2 Pro, len over three Clone layers on a
65,527-byte slice measured 11532 ns/op, 196608 B/op, 3 allocs/op -> 0.294
ns/op, 0 B/op, 0 allocs/op (median of five runs): about 39,300x faster,
eliminating all three full-slice copies and reducing the operation to an O(1)
length load.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5083",
		Doc:  "len consumes throwaway bytes/slices/maps/strings Clone chains",
		Run:  runPS5083,
	},
})

func runPS5083(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			lengthCall, ok := node.(*ast.CallExpr)
			if !ok || len(lengthCall.Args) != 1 || lengthCall.Ellipsis.IsValid() || !ps5083LenBuiltin(pass, lengthCall.Fun) {
				return true
			}
			matched, ok := matchTypedUnaryPackageCallChain(pass, lengthCall.Args[0], isTypedStdlibClone)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     lengthCall.Pos(),
				End:     lengthCall.End(),
				Message: fmt.Sprintf("len consumes %d throwaway standard-library Clone layer(s); read the original value's identical length directly", len(matched.calls)),
			}
			if fix, ok := ps5083FixedPointFix(pass, file, lengthCall, matched); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			} else if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, matched.paths, "remove clones before len", matched.spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5083LenBuiltin(pass *analysis.Pass, expr ast.Expr) bool {
	return typedBuiltinName(pass, expr, "len")
}

type ps5083FixedPoint struct {
	conversion *ast.CallExpr
	kept       ast.Expr
	paths      []string
	nested     typedUnaryCallChain
	nestedOK   bool
}

func ps5083FixedPointMatch(pass *analysis.Pass, lengthCall *ast.CallExpr, chain typedUnaryCallChain) (ps5083FixedPoint, bool) {
	if !ps5083LenBuiltin(pass, lengthCall.Fun) {
		return ps5083FixedPoint{}, false
	}
	conversion, ok := ps5084StringByteConversion(pass, chain.base)
	if !ok {
		return ps5083FixedPoint{}, false
	}
	kept := conversion.Args[0]
	paths := make([]string, len(chain.paths))
	copy(paths, chain.paths)
	if path := ps5084TypeQualifierPath(pass, conversion.Fun); path != "" {
		paths = append(paths, path)
	}
	nested, nestedOK := matchTypedUnaryPackageCallChain(pass, kept, isTypedStringStdlibClone)
	if nestedOK {
		kept = nested.base
		paths = append(paths, nested.paths...)
	}
	typedValue, ok := pass.TypesInfo.Types[kept]
	if !ok || typedValue.Type == nil || typedValue.Value != nil || !ps5084StringType(typedValue.Type) {
		return ps5083FixedPoint{}, false
	}
	return ps5083FixedPoint{conversion: conversion, kept: kept, paths: paths, nested: nested, nestedOK: nestedOK}, true
}

func ps5083FixedPointFix(pass *analysis.Pass, file *ast.File, lengthCall *ast.CallExpr, chain typedUnaryCallChain) (analysis.SuggestedFix, bool) {
	fixedPoint, ok := ps5083FixedPointMatch(pass, lengthCall, chain)
	if !ok {
		return analysis.SuggestedFix{}, false
	}
	outerClone := chain.calls[0]
	return fixReplacedCallScaffoldingPaths(pass, file, fixedPoint.paths, "remove clones and the redundant []byte conversion before len",
		analysis.TextEdit{Pos: outerClone.Pos(), End: fixedPoint.kept.Pos()},
		analysis.TextEdit{Pos: fixedPoint.kept.End(), End: outerClone.End()},
	)
}
