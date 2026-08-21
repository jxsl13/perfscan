package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5092 removes standard-library Clone chains whose only consumer is a
// comparison that can observe only string value or container nilness.
var PS5092 = register(&lint.Check{
	ID:       "PS5092",
	Category: "alloc",
	Slug:     "clone-fed-nonretaining-comparison",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a string or nilness comparison consumes throwaway clones",
		Text: `Standard-library Clone functions preserve the observations made by
comparison operators, so allocating a copy immediately before the comparison
is wasted:

  strings.Clone(left) == strings.Clone(right) -> left == right
  strings.Clone(left) < right                  -> left < right
  bytes.Clone(data) == nil                     -> data == nil
  slices.Clone(values) != nil                  -> values != nil
  maps.Clone(index) == nil                     -> index == nil

This check handles all six string operators (==, !=, <, <=, >, >=) and both
orientations of exact nil equality/inequality. It unwraps every safe operand's
arbitrarily deep Clone chain in one fix, including heterogeneous
bytes.Clone/slices.Clone chains. Callees and nil resolve through go/types, so
aliases and explicit generic instantiations work while shadowed identifiers,
dot imports, function values, user Clone methods, and type-changing wrappers
stay untouched.

Container clones are deliberately limited to == nil and != nil. Slices and
maps cannot otherwise be compared, and replacing a clone in a consumer that
stores or returns it would destroy its independence contract. String
concatenation and slicing are excluded because strings.Clone can be an
intentional backing-retention boundary when their result aliases an operand.

The rewrite is BIT-IDENTICAL for race-free Go programs. strings.Clone
preserves every byte and therefore lexical comparison order; bytes.Clone,
slices.Clone, and maps.Clone preserve nilness exactly. Each base expression
is evaluated once in the same left-to-right position. Only allocation and
copy scaffolding disappears. Comments keep the finding advisory. The shared
multi-package deletion engine removes orphaned Clone imports safely, while
terminal ownership prevents overlapping nested-Clone diagnostics and reaches
the allocation-free comparison in one -fix pass. If removing string Clone
calls would turn a comparison into a constant switch case or map key, the
finding remains advisory so the fix cannot introduce an illegal duplicate
constant.`,
		Before: `same := strings.Clone(left) == strings.Clone(right)
missing := bytes.Clone(data) == nil`,
		After: `same := left == right
missing := data == nil`,
		MeasuredWin: `On Apple M2 Pro, comparing two equal 65,525-byte strings
through three forced Clone layers measured 12,517 ns/op, 196,608 B/op,
3 allocs/op versus comparing the originals at 1,231 ns/op, 0 B/op,
0 allocs/op (median of five 200ms runs): 10.17x faster and 90.2% less time.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5092",
		Doc:  "throwaway standard-library Clone calls immediately feed a value or nilness comparison",
		Run:  runPS5092,
	},
})

func runPS5092(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			comparison, ok := node.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			matches := ps5092ComparisonMatches(pass, comparison)
			if len(matches) == 0 {
				return true
			}

			totalLayers := 0
			var spans []tokenSpan
			var paths []string
			for _, match := range matches {
				totalLayers += len(match.calls)
				spans = append(spans, match.spans...)
				paths = append(paths, match.paths...)
			}
			diagnostic := analysis.Diagnostic{
				Pos:     comparison.Pos(),
				End:     comparison.End(),
				Message: fmt.Sprintf("%s comparison consumes %d throwaway standard-library Clone layer(s) across %d operand(s); compare the original values directly", comparison.Op, totalLayers, len(matches)),
			}
			if !ps5092ReplacementIntroducesConstant(pass, comparison, parents) {
				if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, paths, "remove clones before non-retaining comparison", spans...); ok {
					diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
				}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5092ReplacementIntroducesConstant(pass *analysis.Pass, comparison *ast.BinaryExpr, parents map[ast.Node]ast.Node) bool {
	if !ps5084StringType(pass.TypesInfo.TypeOf(comparison.X)) || !ps5084StringType(pass.TypesInfo.TypeOf(comparison.Y)) {
		return false
	}
	for _, operand := range []ast.Expr{comparison.X, comparison.Y} {
		if chain, ok := matchTypedUnaryPackageCallChain(pass, operand, isTypedStringStdlibClone); ok {
			operand = chain.base
		}
		typed, ok := pass.TypesInfo.Types[ps2110Unparen(operand)]
		if !ok || typed.Value == nil {
			return false
		}
	}
	return replacementIntroducesConstantInUniqueContext(pass, comparison, parents)
}

func ps5092ComparisonMatches(pass *analysis.Pass, comparison *ast.BinaryExpr) []typedUnaryCallChain {
	if ps5106OpString(comparison.Op) == "" {
		return nil
	}
	var matches []typedUnaryCallChain
	if ps5084StringType(pass.TypesInfo.TypeOf(comparison.X)) && ps5084StringType(pass.TypesInfo.TypeOf(comparison.Y)) {
		if chain, ok := matchTypedUnaryPackageCallChain(pass, comparison.X, isTypedStringStdlibClone); ok {
			matches = append(matches, chain)
		}
		if chain, ok := matchTypedUnaryPackageCallChain(pass, comparison.Y, isTypedStringStdlibClone); ok {
			matches = append(matches, chain)
		}
		return matches
	}
	if comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return nil
	}
	if ps5092Nil(pass, comparison.Y) {
		if chain, ok := matchTypedUnaryPackageCallChain(pass, comparison.X, isTypedContainerStdlibClone); ok {
			matches = append(matches, chain)
		}
	}
	if ps5092Nil(pass, comparison.X) {
		if chain, ok := matchTypedUnaryPackageCallChain(pass, comparison.Y, isTypedContainerStdlibClone); ok {
			matches = append(matches, chain)
		}
	}
	return matches
}

func ps5092Nil(pass *analysis.Pass, expression ast.Expr) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.Uses[identifier] == types.Universe.Lookup("nil")
}
