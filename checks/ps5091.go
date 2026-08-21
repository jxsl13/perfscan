package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS5091 removes strings.Clone chains consumed only by non-retaining index,
// lookup, and delete operations.
var PS5091 = register(&lint.Check{
	ID:       "PS5091",
	Category: "alloc",
	Slug:     "string-clone-fed-nonretaining-index-lookup",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a string byte index or map-key lookup consumes a throwaway clone",
		Text: `strings.Clone forces an allocation and copies every byte. That copy
is wasted when the immediate consumer cannot retain the cloned string:

  strings.Clone(text)[index]        -> text[index]
  values[strings.Clone(key)]        -> values[key]
  delete(values, strings.Clone(key)) -> delete(values, key)

This check resolves strings.Clone and the delete builtin through go/types and
unwraps arbitrarily deep Clone chains in one fix. Read-only map indexing works
for every map key type to which string is assignable, including interface
keys. A map lookup nested under a further selector or index remains a read:
values[strings.Clone(key)].Field does not store the lookup key. Aliased
strings imports work, while dot imports, function values, user Clone methods,
shadowed delete identifiers, and type-changing wrappers stay untouched.

Map storage is deliberately excluded. In
values[strings.Clone(key)] = value, values[strings.Clone(key)]++, compound
assignments, and assignment-form range clauses, the map may retain a newly
inserted key. There strings.Clone can be an intentional retention boundary
that detaches a short substring from a large backing string. String slicing is
excluded for the same reason: strings.Clone(text)[low:high] may intentionally
keep the result from retaining text's backing storage.

The rewrite is BIT-IDENTICAL for race-free Go programs. String bytes,
comparison and hashing, nil-map lookup results, delete behavior, bounds
panics, evaluation count, and evaluation order are unchanged; only the forced
copy disappears. Parent-aware AST classification distinguishes reads from
storage contexts before offering a fix. Comments keep the finding advisory,
and the shared import-liveness editor safely removes newly unused strings
imports. Terminal ownership also prevents overlapping nested-Clone fixes, so
one -fix pass reaches the allocation-free form.`,
		Before: `b := strings.Clone(text)[index]
value := values[strings.Clone(key)]
delete(values, strings.Clone(key))`,
		After: `b := text[index]
value := values[key]
delete(values, key)`,
		MeasuredWin: `On Apple M2 Pro, indexing one byte from a 68,980-byte
strings.Clone result measured 4,361 ns/op, 73,728 B/op, 1 alloc/op versus a
direct string index at 1.930 ns/op, 0 B/op, 0 allocs/op (median of five 200ms
runs): about 2,260x faster and more than 99.9% less time.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5091",
		Doc:  "throwaway strings.Clone immediately feeds a non-retaining index, lookup, or delete",
		Run:  runPS5091,
	},
})

func runPS5091(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		astutil.WithStack(file, func(node ast.Node, stack []ast.Node) bool {
			var (
				chain typedUnaryCallChain
				kind  string
				ok    bool
			)
			switch value := node.(type) {
			case *ast.IndexExpr:
				chain, kind, ok = ps5091IndexMatch(pass, value, stack)
			case *ast.CallExpr:
				chain, kind, ok = ps5091DeleteMatch(pass, value)
			}
			if !ok {
				return true
			}

			diagnostic := analysis.Diagnostic{
				Pos:     node.Pos(),
				End:     node.End(),
				Message: fmt.Sprintf("%s consumes %d throwaway strings.Clone layer(s); use the original string directly", kind, len(chain.calls)),
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, chain.paths, "remove clones before non-retaining index or lookup", chain.spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5091IndexMatch(pass *analysis.Pass, index *ast.IndexExpr, stack []ast.Node) (typedUnaryCallChain, string, bool) {
	if ps5084StringType(pass.TypesInfo.TypeOf(index.X)) {
		chain, ok := matchTypedUnaryPackageCallChain(pass, index.X, isTypedStringStdlibClone)
		return chain, "string byte index", ok
	}

	containerType := pass.TypesInfo.TypeOf(index.X)
	if containerType == nil {
		return typedUnaryCallChain{}, "", false
	}
	if _, ok := containerType.Underlying().(*types.Map); !ok || ps5091MapIndexStores(index, stack) {
		return typedUnaryCallChain{}, "", false
	}
	chain, ok := matchTypedUnaryPackageCallChain(pass, index.Index, isTypedStringStdlibClone)
	return chain, "read-only map lookup key", ok
}

// ps5091MapIndexStores reports whether index itself is the storage target.
// An index nested beneath another selector/index is a read that obtains the
// pointer or slice subsequently mutated, so only an exact LHS/IncDec/range
// target is rejected. Parentheses around the target are transparent.
func ps5091MapIndexStores(index *ast.IndexExpr, stack []ast.Node) bool {
	parent := len(stack) - 1
	for parent >= 0 {
		if _, ok := stack[parent].(*ast.ParenExpr); ok {
			parent--
			continue
		}
		break
	}
	if parent < 0 {
		return false
	}
	switch value := stack[parent].(type) {
	case *ast.AssignStmt:
		for _, lhs := range value.Lhs {
			if ps2110Unparen(lhs) == index {
				return true
			}
		}
	case *ast.IncDecStmt:
		return ps2110Unparen(value.X) == index
	case *ast.RangeStmt:
		if value.Tok != token.ASSIGN {
			return false
		}
		return ps2110Unparen(value.Key) == index || ps2110Unparen(value.Value) == index
	}
	return false
}

func ps5091DeleteMatch(pass *analysis.Pass, call *ast.CallExpr) (typedUnaryCallChain, string, bool) {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() || !typedBuiltinName(pass, call.Fun, "delete") {
		return typedUnaryCallChain{}, "", false
	}
	containerType := pass.TypesInfo.TypeOf(call.Args[0])
	if containerType == nil {
		return typedUnaryCallChain{}, "", false
	}
	if _, ok := containerType.Underlying().(*types.Map); !ok {
		return typedUnaryCallChain{}, "", false
	}
	chain, ok := matchTypedUnaryPackageCallChain(pass, call.Args[1], isTypedStringStdlibClone)
	return chain, "delete map key", ok
}
