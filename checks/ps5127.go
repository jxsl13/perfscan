package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5127 removes a negated utf8.ValidString guard whose only purpose is to
// skip strings.ToValidUTF8 for valid input. The sanitizer already returns
// valid input unchanged.
var PS5127 = register(&lint.Check{
	ID:       "PS5127",
	Category: "arith",
	Slug:     "utf8-validation-guarded-sanitizer",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "utf8.ValidString guards a sanitizer that already preserves valid strings",
		Text: `A validation guard around strings.ToValidUTF8 splits one total
operation into a predicate and a conditional transformer:

  if !utf8.ValidString(value) {
      value = strings.ToValidUTF8(value, replacement)
  }

strings.ToValidUTF8 already returns value unchanged when it is valid and
repairs it when invalid. Calling it directly therefore represents both paths:

  value = strings.ToValidUTF8(value, replacement)

This check introduces the shared typed negated cross-package guarded-
transformer abstraction. The predicate must resolve to the ordinary
unicode/utf8.ValidString package function and the transformer to ordinary
strings.ToValidUTF8, each through its own ordinary import binding. Both calls
must consume the same plain string object. Aliases and parentheses work; dot
imports, methods, function values, shadowed packages, and user lookalikes stay
silent.

The replacement must be a compile-time constant or plain identifier. Removing
the guard evaluates it on the formerly valid path, so calls, selectors,
indexes, receives, and other observable expressions are excluded. The
replacement itself need not be valid UTF-8: valid input never consults its
bytes, while invalid input already evaluated and used the exact same value.

Accepted control flow includes a sole in-place assignment; an invalid-path
return followed immediately by returning the original input; and assignments
whose same plain target is initialized/assigned the original input immediately
before or in a one-statement else. The branch contains no other work. Existing
if initializers, compound/double-negated conditions, different inputs,
delayed/different fallbacks, short declarations, and additional work stay
silent. The shared identity engine also rejects a named-string fallback and a
builtin-string sanitizer result when an interface destination would preserve
different dynamic types.

bytes.ToValidUTF8 is deliberately excluded. On valid input the guarded form
leaves the original slice and backing array in place, whereas an unconditional
bytes sanitizer returns independent storage; removing the guard would change
pointer, capacity, allocation, and alias behavior.

The automatic fix is deletion-only around the existing ToValidUTF8 assignment
or return. It retains the sanitizer call, replacement spelling, target, and
evaluation order byte-for-byte and removes the predicate-only unicode/utf8
import when it becomes orphaned. Comments or last required local uses in
deleted syntax keep the finding advisory.

Within the accepted domain the rewrite is BIT-IDENTICAL for valid and invalid
UTF-8, empty strings, every invalid-byte run shape, empty/valid/invalid
replacement strings, and every accepted concrete string destination.
It removes the preliminary validation scan on invalid inputs without changing
the repaired string or allocation behavior.`,
		Before: `if !utf8.ValidString(value) {
	value = strings.ToValidUTF8(value, replacement)
}`,
		After: `value = strings.ToValidUTF8(value, replacement)`,
		MeasuredWin: `benchmarks/ps5127_test.go measures a 96 KiB input with one
invalid byte near the end, so ValidString scans almost the whole payload before
ToValidUTF8 repeats the scan and repairs it. On Apple M2 Pro with Go 1.26.6
(10 runs, one CPU), removing validation reduced median time from 66,711 ns/op
to 64,771.5 ns/op (2.9%, 1.03x); both forms retained 98,304 B/op and 1
allocation because the repaired output itself is unchanged.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5127",
		Doc:  "negated utf8.ValidString redundantly guards strings.ToValidUTF8",
		Run:  runPS5127,
	},
})

type ps5127Match struct {
	flow typedGuardedFallbackTransformer
}

func runPS5127(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			statement, ok := node.(*ast.IfStmt)
			if !ok {
				return true
			}
			match, ok := ps5127GuardedSanitizer(pass, statement, parents)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos: statement.Cond.Pos(), End: match.flow.composition.transformerExpression.End(),
				Message: "utf8.ValidString scans before strings.ToValidUTF8 repeats validation and repair; ToValidUTF8 already preserves valid input, so the guard is redundant",
			}
			if fix, ok := guardedFallbackDeletionFixPaths(pass, file, []string{"unicode/utf8"}, &match.flow,
				"remove the redundant UTF-8 validation guard"); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5127GuardedSanitizer(pass *analysis.Pass, statement *ast.IfStmt, parents map[ast.Node]ast.Node) (ps5127Match, bool) {
	composition, ok := matchTypedGuardedCrossPackageTransformer(
		pass, statement,
		"unicode/utf8", "ValidString", 1,
		"strings", "ToValidUTF8", 2,
		true,
	)
	if !ok || !stableTypedValue(pass, composition.transformer.Args[1]) {
		return ps5127Match{}, false
	}
	flow, ok := matchTypedGuardedIdentityTransformer(pass, statement, parents, composition)
	if !ok {
		return ps5127Match{}, false
	}
	return ps5127Match{flow: flow}, true
}
