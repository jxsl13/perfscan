package checks

import (
	"fmt"
	"go/ast"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5115 removes strings.ToValidUTF8 layers outside a call whose valid
// replacement already guarantees that the retained result is valid UTF-8.
var PS5115 = register(&lint.Check{
	ID:       "PS5115",
	Category: "arith",
	Slug:     "tovalidutf8-around-proven-valid-result",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.ToValidUTF8 rescans the already-valid result of another ToValidUTF8 call",
		Text: `strings.ToValidUTF8 guarantees valid UTF-8 whenever its replacement
is itself valid UTF-8. Applying ToValidUTF8 again to that result cannot find an
invalid encoding and returns the same string:

  strings.ToValidUTF8(strings.ToValidUTF8(s, "?"), "�") ->
  strings.ToValidUTF8(s, "?")

The outer replacement need not be valid because it is never inserted; it must
only be a compile-time string constant so deleting the outer call cannot
suppress side effects. Empty strings, literals with escaped invalid bytes,
named constants, conversions of constants, and different replacement values
are all analyzed by value. The retained call's replacement must pass
utf8.ValidString. An invalid retained replacement is a real counterexample:
an inner call may emit invalid bytes that the outer call then repairs.

Arbitrarily deep chains collapse to the deepest call that proves the valid-UTF8
postcondition. If a deeper call has an invalid replacement, the new shared
terminal-postcondition matcher retains the first valid enclosing sanitizer and
still removes every redundant layer outside it in one pass. Companion-argument
compatibility is checked separately, so a dynamic outer replacement keeps its
evaluation and blocks deletion across that layer.

The matcher resolves every call through go/types, requires the same ordinary
strings import binding, exact arity and concrete result type, and follows only
argument zero. Aliases and parentheses work. Dot imports, function values,
methods, user lookalikes, bytes.ToValidUTF8, ellipsis calls, and changed import
bindings stay silent. bytes.ToValidUTF8 is deliberately excluded because it
always returns a fresh slice; deleting an outer call would change observable
slice allocation, capacity, and aliasing.

The rewrite is BIT-IDENTICAL for Go string values. It preserves the retained
call and original input byte-for-byte and deletes only outer sanitizer
scaffolding plus compile-time replacement expressions. Input evaluation stays
exactly once and in the same argument position. Comments, last-use local
constants, and last-use imported constant qualifiers keep the diagnostic
advisory through the shared deletion-safety checks.`,
		Before: `clean := strings.ToValidUTF8(strings.ToValidUTF8(payload, "�"), "?")`,
		After:  `clean := strings.ToValidUTF8(payload, "�")`,
		MeasuredWin: `benchmarks/ps5115_test.go measures a 96 KiB invalid-byte
payload whose retained sanitizer expands to a valid 192 KiB string. On an
Apple M2 Pro (10 runs, one CPU), nested ToValidUTF8 measured a median 632,393
ns/op versus 335,981 ns/op for the retained sanitizer alone: about 1.88x
faster and 46.9% less time. Both forms used 655,360 B/op and 4 allocs/op because
the removed outer valid-input scan already returned its input without
allocating; the gain is the eliminated full UTF-8 decode pass.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5115",
		Doc:  "strings.ToValidUTF8 wraps a call whose valid replacement already guarantees valid UTF-8",
		Run:  runPS5115,
	},
})

func runPS5115(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ownedByTerminalValidation := ps5116OwnedStringSanitizers(pass, file)
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok || covered[outer] || ownedByTerminalValidation[outer] {
				return true
			}
			matched, ok := ps5115ValidUTF8Chain(pass, outer)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos: matched.outer.Pos(), End: matched.outer.End(),
				Message: fmt.Sprintf("strings.ToValidUTF8 is applied %d times even though the retained call's valid replacement already guarantees valid UTF-8; remove %d redundant rescan layer(s)", matched.layers, matched.layers-1),
			}
			spans := []tokenSpan{
				{start: matched.outer.Pos(), end: matched.keep.Pos()},
				{start: matched.keep.End(), end: matched.outer.End()},
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, []string{"strings"}, "remove ToValidUTF8 layers around the proven-valid result", spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			markRepeatedTypedCall(covered, matched)
			return true
		})
	}
	return nil, nil
}

func ps5115ValidUTF8Chain(pass *analysis.Pass, outer *ast.CallExpr) (repeatedTypedCall, bool) {
	return matchRepeatedTypedPackageCallEndingIn(
		pass, outer, 2, 0,
		func(pkgPath, name string) bool { return pkgPath == "strings" && name == "ToValidUTF8" },
		func(outer, _ *ast.CallExpr) bool {
			_, ok := ps5077Cutset(pass, outer.Args[1])
			return ok
		},
		func(call *ast.CallExpr) bool {
			replacement, ok := ps5077Cutset(pass, call.Args[1])
			return ok && utf8.ValidString(replacement)
		},
	)
}
