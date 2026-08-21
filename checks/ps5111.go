package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5111 removes path/filepath Clean layers around producers whose result is
// already a canonical, nonempty fixed point for Clean.
var PS5111 = register(&lint.Check{
	ID:       "PS5111",
	Category: "arith",
	Slug:     "clean-around-canonical-path-producer",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "path.Clean or filepath.Clean rescans an already canonical standard-library result",
		Text: `Several path producers already return a canonical, nonempty value.
Wrapping them in path.Clean or filepath.Clean repeats lexical path processing
without changing a byte:

  path.Clean(path.Dir(name))                    -> path.Dir(name)
  path.Clean(path.Base(name))                   -> path.Base(name)
  filepath.Clean(filepath.Dir(name))            -> filepath.Dir(name)
  filepath.Clean(filepath.Base(name))           -> filepath.Base(name)
  path.Clean(path.Join(root, "fixed"))          -> path.Join(root, "fixed")
  filepath.Clean(filepath.Join("fixed", tail))  -> filepath.Join("fixed", tail)

Dir explicitly cleans the directory it returns and never returns an empty
string. Base returns either a separator/root, ".", "..", or one separator-free
path element, all nonempty Clean fixed points. Join explicitly cleans its
result, but has one exception: no arguments or all-empty arguments return "",
whereas Clean("") returns ".". PS5111 therefore removes Clean around Join only
when at least one Join argument is a compile-time nonempty string constant.
Other arguments may remain dynamic and side-effecting; the retained Join still
evaluates every argument exactly once in the original order.

The rule uses the shared typed repeated-wrapper/fixed-point-producer
abstraction. It follows arbitrarily many Clean layers and removes them all in
one fix. Wrapper and producer must resolve through go/types to the same ordinary
path or path/filepath package binding, every intermediate result must have the
same concrete string type, and the producer shape must satisfy the rule above.
Aliases, parentheses, named constants, and constant string expressions work. Dot imports,
function values, methods, user lookalikes, cross-package compositions, empty or
all-dynamic Join calls, and type mismatches stay silent.

The rewrite is BIT-IDENTICAL on every supported OS. Path Dir/Join call Clean
directly. Filepath Dir/Join use filepathlite or platform join logic ending in
Clean, including Windows volume/UNC rules. Base removes the volume and final
separator-delimited prefix before returning one stable element. Because every
accepted producer result is nonempty, Clean's empty-string special case cannot
apply. Removing the pure outer scans cannot move, duplicate, or suppress any
producer argument evaluation, panic, or allocation.

The fix retains the complete producer expression byte-for-byte and deletes only
outer Clean scaffolding. Comments or local/import uses inside removed syntax
keep the diagnostic advisory through the shared call-chain editor.`,
		Before: `directory := filepath.Clean(filepath.Clean(filepath.Dir(name)))`,
		After:  `directory := filepath.Dir(name)`,
		MeasuredWin: `On an Apple M2 Pro, benchmarks/ps5111_test.go (10 runs,
single CPU) measured Clean(Dir(path)) on a canonical 72 KiB path at a median
149,831 ns/op, versus 73,504 ns/op for Dir(path): about 2.04x faster. Both
forms used 0 B/op and 0 allocs/op; the entire delta is the eliminated second
lexical scan.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5111",
		Doc:  "path/filepath Clean wraps a canonical nonempty Dir, Base, or proven-nonempty Join result",
		Run:  runPS5111,
	},
})

func runPS5111(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			root, ok := node.(*ast.CallExpr)
			if !ok || covered[root] {
				return true
			}
			match, ok := ps5111CleanedProducerChain(pass, root)
			if !ok {
				return true
			}
			for _, wrapper := range match.wrappers {
				covered[wrapper] = true
			}
			pkgPath := match.wrapper.Pkg().Path()
			diagnostic := analysis.Diagnostic{
				Pos: match.outer.Pos(), End: match.outer.End(),
				Message: fmt.Sprintf("%s.Clean rescans the canonical nonempty result of %s.%s through %d redundant Clean layer(s); retain the producer directly", pkgPath, pkgPath, match.producerFunction.Name(), len(match.wrappers)),
			}
			spans := []tokenSpan{
				{start: match.outer.Pos(), end: match.producerExpression.Pos()},
				{start: match.producerExpression.End(), end: match.outer.End()},
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, []string{pkgPath}, "remove Clean around the canonical path producer", spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5111CleanedProducerChain(pass *analysis.Pass, root *ast.CallExpr) (typedPackageWrapperProducerChain, bool) {
	function, signature, ok := typedCallee(pass, root.Fun)
	if !ok || signature.Recv() != nil || function.Pkg() == nil || function.Name() != "Clean" {
		return typedPackageWrapperProducerChain{}, false
	}
	pkgPath := function.Pkg().Path()
	if pkgPath != "path" && pkgPath != "path/filepath" {
		return typedPackageWrapperProducerChain{}, false
	}
	return matchTypedPackageWrapperProducerChain(pass, root, pkgPath, "Clean", func(
		producer *types.Func,
		producerSignature *types.Signature,
		call *ast.CallExpr,
	) bool {
		if producerSignature.Recv() != nil || producer.Pkg() == nil || producer.Pkg().Path() != pkgPath {
			return false
		}
		switch producer.Name() {
		case "Dir", "Base":
			return len(call.Args) == 1 && !call.Ellipsis.IsValid()
		case "Join":
			for _, argument := range call.Args {
				value, ok := pass.TypesInfo.Types[ps2110Unparen(argument)]
				if ok && value.Value != nil && value.Value.Kind() == constant.String && constant.StringVal(value.Value) != "" {
					return true
				}
			}
		}
		return false
	})
}
