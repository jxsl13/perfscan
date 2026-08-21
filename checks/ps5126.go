package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5126 collapses a forward membership predicate guarding the matching
// backward index lookup. LastIndex and LastIndexAny already return -1 when the
// predicate is false.
var PS5126 = register(&lint.Check{
	ID:       "PS5126",
	Category: "arith",
	Slug:     "contains-guarded-lastindex",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "Contains guards a backward index call that already returns the absence sentinel",
		Text: `A forward membership guard followed by the matching backward lookup
searches the same input twice on every hit:

  if strings.Contains(value, needle) {
      return strings.LastIndex(value, needle)
  }
  return -1

LastIndex already returns -1 when needle is absent, so the direct backward
lookup represents the complete control flow:

  return strings.LastIndex(value, needle)

This check uses the shared typed guarded-transformer, stable-companion,
integer-sentinel, and fallback-deletion abstractions. It covers
strings/bytes Contains+LastIndex and ContainsAny+LastIndexAny. Calls must
resolve through the same ordinary standard-library package binding, consume
the same plain input object, and use the same stable companion. String
needles/cutsets may be the same object or equal compile-time constants. The
[]byte needle of bytes.Contains/LastIndex must be the same slice object;
independently constructed slices stay silent because the guarded hit path
evaluates both original expressions separately.

Accepted control flow is exact: a transformed return immediately followed by
return -1; the equivalent one-statement else; an assignment whose else writes
-1 to the same plain target; or an assignment immediately preceded by that
target's =/:= -1 initialization. No other branch work is accepted. If
initializers, compound or negated predicates, different inputs/companions or
targets, delayed/non--1 fallbacks, short declarations, methods, function
values, dot imports, effectful companions, and user lookalikes stay silent.
Aliases and parentheses work.

The automatic fix deletes only the redundant guard and -1 fallback while
retaining the existing LastIndex expression, return/assignment spelling, and
evaluation order byte-for-byte. Comments, imports, or last local uses in the
deleted spans keep the diagnostic advisory. Its retained call boundaries also
compose in one fix pass with direct backward-search rules: a one-byte
LastIndex/LastIndexAny call can simultaneously become LastIndexByte.

Within the accepted domain the rewrite is BIT-IDENTICAL for hits, misses,
empty needles/cutsets, invalid UTF-8, RuneError cutsets, overlapping matches,
nil/empty byte slices, and named string or byte-slice types accepted by the
standard APIs. It removes one complete forward membership scan on every hit
without changing allocations or alias behavior.`,
		Before: `if strings.Contains(value, needle) {
	return strings.LastIndex(value, needle)
}
return -1`,
		After: `return strings.LastIndex(value, needle)`,
		MeasuredWin: `benchmarks/ps5126_test.go measures a matching 20 KiB input
whose sole occurrence is near the end. Contains therefore scans almost the
whole string before LastIndex immediately rediscovers the occurrence from the
other direction. On Apple M2 Pro with Go 1.26.6 (10 runs, one CPU), removing
the guard reduced median time from 343.75 ns/op to 48.425 ns/op (85.9%,
7.10x); both forms retained 0 B/op and 0 allocs/op.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5126",
		Doc:  "Contains redundantly guards the matching LastIndex call and -1 fallback",
		Run:  runPS5126,
	},
})

type ps5126Spec struct {
	pkgPath        string
	predicate      string
	lastIndex      string
	sliceCompanion bool
}

var ps5126Specs = []ps5126Spec{
	{pkgPath: "strings", predicate: "Contains", lastIndex: "LastIndex"},
	{pkgPath: "bytes", predicate: "Contains", lastIndex: "LastIndex", sliceCompanion: true},
	{pkgPath: "strings", predicate: "ContainsAny", lastIndex: "LastIndexAny"},
	{pkgPath: "bytes", predicate: "ContainsAny", lastIndex: "LastIndexAny"},
}

type ps5126Match struct {
	flow typedGuardedFallbackTransformer
	spec ps5126Spec
}

func runPS5126(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			statement, ok := node.(*ast.IfStmt)
			if !ok {
				return true
			}
			match, ok := ps5126GuardedLastIndex(pass, statement, parents)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos: statement.Cond.Pos(), End: match.flow.composition.transformerExpression.End(),
				Message: match.spec.pkgPath + "." + match.spec.predicate + " searches forward before " +
					match.spec.pkgPath + "." + match.spec.lastIndex + " repeats the lookup backward; " +
					match.spec.lastIndex + " already returns -1 when absent, so the guard is redundant",
			}
			if fix, ok := guardedFallbackDeletionFix(pass, file, &match.flow,
				"remove the redundant "+match.spec.predicate+" guard and -1 fallback"); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5126GuardedLastIndex(pass *analysis.Pass, statement *ast.IfStmt, parents map[ast.Node]ast.Node) (ps5126Match, bool) {
	for _, spec := range ps5126Specs {
		composition, ok := matchTypedGuardedPackageTransformer(
			pass, statement, spec.pkgPath, spec.predicate, spec.lastIndex, 2,
		)
		if !ok || !sameTypedStableCompanion(pass, composition.predicateCompanion,
			composition.transformerCompanion, !spec.sliceCompanion) {
			continue
		}
		flow, ok := matchTypedGuardedFallbackTransformer(pass, statement, parents, composition, func(expression ast.Expr) bool {
			return typedIntegerSentinel(pass, expression, -1)
		})
		if ok {
			return ps5126Match{flow: flow, spec: spec}, true
		}
	}
	return ps5126Match{}, false
}
