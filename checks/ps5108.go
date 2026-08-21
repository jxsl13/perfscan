package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5108 combines nested standard-library Repeat counts. One final Repeat can
// construct the same output without allocating/copying intermediate repeats.
var PS5108 = register(&lint.Check{
	ID:       "PS5108",
	Category: "alloc",
	Slug:     "nested-stdlib-repeat-product",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "nested strings/bytes/slices.Repeat materializes intermediate repetitions",
		Text: `Repeat composition multiplies counts. Building an inner repetition
only to repeat that complete value again performs avoidable allocation,
copying, and growth work:

  strings.Repeat(strings.Repeat(text, 4), 3)
  -> strings.Repeat(text, 12)

  bytes.Repeat(bytes.Repeat(data, 2), 8)
  -> bytes.Repeat(data, 16)

  slices.Repeat(slices.Repeat(values, 3), 5)
  -> slices.Repeat(values, 15)

PS5108 collapses an arbitrarily deep adjacent run in one fix. Every count must
be a positive compile-time integer constant, and their product must fit signed
32-bit int. The conservative 32-bit ceiling keeps the generated literal valid
on every Go target. Dynamic and negative counts remain untouched. Zero counts
are excluded because an outer zero can suppress an inner overflow panic; for
example Repeat(Repeat(s, huge), 0) can panic while Repeat(s, 0) cannot.

The rewrite is BIT-IDENTICAL for the accepted domain. Repeating a sequence a
times and then b times produces exactly a*b copies in the same order. The seed
expression is still evaluated once. Removed counts are constants, so there is
no lost side effect or evaluation-order change. Positive factors make the
original nested calls and the combined call overflow on exactly the same final
length and with the same package panic. Empty strings and slices retain their
documented results. bytes.Repeat and slices.Repeat still return one independent
final slice with the same nilness, length, capacity, element order, and shallow
element aliasing; only throwaway intermediate storage disappears.

The shared repeated-call abstraction resolves every layer through go/types,
requires one ordinary package binding, and now also requires identical result
types across the chain. That last condition prevents an explicit generic form
such as slices.Repeat[[]T](slices.Repeat[Named](x, 2), 3) from changing an
interface's dynamic type when the outer call is removed. Aliases, parentheses,
and identical explicit instantiations work; dot imports, function values, user
lookalikes, cross-package calls, mixed generic result types, and single calls
do not match.

The fix keeps the innermost Repeat call and seed byte-for-byte, removes outer
call scaffolding, and replaces only the retained count with the decimal product.
When removed text contains a comment, an import's last qualifier, or a local
constant's last use, the diagnostic remains advisory. A deep chain reaches its
one-call fixed point in one -fix pass.`,
		Before: `expanded := bytes.Repeat(
	bytes.Repeat(payload, 4),
	8,
)`,
		After: `expanded := bytes.Repeat(payload, 32)`,
		MeasuredWin: `On an Apple M2 Pro, benchmarks/ps5108_test.go (10 runs,
single CPU) measured a 64 KiB byte result built through two nested Repeat
allocations at a median 7,095.5 ns/op, 81,920 B/op, and 2 allocs/op, versus
5,439 ns/op, 65,536 B/op, and 1 alloc/op with the combined count: about 1.30x
faster while removing the 16 KiB intermediate allocation and copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5108",
		Doc:  "nested strings/bytes/slices.Repeat calls use constant counts that can be multiplied",
		Run:  runPS5108,
	},
})

var ps5108Packages = map[string]bool{
	"strings": true,
	"bytes":   true,
	"slices":  true,
}

func runPS5108(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok || covered[outer] {
				return true
			}
			matched, ok := matchRepeatedTypedPackageCall(
				pass, outer, 2, 0, ps5108Allowed,
				func(outer, inner *ast.CallExpr) bool {
					_, outerOK := ps5108PositiveCount(pass, outer.Args[1])
					_, innerOK := ps5108PositiveCount(pass, inner.Args[1])
					return outerOK && innerOK
				},
			)
			if !ok {
				return true
			}
			product, ok := ps5108CountProduct(pass, matched.calls)
			if !ok {
				return true
			}
			markRepeatedTypedCall(covered, matched)
			pkgPath := matched.fn.Pkg().Path()
			diagnostic := analysis.Diagnostic{
				Pos: matched.outer.Pos(),
				End: matched.outer.End(),
				Message: fmt.Sprintf("%s.Repeat is nested %d times with positive constant counts; combine them to %d and avoid %d intermediate repetition(s)",
					pkgPath, matched.layers, product, matched.layers-1),
			}
			if fix, ok := ps5108Fix(pass, file, matched, product); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5108Allowed(pkgPath, name string) bool {
	return name == "Repeat" && ps5108Packages[pkgPath]
}

func ps5108PositiveCount(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	typed, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || typed.Value == nil || typed.Value.Kind() != constant.Int {
		return 0, false
	}
	value, exact := constant.Int64Val(constant.ToInt(typed.Value))
	return value, exact && value > 0 && value <= 1<<31-1
}

func ps5108CountProduct(pass *analysis.Pass, calls []*ast.CallExpr) (int64, bool) {
	const maxPortableInt = int64(1<<31 - 1)
	product := int64(1)
	for _, call := range calls {
		if call == nil || len(call.Args) != 2 {
			return 0, false
		}
		count, ok := ps5108PositiveCount(pass, call.Args[1])
		if !ok || product > maxPortableInt/count {
			return 0, false
		}
		product *= count
	}
	return product, true
}

func ps5108Fix(pass *analysis.Pass, file *ast.File, matched repeatedTypedCall, product int64) (analysis.SuggestedFix, bool) {
	edits := []analysis.TextEdit{
		{Pos: matched.outer.Pos(), End: matched.keep.Pos()},
		{Pos: matched.keep.End(), End: matched.outer.End()},
	}
	keptCount, ok := ps5108PositiveCount(pass, matched.keep.Args[1])
	if !ok {
		return analysis.SuggestedFix{}, false
	}
	if keptCount != product {
		edits = append(edits, analysis.TextEdit{
			Pos: matched.keep.Args[1].Pos(), End: matched.keep.Args[1].End(),
			NewText: strconv.AppendInt(nil, product, 10),
		})
	}
	return fixReplacedCallScaffoldingPaths(pass, file, []string{matched.fn.Pkg().Path()}, "combine nested Repeat counts", edits...)
}
