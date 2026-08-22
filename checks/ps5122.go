package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5122 removes a strings.Contains guard whose only purpose is to avoid a
// no-op strings.ReplaceAll call. ReplaceAll already returns its input unchanged
// when no match exists, so the guard adds a matching-path scan.
var PS5122 = register(&lint.Check{
	ID:       "PS5122",
	Category: "arith",
	Slug:     "contains-guarded-replaceall",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Contains guards a ReplaceAll call that already handles the no-match case",
		Text: `A Contains guard around an otherwise unconditional ReplaceAll scans
the input before ReplaceAll performs its own match count and replacement work:

  if strings.Contains(value, old) {
      value = strings.ReplaceAll(value, old, replacement)
  }

strings.ReplaceAll already returns value unchanged when old does not occur, so
the guard can be removed:

  value = strings.ReplaceAll(value, old, replacement)

This check uses the shared typed guarded-transformer and identity-fallback
abstractions and requires the exact strings package binding, the same plain
input object, equal constant old values or the same old object, and a stable
replacement value. It covers a sole in-place assignment; explicit else or
immediate return fallbacks of the original input; and assignments whose same
plain target is initialized/assigned the original input immediately before or
in a one-statement else. Aliases and parentheses work.

Initializers, compound or negated conditions, additional guarded work,
different inputs/needles/targets, short declarations, effectful replacements,
delayed fallbacks, methods, function values, dot imports, and user lookalikes
stay silent. bytes.Contains+bytes.ReplaceAll is intentionally excluded:
bytes.ReplaceAll returns a fresh slice copy even when the needle is absent, so
removing that guard would change allocation, pointer, capacity, and aliasing.

The automatic fix is deletion-only: it retains the existing assignment or
transformed return byte-for-byte and removes the guard plus the redundant
fallback. Comments, imports, or last local constant/variable uses in deleted
syntax keep the finding advisory.

Within the accepted domain the rewrite is BIT-IDENTICAL for empty needles,
empty replacements, invalid UTF-8, overlapping candidates, equal old/new
values, and match/no-match paths. The replacement
expression is restricted to a constant or plain identifier because the
unguarded form evaluates it on the no-match path.`,
		Before: `if strings.Contains(value, old) {
	value = strings.ReplaceAll(value, old, replacement)
}`,
		After: `value = strings.ReplaceAll(value, old, replacement)`,
		MeasuredWin: `benchmarks/ps5122_test.go measures a matching 20 KiB input
on Apple M2 Pro (darwin/arm64, Go benchmark medians across 10 single-CPU runs).
Removing the guard reduced median time from 3166 ns/op to 2759 ns/op (12.9%,
1.15x) while retaining 21760 B/op and 1 alloc/op.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5122",
		Doc:  "strings.Contains redundantly guards an in-place or returned strings.ReplaceAll",
		Run:  runPS5122,
	},
})

type ps5122Match struct {
	flow typedGuardedFallbackTransformer
}

func runPS5122(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			statement, ok := node.(*ast.IfStmt)
			if !ok {
				return true
			}
			match, ok := ps5122GuardedReplaceAll(pass, statement, parents)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos: statement.Cond.Pos(), End: match.flow.composition.transformerExpression.End(),
				Message: "strings.Contains scans before strings.ReplaceAll repeats the match work; ReplaceAll already returns the original string when no match exists, so the guard is redundant",
			}
			if fix, ok := ps5122SuggestedFix(pass, file, &match); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5122GuardedReplaceAll(pass *analysis.Pass, statement *ast.IfStmt, parents map[ast.Node]ast.Node) (ps5122Match, bool) {
	composition, ok := matchTypedGuardedPackageTransformer(
		pass, statement, "strings", "Contains", "ReplaceAll", 3,
	)
	if !ok || len(statement.Body.List) != 1 ||
		!sameTypedStableCompanion(pass, composition.predicateCompanion, composition.transformerCompanion, true) ||
		!stableTypedValue(pass, composition.transformer.Args[2]) {
		return ps5122Match{}, false
	}
	flow, ok := matchTypedGuardedIdentityTransformer(pass, statement, parents, composition)
	if !ok {
		return ps5122Match{}, false
	}
	return ps5122Match{flow: flow}, true
}

func ps5122SuggestedFix(pass *analysis.Pass, file *ast.File, match *ps5122Match) (analysis.SuggestedFix, bool) {
	return guardedFallbackDeletionFix(pass, file, &match.flow, "remove the redundant Contains guard")
}
