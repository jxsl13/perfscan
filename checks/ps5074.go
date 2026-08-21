package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5074 reports nested calls to the same standard-library Clone function.
// The innermost clone already provides the independent result promised by the
// API; cloning that result again repeats the same allocation and copy.
var PS5074 = register(&lint.Check{
	ID:       "PS5074",
	Category: "alloc",
	Slug:     "redundant-nested-stdlib-clone",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "nested bytes/strings/slices/maps.Clone repeats an already-complete clone",
		Text: `The standard library's bytes.Clone, strings.Clone, slices.Clone,
and maps.Clone each return an independent shallow copy. Passing that fresh
result immediately to the same Clone function cannot make it more independent:

  bytes.Clone(bytes.Clone(b))       -> bytes.Clone(b)
  strings.Clone(strings.Clone(s))   -> strings.Clone(s)
  slices.Clone(slices.Clone(xs))    -> slices.Clone(xs)
  maps.Clone(maps.Clone(m))         -> maps.Clone(m)

Every redundant layer repeats an O(n) copy and, for non-empty inputs, another
allocation. The check collapses an arbitrarily deep run in one diagnostic, so
Clone(Clone(Clone(x))) becomes Clone(x) directly.

The rewrite is BIT-IDENTICAL in ordinary Go semantics. The retained Clone
still evaluates the original argument once and returns storage independent of
that argument. Nil inputs remain nil; empty non-nil bytes and slices retain
their nilness; lengths, capacities, elements, string bytes, and map entries are
unchanged. Slice and map cloning remains shallow in both forms. Mutating the
returned bytes, slice, or map cannot affect the original in either form.

Callees are resolved through go/types, so aliases and explicit generic
instantiations with identical result types work while shadowed or same-named
user functions do not match. Every layer must resolve to the same package
function, use the same import binding, and return the same static type. The
type guard excludes mixed instantiations whose outer []T and inner NamedSlice
results would give an interface different dynamic types after collapsing. The
binding restriction lets the fix retain the innermost call without orphaning
an import. The supported functions are exactly bytes.Clone, strings.Clone,
slices.Clone, and maps.Clone; cross-kind conversion pipelines and user-defined
clone helpers are excluded.

The fix deletes only the redundant outer call scaffolding and keeps the
innermost Clone call byte-for-byte. If a comment occurs in deleted scaffolding,
the diagnostic remains but the automatic fix is withheld.`,
		Before: `snapshot := bytes.Clone(bytes.Clone(payload))`,
		After:  `snapshot := bytes.Clone(payload)`,
		MeasuredWin: `BenchmarkPS5074 on a 64 KiB byte slice (Apple M2 Pro,
go1.26; five runs): nested bytes.Clone median 8229 ns/op, 131072 B/op,
2 allocs/op -> one bytes.Clone median 4160 ns/op, 65536 B/op, 1 alloc/op
(~1.98x, -49.45%). Each additional redundant layer adds another full-sized
allocation and copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5074",
		Doc:  "nested standard-library Clone calls repeat an already-complete clone",
		Run:  runPS5074,
	},
})

var ps5074Packages = map[string]bool{
	"bytes":   true,
	"strings": true,
	"slices":  true,
	"maps":    true,
}

func runPS5074(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		terminalOwned := cloneCallsOwnedByTerminalConsumer(pass, file)
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if covered[outer] || terminalOwned[outer] {
				return true
			}
			matched, ok := matchRepeatedTypedUnaryPackageCall(pass, outer, ps5074Allowed)
			if !ok {
				return true
			}

			diagnostic := analysis.Diagnostic{
				Pos:     outer.Pos(),
				End:     outer.End(),
				Message: fmt.Sprintf("%s.Clone is nested %d times; one Clone already returns an independent copy, so the extra %d layer(s) repeat allocation and copying", matched.fn.Pkg().Path(), matched.layers, matched.layers-1),
			}
			if !ps2111CommentIn(file, outer.Pos(), matched.keep.Pos()) &&
				!ps2111CommentIn(file, matched.keep.End(), outer.End()) {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "remove redundant outer Clone calls",
					TextEdits: []analysis.TextEdit{
						{Pos: outer.Pos(), End: matched.keep.Pos()},
						{Pos: matched.keep.End(), End: outer.End()},
					},
				}}
			}
			pass.Report(diagnostic)
			markRepeatedTypedCall(covered, matched)
			return true // continue into the retained call's argument for independent runs
		})
	}
	return nil, nil
}

func ps5074Allowed(pkgPath, name string) bool {
	return name == "Clone" && ps5074Packages[pkgPath]
}
