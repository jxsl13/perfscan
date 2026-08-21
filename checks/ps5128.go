package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5128 removes a Contains guard around a matching split call when the false
// path constructs the exact singleton result that every supported split API
// already returns for a missing separator.
var PS5128 = register(&lint.Check{
	ID:       "PS5128",
	Category: "arith",
	Slug:     "contains-guarded-split",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "Contains guards a split call whose miss result is already the singleton input",
		Text: `A membership guard around a split operation scans once to decide
whether to split and then scans again on every hit:

  if strings.Contains(value, separator) {
      return strings.Split(value, separator)
  }
  return []string{value}

Split already returns []string{value} when a nonempty separator is absent, and
an empty separator makes Contains true, so the direct call represents both
paths exactly:

  return strings.Split(value, separator)

This check covers strings and bytes variants of Split, SplitAfter, SplitN, and
SplitAfterN. Counted variants require an exact compile-time count equal to one
or less than zero. Zero is excluded because SplitN(..., 0) returns nil even
when Contains is false. Positive counts above one are also excluded: on a miss
the element is identical but SplitN preallocates outer capacity N, whereas the
singleton fallback has capacity one. Negative counts derive capacity from the
actual match count and therefore retain capacity one on a miss; count one also
does so by construction.

The rule uses the shared typed guarded-transformer, stable-companion,
sentinel-fallback, and deletion editor abstractions. Predicate and producer
must resolve through the same ordinary strings or bytes package binding,
consume the same plain input object, and use the same stable separator.
Strings accept the same object or equal compile-time separator values. Bytes
requires the same separator slice object; separately constructed slices stay
silent because the original hit path evaluated both expressions.

The false path must be the exact singleton composite literal []string{value}
or [][]byte{value}, either in an immediate/else return, a same-target else
assignment, or an immediately preceding =/:= initializer. Named outer slice
types, arrays, keyed/multiple/empty literals, conversions, append calls, nil,
and different elements stay silent. For bytes, the accepted literal and Split
miss result preserve the inner slice's nil state, start pointer, length,
capacity, and backing array; the outer slice is a new one-element result in
both forms.

If initializers, compound/negated predicates, different inputs/separators,
dynamic or zero counts, effectful separators, delayed fallbacks, branch work,
short declarations, methods, function values, dot imports, or user lookalikes
are present, the rule stays silent. Aliases and parentheses work.

The automatic fix deletes only the Contains control flow and singleton
fallback while retaining the existing split return/assignment byte-for-byte.
Comments or last required local/import uses in deleted syntax keep the finding
advisory. It deliberately does not rewrite guarded immediate piece indexing:
PS5121 owns that stronger Contains+SplitN-to-Cut transformation and removes the
result slice allocation entirely.

Within the accepted domain the rewrite is BIT-IDENTICAL for hits, misses,
empty inputs and separators, invalid UTF-8, adjacent separators, count one or
negative counts, nil/empty byte slices, and byte slices with spare capacity. It removes
one complete preliminary separator scan on every hit without changing result
allocation, element identity, or alias behavior.`,
		Before: `if strings.Contains(value, separator) {
	return strings.Split(value, separator)
}
return []string{value}`,
		After: `return strings.Split(value, separator)`,
		MeasuredWin: `benchmarks/ps5128_test.go measures a matching 20 KiB string
whose only separator is near the end. Contains therefore scans almost the
whole input before Split repeats the scan and creates its result. Across 10
single-CPU runs, median latency falls from 984.0 ns/op to 697.15 ns/op: a
29.15% reduction, or 1.41x faster, with both versions retaining 32 B/op and
1 alloc/op.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5128",
		Doc:  "Contains redundantly guards a Split-family call and exact singleton fallback",
		Run:  runPS5128,
	},
})

type ps5128Spec struct {
	pkgPath        string
	producer       string
	arity          int
	sliceCompanion bool
	counted        bool
}

var ps5128Specs = []ps5128Spec{
	{pkgPath: "strings", producer: "Split", arity: 2},
	{pkgPath: "strings", producer: "SplitAfter", arity: 2},
	{pkgPath: "strings", producer: "SplitN", arity: 3, counted: true},
	{pkgPath: "strings", producer: "SplitAfterN", arity: 3, counted: true},
	{pkgPath: "bytes", producer: "Split", arity: 2, sliceCompanion: true},
	{pkgPath: "bytes", producer: "SplitAfter", arity: 2, sliceCompanion: true},
	{pkgPath: "bytes", producer: "SplitN", arity: 3, sliceCompanion: true, counted: true},
	{pkgPath: "bytes", producer: "SplitAfterN", arity: 3, sliceCompanion: true, counted: true},
}

type ps5128Match struct {
	flow typedGuardedFallbackTransformer
	spec ps5128Spec
}

func runPS5128(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			statement, ok := node.(*ast.IfStmt)
			if !ok {
				return true
			}
			match, ok := ps5128GuardedSplit(pass, statement, parents)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos: statement.Cond.Pos(), End: match.flow.composition.transformerExpression.End(),
				Message: match.spec.pkgPath + ".Contains scans before " + match.spec.pkgPath + "." +
					match.spec.producer + " repeats the separator search; " + match.spec.producer +
					" already returns the exact singleton input on a miss, so the guard is redundant",
			}
			if fix, ok := guardedFallbackDeletionFix(pass, file, &match.flow,
				"remove the redundant Contains guard and singleton fallback"); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5128GuardedSplit(pass *analysis.Pass, statement *ast.IfStmt, parents map[ast.Node]ast.Node) (ps5128Match, bool) {
	for _, spec := range ps5128Specs {
		composition, ok := matchTypedGuardedPackageTransformer(
			pass, statement, spec.pkgPath, "Contains", spec.producer, spec.arity,
		)
		if !ok || !sameTypedStableCompanion(pass, composition.predicateCompanion,
			composition.transformerCompanion, !spec.sliceCompanion) {
			continue
		}
		if spec.counted {
			count, constantCount := ps5120IntegerConstant(pass, composition.transformer.Args[2])
			if !constantCount || count == 0 || count > 1 {
				continue
			}
		}
		inputObject := pass.TypesInfo.ObjectOf(composition.input)
		flow, ok := matchTypedGuardedFallbackTransformer(pass, statement, parents, composition, func(expression ast.Expr) bool {
			return ps5128SingletonFallback(pass, expression, inputObject, spec.pkgPath)
		})
		if ok {
			return ps5128Match{flow: flow, spec: spec}, true
		}
	}
	return ps5128Match{}, false
}

func ps5128SingletonFallback(pass *analysis.Pass, expression ast.Expr, input types.Object, pkgPath string) bool {
	literal, ok := ps2110Unparen(expression).(*ast.CompositeLit)
	if !ok || literal.Type == nil || literal.Incomplete || len(literal.Elts) != 1 {
		return false
	}
	if _, keyed := literal.Elts[0].(*ast.KeyValueExpr); keyed ||
		!plainObjectExpression(pass, literal.Elts[0], input) {
		return false
	}
	want := types.NewSlice(types.Typ[types.String])
	if pkgPath == "bytes" {
		want = types.NewSlice(types.NewSlice(types.Typ[types.Byte]))
	}
	return types.Identical(pass.TypesInfo.TypeOf(literal), want)
}
