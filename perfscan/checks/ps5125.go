package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5125 removes a strings.Contains guard whose only purpose is to avoid a
// strings.Replace call. Replace already preserves the input when no match is
// found, for every replacement limit.
var PS5125 = register(&lint.Check{
	ID:       "PS5125",
	Category: "arith",
	Slug:     "contains-guarded-replace",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Contains guards a Replace call that already preserves misses",
		Text: `A Contains guard around strings.Replace performs a preliminary
membership scan before Replace searches for the same old string:

  if strings.Contains(value, old) {
      value = strings.Replace(value, old, replacement, limit)
  }

strings.Replace already returns value unchanged when old is absent, regardless
of whether limit requests zero, one, several, or all replacements. The direct
form therefore carries the same false path itself:

  value = strings.Replace(value, old, replacement, limit)

This check uses the shared typed guarded-transformer, stable-companion, and
identity-fallback abstractions. Both calls must resolve through the same
ordinary strings package binding, consume the same plain input object, and use
the same old value. Equal compile-time old constants or the same old object are
accepted. The replacement and limit must each be a compile-time constant or a
plain identifier because removing the guard evaluates both on the former
no-match path. Aliases and parentheses work.

The accepted control flow includes a sole in-place assignment; explicit else
or immediate returns whose fallback is the original input; and assignments
whose same plain target is initialized/assigned the original input immediately
before or in a one-statement else. The true branch contains no other work. If
initializers, compound or negated conditions, different inputs/old values,
effectful replacements or limits, selectors/indexes, delayed or different
fallbacks, short declarations, methods, function values, dot imports, and user
lookalikes stay silent.

bytes.Contains+bytes.Replace is intentionally excluded. bytes.Replace returns
a fresh slice copy even when old is absent, so unconditional replacement would
change allocation, pointer, capacity, and alias behavior on the false path.

The automatic fix is deletion-only around the retained Replace call. It keeps
the call, operand spelling, assignment/return, and evaluation order
byte-for-byte while deleting the redundant predicate and identity fallback.
Comments, imports, or last local uses in deleted syntax keep the finding
advisory. Replace and ReplaceAll share the same identity-fallback engine, so
their control-flow and liveness safety policy cannot drift.

Within the accepted domain the rewrite is BIT-IDENTICAL for empty old strings,
empty replacements, invalid UTF-8, overlapping candidates, all negative/zero/
positive limits, equal old/new values, and hit/miss paths. It removes the
complete membership scan on hits and can remove all scanning when limit is
zero, without changing the returned string or allocation behavior.`,
		Before: `if strings.Contains(value, old) {
	value = strings.Replace(value, old, replacement, limit)
}`,
		After: `value = strings.Replace(value, old, replacement, limit)`,
		MeasuredWin: `On Apple M2 Pro with Go 1.26.6 (10 runs, one CPU),
benchmarks/ps5125_test.go measured one replacement near the end of a matching
20 KiB input. Removing the guard reduced median time from 2515 ns/op to 2207.5
ns/op (12.2%, 1.14x) while retaining 21760 B/op and 1 alloc/op.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5125",
		Doc:  "strings.Contains redundantly guards strings.Replace",
		Run:  runPS5125,
	},
})

type ps5125Match struct {
	flow typedGuardedFallbackTransformer
}

func runPS5125(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			statement, ok := node.(*ast.IfStmt)
			if !ok {
				return true
			}
			match, ok := ps5125GuardedReplace(pass, statement, parents)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos: statement.Cond.Pos(), End: match.flow.composition.transformerExpression.End(),
				Message: "strings.Contains scans before strings.Replace repeats the same lookup; Replace already returns the original string when no match exists, so the guard is redundant",
			}
			if fix, ok := guardedFallbackDeletionFix(pass, file, &match.flow,
				"remove the redundant Contains guard"); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5125GuardedReplace(pass *analysis.Pass, statement *ast.IfStmt, parents map[ast.Node]ast.Node) (ps5125Match, bool) {
	composition, ok := matchTypedGuardedPackageTransformer(
		pass, statement, "strings", "Contains", "Replace", 4,
	)
	if !ok || !sameTypedStableCompanion(pass, composition.predicateCompanion, composition.transformerCompanion, true) ||
		!stableTypedValue(pass, composition.transformer.Args[2]) ||
		!stableTypedValue(pass, composition.transformer.Args[3]) {
		return ps5125Match{}, false
	}
	flow, ok := matchTypedGuardedIdentityTransformer(pass, statement, parents, composition)
	if !ok {
		return ps5125Match{}, false
	}
	return ps5125Match{flow: flow}, true
}
