package checks

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5084 removes standard-library clones consumed immediately by a builtin
// that already copies the cloned values and cannot expose the clone itself.
var PS5084 = register(&lint.Check{
	ID:       "PS5084",
	Category: "alloc",
	Slug:     "clone-fed-copying-builtin",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a copying builtin consumes throwaway standard-library clones",
		Text: `bytes.Clone, slices.Clone, and strings.Clone are wasted when their
only consumer immediately copies the same values again:

  copy(dst, bytes.Clone(src))                    -> copy(dst, src)
  append(dst, slices.Clone(src)...)              -> append(dst, src...)
  string(bytes.Clone(data))                      -> string(data)
  []byte(strings.Clone(text))                    -> []byte(text)
  []rune(strings.Clone(text))                    -> []rune(text)
  copy(dst, bytes.Clone(slices.Clone(src)))       -> copy(dst, src)
  copy(dst, bytes.Clone([]byte(strings.Clone(s)))) -> copy(dst, s)
  append(dst, bytes.Clone([]byte(s))...)          -> append(dst, s...)
  string(bytes.Clone([]byte(strings.Clone(s))))    -> s

This check uses the shared typed unary-call-chain matcher to unwrap arbitrarily
deep heterogeneous bytes.Clone/slices.Clone chains from copy's source and
append's expanded source, or strings.Clone chains from copying string-to-slice
conversions. The reverse slice-to-string conversion accepts byte and rune
slices. Callees and builtins resolve through go/types, so aliases and explicit
generic instantiations work while shadowed copy/append functions, dot imports,
user Clone methods, non-expanded append arguments, map clones, destination
clones, identity conversions, and type-changing wrappers stay untouched.

The rewrite is BIT-IDENTICAL for race-free Go programs, including overlapping
slices. The Go specification states that both copy and append produce results
independent of whether argument memory overlaps; removing a temporary snapshot
therefore preserves element values, write count, append result length/capacity,
destination mutations, evaluation order, and panics. Converting a string to a
byte/rune slice or such a slice to a string already creates the independent
value promised by the removed Clone. Nil and empty sources, invalid UTF-8,
invalid runes, subslices, and named slice types retain their conversion
behavior.

When the clone chain wraps an unnamed []byte conversion of a string, the
terminal reducer composes PS5020/PS5021/PS2108's independently safe identities
in the SAME diagnostic: copy accepts a string source directly, append has its
string-spread special form, and string([]byte(s)) equals a non-constant plain
string s. Nested strings.Clone layers inside that conversion are removed too.
The whole chain therefore reaches copy(dst, s), append(dst, s...), or s in one
-fix pass. Constants keep the outer string round-trip because replacing a
non-constant expression with a constant can change switch-case validity; named
string results likewise retain their required conversion.

The clone result must be terminal: append without ... stores the clone as an
element and is excluded, as are cap, slice-returning APIs, readers, and any
consumer that could expose or mutate the snapshot. Comments keep the finding
advisory. The shared deletion/import-liveness engine removes all now-unused
clone imports safely, and terminal ownership prevents overlapping PS5074
nested-clone fixes so one -fix pass reaches the allocation-free form.`,
		Before: `n := copy(dst, bytes.Clone(slices.Clone(src)))
out := append(prefix, bytes.Clone(src)...)
text := string(bytes.Clone(src))
direct := string(bytes.Clone([]byte(strings.Clone(name))))`,
		After: `n := copy(dst, src)
out := append(prefix, src...)
text := string(src)
direct := name`,
		MeasuredWin: `On Apple M2 Pro, copying a 65,546-byte source through
bytes.Clone measured 11,940 ns/op, 73,728 B/op, 1 alloc/op versus 983.3 ns/op,
0 B/op, 0 allocs/op for direct copy (median of five 100-iteration runs):
12.14x faster and 91.8% less time, with the full-size clone allocation removed.
Append and slice/string conversions remove the same redundant O(n) clone before
their required copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5084",
		Doc:  "copying builtin consumes throwaway standard-library Clone chains",
		Run:  runPS5084,
	},
})

func runPS5084(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		owned := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			consumer, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if typedBuiltinName(pass, consumer.Fun, "len") && len(consumer.Args) == 1 && !consumer.Ellipsis.IsValid() {
				chain, matched := matchTypedUnaryPackageCallChain(pass, consumer.Args[0], isTypedStdlibClone)
				if matched {
					if fixedPoint, ok := ps5083FixedPointMatch(pass, consumer, chain); ok && fixedPoint.nestedOK {
						owned[fixedPoint.conversion] = true
					}
				}
			}
			chain, _, matched := ps5084MatchCloneConsumer(pass, consumer)
			if !matched {
				return true
			}
			if fixedPoint, ok := ps5084FixedPointMatch(pass, consumer, chain); ok && fixedPoint.nestedOK {
				// The outer terminal rewrite removes this nested
				// []byte(strings.Clone(...)) consumer too. Letting both
				// diagnostics edit it would overlap and lose the fixed point.
				owned[fixedPoint.conversion] = true
			}
			return true
		})
		ast.Inspect(file, func(node ast.Node) bool {
			consumer, ok := node.(*ast.CallExpr)
			if !ok || owned[consumer] {
				return true
			}
			chain, kind, matched := ps5084MatchCloneConsumer(pass, consumer)
			if !matched {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     consumer.Pos(),
				End:     consumer.End(),
				Message: fmt.Sprintf("%s consumes %d throwaway standard-library Clone layer(s) before copying the same values; copy from the original value directly", kind, len(chain.calls)),
			}
			if fix, ok := ps5084FixedPointFix(pass, file, consumer, chain); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			} else if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, chain.paths, "remove clones before copying builtin", chain.spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5084MatchCloneConsumer(pass *analysis.Pass, call *ast.CallExpr) (typedUnaryCallChain, string, bool) {
	if typedBuiltinName(pass, call.Fun, "copy") && len(call.Args) == 2 && !call.Ellipsis.IsValid() {
		chain, ok := matchTypedUnaryPackageCallChain(pass, call.Args[1], isTypedSliceStdlibClone)
		return chain, "copy source", ok
	}
	if typedBuiltinName(pass, call.Fun, "append") && len(call.Args) == 2 && call.Ellipsis.IsValid() {
		chain, ok := matchTypedUnaryPackageCallChain(pass, call.Args[1], isTypedSliceStdlibClone)
		return chain, "append expanded source", ok
	}
	if len(call.Args) != 1 || call.Ellipsis.IsValid() || !ps5084TypeConversion(pass, call.Fun) {
		return typedUnaryCallChain{}, "", false
	}
	target := pass.TypesInfo.TypeOf(call)
	source := pass.TypesInfo.TypeOf(call.Args[0])
	if ps5084StringType(target) && ps5084ByteOrRuneSlice(source) {
		chain, ok := matchTypedUnaryPackageCallChain(pass, call.Args[0], isTypedSliceStdlibClone)
		return chain, "slice-to-string conversion", ok
	}
	if ps5084ByteOrRuneSlice(target) && ps5084StringType(source) {
		chain, ok := matchTypedUnaryPackageCallChain(pass, call.Args[0], isTypedStringStdlibClone)
		return chain, "string-to-slice conversion", ok
	}
	return typedUnaryCallChain{}, "", false
}

func ps5084TypeConversion(pass *analysis.Pass, expression ast.Expr) bool {
	typeValue, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	return ok && typeValue.IsType()
}

func ps5084StringType(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := value.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.String
}

func ps5084ByteOrRuneSlice(value types.Type) bool {
	if value == nil {
		return false
	}
	slice, ok := value.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	basic, ok := slice.Elem().Underlying().(*types.Basic)
	return ok && (basic.Kind() == types.Uint8 || basic.Kind() == types.Int32)
}

// ps5084FixedPointFix composes a terminal Clone removal with the existing
// []byte(string) consumer identities. This is deliberately narrower than the
// ordinary Clone fix: only the unnamed []byte conversion can be passed as a
// string directly to copy/append, and only an exact non-constant predeclared
// string can replace string([]byte(s)) without changing static/constant type.
type ps5084FixedPoint struct {
	conversion *ast.CallExpr
	kept       ast.Expr
	paths      []string
	nested     typedUnaryCallChain
	nestedOK   bool
}

func ps5084FixedPointMatch(pass *analysis.Pass, consumer *ast.CallExpr, chain typedUnaryCallChain) (ps5084FixedPoint, bool) {
	conversion, ok := ps5084StringByteConversion(pass, chain.base)
	if !ok {
		return ps5084FixedPoint{}, false
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

	if typedBuiltinName(pass, consumer.Fun, "copy") || typedBuiltinName(pass, consumer.Fun, "append") {
		return ps5084FixedPoint{conversion: conversion, kept: kept, paths: paths, nested: nested, nestedOK: nestedOK}, true
	}
	if !ps2108IsUniverseString(pass, consumer.Fun) {
		return ps5084FixedPoint{}, false
	}
	typedValue, ok := pass.TypesInfo.Types[kept]
	if !ok || typedValue.Type == nil || typedValue.Value != nil || !types.Identical(types.Default(typedValue.Type), types.Typ[types.String]) {
		return ps5084FixedPoint{}, false
	}
	return ps5084FixedPoint{conversion: conversion, kept: kept, paths: paths, nested: nested, nestedOK: nestedOK}, true
}

func ps5084FixedPointFix(pass *analysis.Pass, file *ast.File, consumer *ast.CallExpr, chain typedUnaryCallChain) (analysis.SuggestedFix, bool) {
	fixedPoint, ok := ps5084FixedPointMatch(pass, consumer, chain)
	if !ok {
		return analysis.SuggestedFix{}, false
	}
	outerClone := chain.calls[0]
	if typedBuiltinName(pass, consumer.Fun, "copy") || typedBuiltinName(pass, consumer.Fun, "append") {
		return fixReplacedCallScaffoldingPaths(pass, file, fixedPoint.paths, "remove clones and the redundant []byte conversion before the copying builtin",
			analysis.TextEdit{Pos: outerClone.Pos(), End: fixedPoint.kept.Pos()},
			analysis.TextEdit{Pos: fixedPoint.kept.End(), End: outerClone.End()},
		)
	}
	lead, trail := []byte(nil), []byte(nil)
	if !ps2108Primary(fixedPoint.kept) {
		lead, trail = []byte("("), []byte(")")
	}
	return fixReplacedCallScaffoldingPaths(pass, file, fixedPoint.paths, "remove clones and the redundant string/[]byte round-trip",
		analysis.TextEdit{Pos: consumer.Pos(), End: fixedPoint.kept.Pos(), NewText: lead},
		analysis.TextEdit{Pos: fixedPoint.kept.End(), End: consumer.End(), NewText: trail},
	)
}

// ps5084StringByteConversion matches an unnamed []byte conversion whose
// operand has string core type. Defined byte-slice types are excluded because
// copy/append's direct-string special forms do not preserve that conversion's
// static type vocabulary.
func ps5084StringByteConversion(pass *analysis.Pass, expression ast.Expr) (*ast.CallExpr, bool) {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() || !ps5084TypeConversion(pass, call.Fun) {
		return nil, false
	}
	slice, ok := types.Unalias(pass.TypesInfo.TypeOf(call.Fun)).(*types.Slice)
	if !ok {
		return nil, false
	}
	element, ok := types.Unalias(slice.Elem()).(*types.Basic)
	if !ok || element.Kind() != types.Uint8 || !ps5084StringType(pass.TypesInfo.TypeOf(call.Args[0])) {
		return nil, false
	}
	return call, true
}

// ps5084TypeQualifierPath returns a package path used to spell a conversion
// through a cross-package type alias. Literal []byte conversions return empty.
// Supplying this path to the shared editor lets it remove a newly orphaned
// alias import, while comments/cgo retain the advisory-only fallback.
func ps5084TypeQualifierPath(pass *analysis.Pass, expression ast.Expr) string {
	selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	identifier, ok := ps2110Unparen(selector.X).(*ast.Ident)
	if !ok {
		return ""
	}
	packageName, ok := pass.TypesInfo.Uses[identifier].(*types.PkgName)
	if !ok {
		return ""
	}
	return packageName.Imported().Path()
}
