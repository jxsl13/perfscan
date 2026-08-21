package checks

import (
	"go/ast"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5113 collapses mixed filepath.ToSlash/FromSlash chains to their outermost
// normalizer. The outer mapping absorbs every earlier slash representation.
var PS5113 = register(&lint.Check{
	ID:       "PS5113",
	Category: "arith",
	Slug:     "absorbed-filepath-slash-normalizer-chain",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "nested filepath.ToSlash and FromSlash calls are absorbed by the outermost normalizer",
		Text: `filepath.ToSlash and filepath.FromSlash each map every path separator
byte to one chosen representation. Applying either function after any earlier
slash normalizer therefore makes the earlier choice irrelevant:

  filepath.ToSlash(filepath.ToSlash(path))    -> filepath.ToSlash(path)
  filepath.ToSlash(filepath.FromSlash(path))  -> filepath.ToSlash(path)
  filepath.FromSlash(filepath.ToSlash(path))  -> filepath.FromSlash(path)
  filepath.FromSlash(filepath.FromSlash(path))-> filepath.FromSlash(path)

The identity holds for arbitrarily deep mixed chains. On Windows, ToSlash maps
both separator spellings to '/', while FromSlash maps both to '\\'; the
outermost mapping completely determines the result. On Unix and Plan 9 the OS
separator is already '/', so both functions return the input and the same
identity follows trivially.

When a mixed chain ends in a filepath producer whose result cannot contain a
forward slash, an outermost FromSlash restores that producer result exactly
and the complete chain disappears:

  filepath.FromSlash(filepath.ToSlash(filepath.Base(path))) -> filepath.Base(path)

PS5114 owns a pure FromSlash chain around such a producer. PS5113 owns the
mixed form, including any nested PS5114 candidate, so fix mode emits one
non-overlapping edit and reaches the shared fixed point in one pass. Clean and
Join do not use this stronger collapse because Windows volume-like inputs can
leave a leading slash in their results; their outer FromSlash is retained while
only the nested slash normalizers are removed.

The shared typed unary call-chain abstraction resolves every layer through
go/types and accepts only package-level path/filepath.ToSlash and FromSlash
calls. Every call result and recursive argument must have the identical
concrete string type. Ordinary aliases and parentheses work; dot imports,
function values, methods, shadowed lookalikes, explicit type changes, and
other intervening calls stop the chain. Independent chains inside a retained
base expression receive separate non-overlapping diagnostics.

The rewrite is BIT-IDENTICAL on every supported GOOS. It retains the outermost
normalizer and the deepest input expression byte-for-byte, so the input is
still evaluated exactly once and the result remains the built-in string type.
Only inner pure normalizer scaffolding disappears. A comment inside that
scaffolding keeps the finding advisory.

On slash-separator systems the standard library and compiler already reduce
both forms to identity work, so no portable speedup is claimed there. The
performance benefit is for Windows and for cross-platform code that later
runs on Windows: each removed layer avoids an IndexByte scan and, when it
changes a separator, a full string allocation and rewrite.`,
		Before: `portable := filepath.ToSlash(filepath.FromSlash(filepath.ToSlash(path)))`,
		After:  `portable := filepath.ToSlash(path)`,
		MeasuredWin: `benchmarks/ps5113_test.go executes Go 1.26's exact
filepathlite replacement algorithm with the Windows '\\' separator over a
92 KiB mixed-separator path. On an Apple M2 Pro (10 runs, one CPU), nested
ToSlash(FromSlash(path)) measured a median 98,451 ns/op, 393,216 B/op, and 4
allocs/op; ToSlash(path) measured 48,031 ns/op, 196,608 B/op, and 2 allocs/op:
about 2.05x faster with half the bytes and allocations.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5113",
		Doc:  "mixed filepath.ToSlash/FromSlash chain is fully determined by its outermost call",
		Run:  runPS5113,
	},
})

func runPS5113(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok || covered[outer] {
				return true
			}
			chain, ok := ps5113SlashChain(pass, outer)
			if !ok {
				return true
			}
			if _, ownedByPS5114 := ps5114NativeFilepathProducerChain(pass, outer); ownedByPS5114 {
				return true
			}
			for _, call := range chain.calls {
				covered[call] = true
			}
			outerFunction, _, _ := typedCallee(pass, outer.Fun)
			removed := len(chain.calls) - 1
			binding, _ := typedPackageBinding(pass, outer.Fun)
			nativeProducer := ps5114NativeFilepathProducerExpression(pass, chain.base, binding)
			removeOuter := outerFunction.Name() == "FromSlash" && nativeProducer
			message := "filepath." + outerFunction.Name() + " absorbs " + strconv.Itoa(removed) + " nested ToSlash/FromSlash layer(s); the outermost slash representation fully determines the result"
			if removeOuter {
				message = "filepath.FromSlash restores the already-native filepath producer after " + strconv.Itoa(removed) + " mixed slash-normalizer layer(s); retain the producer directly"
			}
			diagnostic := analysis.Diagnostic{
				Pos: outer.Pos(), End: outer.End(),
				Message: message,
			}
			spans := chain.spans
			if !removeOuter {
				firstExpression := outer.Args[0]
				spans = []tokenSpan{
					{start: firstExpression.Pos(), end: chain.base.Pos()},
					{start: chain.base.End(), end: firstExpression.End()},
				}
			}
			if fix, ok := fixDeletedCallScaffolding(pass, file, "path/filepath", "remove absorbed filepath slash normalizers", spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5113SlashChain(pass *analysis.Pass, outer *ast.CallExpr) (typedUnaryCallChain, bool) {
	chain, ok := matchTypedUnaryPackageCallChain(pass, outer, func(pkgPath, name string) bool {
		return pkgPath == "path/filepath" && (name == "ToSlash" || name == "FromSlash")
	})
	return chain, ok && len(chain.calls) >= 2
}
