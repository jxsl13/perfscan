package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5124 collapses a Contains predicate guarding the matching Count call.
// Count already returns zero when the predicate is false.
var PS5124 = register(&lint.Check{
	ID:       "PS5124",
	Category: "arith",
	Slug:     "contains-guarded-count",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "Contains guards a Count call that already returns zero on misses",
		Text: `A membership guard followed by the matching occurrence count asks the
standard library to scan the same input once to prove a hit and again to count
all hits:

  if strings.Contains(value, needle) {
      return strings.Count(value, needle)
  }
  return 0

strings.Count and bytes.Count already return zero when a non-empty needle is
absent. Empty needles are present by definition and Count returns the UTF-8
rune count plus one, so the direct call is equivalent there too:

  return strings.Count(value, needle)

This check uses the shared typed guarded-transformer, stable-companion, and
sentinel-fallback abstractions. It covers strings.Contains+strings.Count and
bytes.Contains+bytes.Count. Both calls must resolve through the same ordinary
package binding, consume the same plain input object, and use the same stable
needle. String needles may be the same object or equal compile-time constants.
Byte-slice needles must be the same slice object; independently constructed
slices stay silent because removing the guard would delete one separately
evaluated allocation/expression. Aliases and parentheses work.

The accepted control-flow forms are exact: an early counted return immediately
followed by return 0; the equivalent one-statement else; assignments whose
else assigns 0 to the same plain target; and an assignment immediately
preceded by the same target's =/:= 0 initialization. The true branch contains
no other work. If initializers, compound or negated conditions, different
inputs/needles/targets, delayed or nonzero fallbacks, short declarations,
methods, function values, dot imports, effectful needles, and user lookalikes
stay silent.

The automatic fix is deletion-only around the retained Count expression. It
preserves the existing Count call, return or assignment target, operator,
operand spelling, and evaluation order byte-for-byte. Comments, imports, or
last local uses in deleted syntax keep the finding advisory. The same shared
fix engine also powers guarded Index removal, so sentinel control-flow policy
and comment/liveness safety do not drift between the two rules.

Within the accepted domain the rewrite is BIT-IDENTICAL for hits, misses,
empty needles, invalid UTF-8, overlapping occurrences, nil/empty byte slices,
and named string/byte-slice types accepted by the standard APIs. It preserves
the int result and removes the complete preliminary membership scan on every
hit without changing allocation or alias behavior.`,
		Before: `if strings.Contains(value, needle) {
	return strings.Count(value, needle)
}
return 0`,
		After: `return strings.Count(value, needle)`,
		MeasuredWin: `On Apple M2 Pro with Go 1.26.6 (10 runs, one CPU),
benchmarks/ps5124_test.go measured a matching 20 KiB input whose only needle
is near the end. Removing the preliminary Contains scan reduced median time
from 666.95 ns/op to 371.65 ns/op (44.3%, 1.79x); both forms retained 0 B/op
and 0 allocs/op.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5124",
		Doc:  "Contains redundantly guards the matching Count call and zero fallback",
		Run:  runPS5124,
	},
})

type ps5124Spec struct {
	pkgPath        string
	sliceCompanion bool
}

var ps5124Specs = []ps5124Spec{
	{pkgPath: "strings"},
	{pkgPath: "bytes", sliceCompanion: true},
}

type ps5124Match struct {
	flow typedGuardedFallbackTransformer
	spec ps5124Spec
}

func runPS5124(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			statement, ok := node.(*ast.IfStmt)
			if !ok {
				return true
			}
			match, ok := ps5124GuardedCount(pass, statement, parents)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos: statement.Cond.Pos(), End: match.flow.composition.transformerExpression.End(),
				Message: match.spec.pkgPath + ".Contains searches before " + match.spec.pkgPath +
					".Count repeats the lookup and counts all matches; Count already returns 0 when absent, so the guard is redundant",
			}
			if fix, ok := guardedFallbackDeletionFix(pass, file, &match.flow,
				"remove the redundant Contains guard and zero fallback"); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5124GuardedCount(pass *analysis.Pass, statement *ast.IfStmt, parents map[ast.Node]ast.Node) (ps5124Match, bool) {
	for _, spec := range ps5124Specs {
		composition, ok := matchTypedGuardedPackageTransformer(
			pass, statement, spec.pkgPath, "Contains", "Count", 2,
		)
		if !ok || !sameTypedStableCompanion(pass, composition.predicateCompanion, composition.transformerCompanion, !spec.sliceCompanion) {
			continue
		}
		flow, ok := matchTypedGuardedFallbackTransformer(pass, statement, parents, composition, func(expression ast.Expr) bool {
			return ps5124Zero(pass, expression)
		})
		if ok {
			return ps5124Match{flow: flow, spec: spec}, true
		}
	}
	return ps5124Match{}, false
}

func ps5124Zero(pass *analysis.Pass, expression ast.Expr) bool {
	return typedIntegerSentinel(pass, expression, 0)
}
