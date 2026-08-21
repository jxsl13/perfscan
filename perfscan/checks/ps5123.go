package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5123 collapses a membership predicate guarding the matching Index call.
// Every supported Index API already returns -1 when the predicate is false.
var PS5123 = register(&lint.Check{
	ID:       "PS5123",
	Category: "arith",
	Slug:     "contains-guarded-index",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "Contains guards an Index call that already returns the absence sentinel",
		Text: `A membership guard followed by the matching index lookup asks the standard
library to search the same input twice on every hit:

  if strings.Contains(value, needle) {
      return strings.Index(value, needle)
  }
  return -1

Index already returns -1 when the needle is absent, so the complete control
flow is the direct lookup:

  return strings.Index(value, needle)

This check is built on the shared typed guarded-transformer and single-fallback
abstractions. It covers the exact standard-library pairs:

  - strings/bytes Contains + Index;
  - strings/bytes ContainsAny + IndexAny;
  - strings/bytes ContainsRune + IndexRune; and
  - slices.Contains + slices.Index.

Both calls must resolve through the same ordinary package binding, consume the
same plain input object, and use the same stable companion. Scalar/string/rune
companions may be the same object or equal compile-time constants. The []byte
needle of bytes.Contains/Index must be the same slice object; independently
constructed slices stay silent because the original evaluates two distinct
expressions on the hit path.

The accepted control-flow forms are deliberately exact: an early transformed
return immediately followed by return -1, the equivalent one-statement else;
an assignment whose else assigns -1 to the same plain target; or an assignment
immediately preceded by the same target's =/:= -1 initialization. The branch
contains no additional work. Initializers on the if, negation/compound
conditions, different inputs/companions/targets, delayed or non--1 fallbacks,
methods, function values, dot imports, effectful companion expressions, nested
slice constructions, and user lookalikes stay silent. Aliases and parentheses
work.

The automatic fix is deletion-only around the retained Index call. Return and
else forms keep the existing transformed return/assignment byte-for-byte. The
initialized-assignment form keeps its original target and =/:= token, splicing
the existing Index expression over the -1 initializer and guard. Comments,
imports, or last local uses in deleted syntax keep the finding advisory.

Because its edits stop at the retained Index expression's boundaries, this
rule composes in the same fix pass with existing direct-search rules. For
example, a one-byte strings.Contains+strings.Index pair can become one
strings.IndexByte call without an intermediate guarded Index form.

Within the accepted domain the rewrite is BIT-IDENTICAL for hits, misses,
empty needles/cutsets, invalid UTF-8, RuneError, nil/empty byte slices, empty
slices, and all comparable slices element types. It preserves the int result
and removes one complete predicate scan on hits without changing allocation
or alias behavior.`,
		Before: `if strings.Contains(value, needle) {
	return strings.Index(value, needle)
}
return -1`,
		After: `return strings.Index(value, needle)`,
		MeasuredWin: `On Apple M2 Pro with Go 1.26.6 (10 runs, one CPU),
benchmarks/ps5123_test.go measured a matching 20 KiB input whose needle is near
the end. Removing the guard reduced median time from 576.75 ns/op to 290.85
ns/op (49.6%, 1.98x); both forms retained 0 B/op and 0 allocs/op.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5123",
		Doc:  "Contains redundantly guards the matching Index call and -1 fallback",
		Run:  runPS5123,
	},
})

type ps5123Spec struct {
	pkgPath        string
	predicate      string
	index          string
	sliceCompanion bool
}

var ps5123Specs = []ps5123Spec{
	{pkgPath: "strings", predicate: "Contains", index: "Index"},
	{pkgPath: "bytes", predicate: "Contains", index: "Index", sliceCompanion: true},
	{pkgPath: "strings", predicate: "ContainsAny", index: "IndexAny"},
	{pkgPath: "bytes", predicate: "ContainsAny", index: "IndexAny"},
	{pkgPath: "strings", predicate: "ContainsRune", index: "IndexRune"},
	{pkgPath: "bytes", predicate: "ContainsRune", index: "IndexRune"},
	{pkgPath: "slices", predicate: "Contains", index: "Index"},
}

type ps5123Match struct {
	flow typedGuardedFallbackTransformer
	spec ps5123Spec
}

func runPS5123(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			statement, ok := node.(*ast.IfStmt)
			if !ok {
				return true
			}
			match, ok := ps5123GuardedIndex(pass, statement, parents)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos: statement.Cond.Pos(), End: match.flow.composition.transformerExpression.End(),
				Message: match.spec.pkgPath + "." + match.spec.predicate + " searches before " +
					match.spec.pkgPath + "." + match.spec.index + " repeats the same lookup; " +
					match.spec.index + " already returns -1 when absent, so the guard is redundant",
			}
			if fix, ok := ps5123SuggestedFix(pass, file, &match); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5123GuardedIndex(pass *analysis.Pass, statement *ast.IfStmt, parents map[ast.Node]ast.Node) (ps5123Match, bool) {
	for _, spec := range ps5123Specs {
		composition, ok := matchTypedGuardedPackageTransformer(
			pass, statement, spec.pkgPath, spec.predicate, spec.index, 2,
		)
		if !ok || !sameTypedStableCompanion(pass, composition.predicateCompanion, composition.transformerCompanion, !spec.sliceCompanion) {
			continue
		}
		flow, ok := matchTypedGuardedFallbackTransformer(pass, statement, parents, composition, func(expression ast.Expr) bool {
			return ps5123MinusOne(pass, expression)
		})
		if ok {
			return ps5123Match{flow: flow, spec: spec}, true
		}
	}
	return ps5123Match{}, false
}

func ps5123MinusOne(pass *analysis.Pass, expression ast.Expr) bool {
	return typedIntegerSentinel(pass, expression, -1)
}

func ps5123SuggestedFix(pass *analysis.Pass, file *ast.File, match *ps5123Match) (analysis.SuggestedFix, bool) {
	return guardedFallbackDeletionFix(pass, file, &match.flow,
		"remove the redundant "+match.spec.predicate+" guard and -1 fallback")
}
