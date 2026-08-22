package checks

import (
	"fmt"
	"go/ast"
	"go/constant"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5077 removes repeated strings/bytes Trim, TrimLeft, and TrimRight calls
// when every layer uses the same compile-time cutset.
var PS5077 = register(&lint.Check{
	ID:       "PS5077",
	Category: "arith",
	Slug:     "nested-idempotent-constant-trim",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings/bytes Trim is repeated with the same constant cutset",
		Text: `strings.Trim, TrimLeft, and TrimRight—and their bytes twins—are
idempotent for a fixed cutset. After one call, the relevant boundary contains
no rune from that set, so applying the same operation again cannot change the
string or slice:

  strings.Trim(strings.Trim(s, "xy"), "xy") -> strings.Trim(s, "xy")
  bytes.TrimLeft(bytes.TrimLeft(b, cutset), cutset) -> bytes.TrimLeft(b, cutset)

This check covers arbitrarily deep runs of strings.Trim/TrimLeft/TrimRight and
bytes.Trim/TrimLeft/TrimRight. Every companion cutset must be a compile-time
string with the same value; named constants and literals may mix. Dynamic
cutsets are excluded even when their source spelling repeats, because removing
a call would remove an evaluation and could hide mutation or side effects.

The rewrite is BIT-IDENTICAL. A retained Trim call evaluates the original value
once and produces the same bytes. For bytes, the retained subslice has the same
nil state, start pointer, length, and capacity: the removed outer call sees an
already-stable boundary and returns its input slice header unchanged. Invalid
UTF-8 follows the same RuneError cutset predicate in both forms. No backing
array is copied or mutated.

The generalized repeated-call matcher resolves every layer through go/types,
requires the same concrete import binding, descends through the configured
value argument, and delegates companion-argument equivalence to this rule.
Aliases work; shadowed helpers, methods, dot imports, cross-package/cross-
function compositions, unequal constants, ellipsis calls, and dynamic cutsets
do not match. Independent nested runs receive non-overlapping diagnostics.

The fix preserves the innermost call byte-for-byte and removes only redundant
outer scaffolding. A comment in deleted scaffolding—or a removed constant
expression that carries the last use of a local constant or import—withholds
the automatic fix so commentary and compilability are preserved.`,
		Before: `clean := strings.Trim(strings.Trim(payload, " \t"), " \t")`,
		After:  `clean := strings.Trim(payload, " \t")`,
		MeasuredWin: `BenchmarkPS5077 trims an already-stable short string with a
non-ASCII constant cutset (Apple M2 Pro, go1.26; five runs): two nested
strings.Trim calls median 23.76 ns/op, 0 B/op, 0 allocs/op -> one call median
12.03 ns/op, 0 B/op, 0 allocs/op (~1.98x, -49.37%). The removed layer repeats
cutset construction plus both stable-boundary membership checks.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5077",
		Doc:  "repeated strings/bytes Trim operation with the same compile-time cutset",
		Run:  runPS5077,
	},
})

var ps5077Functions = map[string]map[string]bool{
	"strings": {"Trim": true, "TrimLeft": true, "TrimRight": true},
	"bytes":   {"Trim": true, "TrimLeft": true, "TrimRight": true},
}

func runPS5077(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok || covered[outer] {
				return true
			}
			matched, ok := matchRepeatedTypedPackageCall(
				pass, outer, 2, 0, ps5077Allowed,
				func(a, b *ast.CallExpr) bool { return ps5077SameCutset(pass, a, b) },
			)
			if !ok {
				return true
			}
			pkgPath := matched.fn.Pkg().Path()
			name := matched.fn.Name()
			diagnostic := analysis.Diagnostic{
				Pos:     outer.Pos(),
				End:     outer.End(),
				Message: fmt.Sprintf("%s.%s is applied %d times with the same constant cutset; the extra %d layer(s) repeat stable-boundary work without changing the result", pkgPath, name, matched.layers, matched.layers-1),
			}
			if !ps2111CommentIn(file, outer.Pos(), matched.keep.Pos()) &&
				!ps2111CommentIn(file, matched.keep.End(), outer.End()) &&
				ps5077FixKeepsRequiredUses(pass, file, matched) {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{ps5077Fix(matched)}
			}
			pass.Report(diagnostic)
			markRepeatedTypedCall(covered, matched)
			return true
		})
	}
	return nil, nil
}

func ps5077Fix(matched repeatedTypedCall) analysis.SuggestedFix {
	return analysis.SuggestedFix{
		Message: "remove redundant trim calls",
		TextEdits: []analysis.TextEdit{
			{Pos: matched.outer.Pos(), End: matched.keep.Pos()},
			{Pos: matched.keep.End(), End: matched.outer.End()},
		},
	}
}

func ps5077Allowed(pkgPath, name string) bool {
	return ps5077Functions[pkgPath][name]
}

func ps5077SameCutset(pass *analysis.Pass, outer, inner *ast.CallExpr) bool {
	a, ok := ps5077Cutset(pass, outer.Args[1])
	if !ok {
		return false
	}
	b, ok := ps5077Cutset(pass, inner.Args[1])
	return ok && a == b
}

func ps5077Cutset(pass *analysis.Pass, expr ast.Expr) (string, bool) {
	tv, ok := pass.TypesInfo.Types[ps2110Unparen(expr)]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

// ps5077FixKeepsRequiredUses rejects a fix that would delete the last use of
// an import qualifier or function-local constant. Package constants may remain
// unused, but either of those objects would make the rewritten file fail to
// compile. Object identity handles shadowing and selector spellings exactly.
func ps5077FixKeepsRequiredUses(pass *analysis.Pass, file *ast.File, matched repeatedTypedCall) bool {
	return deletionsKeepRequiredUses(pass, file,
		tokenSpan{start: matched.outer.Pos(), end: matched.keep.Pos()},
		tokenSpan{start: matched.keep.End(), end: matched.outer.End()},
	)
}
