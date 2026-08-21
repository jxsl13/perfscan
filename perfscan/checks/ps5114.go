package checks

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5114 removes filepath.FromSlash layers around filepath producers that
// already return native-separator paths or separator-free path elements.
var PS5114 = register(&lint.Check{
	ID:       "PS5114",
	Category: "arith",
	Slug:     "fromslash-around-native-filepath-producer",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "filepath.FromSlash rescans a result that is already in native filepath form",
		Text: `Several path/filepath producers already return strings on which
filepath.FromSlash is an identity:

  filepath.FromSlash(filepath.Clean(name))       -> filepath.Clean(name)
  filepath.FromSlash(filepath.Join(root, tail))  -> filepath.Join(root, tail)
  filepath.FromSlash(filepath.Dir(name))         -> filepath.Dir(name)
  filepath.FromSlash(filepath.Base(name))        -> filepath.Base(name)
  filepath.FromSlash(filepath.Ext(name))         -> filepath.Ext(name)
  filepath.FromSlash(filepath.VolumeName(name))  -> filepath.VolumeName(name)

On Windows, Clean finishes by converting every slash to the native separator;
Join finishes with Clean, and Dir cleans the directory portion it returns.
VolumeName applies the same conversion itself. Base and Ext return only the
final separator-delimited element (or, for an all-separator Base input, one
native separator), so their results cannot contain '/'. On every system whose
native separator is '/', FromSlash is an identity for all strings. Empty
results from Join, Ext, or VolumeName remain empty, so no nonempty proof is
needed.

The rule uses the shared typed repeated-wrapper/fixed-point-producer
abstraction. It follows arbitrarily many FromSlash layers and removes them in
one fix. Wrapper and producer must resolve through go/types to the same
ordinary path/filepath import binding, every intermediate result must have the
same concrete string type, and the producer must be Clean, Join, Dir, Base,
Ext, or VolumeName. Aliases, parentheses, variadic Join calls, and Join calls
with no elements work. Dot imports, function values, methods, user lookalikes,
path package calls, explicit type changes, and other producers stay silent.

The rewrite is BIT-IDENTICAL on every supported GOOS. It retains the complete
producer expression byte-for-byte, including all argument evaluation, and
deletes only pure outer FromSlash scaffolding. Comments or local/import uses
inside removed syntax keep the diagnostic advisory through the shared
call-chain editor.

On slash-separator systems the standard library and compiler reduce
FromSlash to identity work, so no portable speedup is claimed. On Windows each
removed layer avoids an IndexByte scan of the complete producer result. The
benefit is largest for long Clean, Join, or Dir results; the rule remains a
small correctness-preserving simplification for short element results.`,
		Before: `native := filepath.FromSlash(filepath.Clean(name))`,
		After:  `native := filepath.Clean(name)`,
		MeasuredWin: `benchmarks/ps5114_test.go isolates the deleted Windows
operation with Go 1.26's exact filepathlite FromSlash replacement algorithm
over a precomputed canonical 92 KiB native-separator producer result. On an
Apple M2 Pro (10 runs, one CPU), the redundant scan measured a median 1,568
ns/op; retaining the already-produced string directly measured 1.892 ns/op.
Both forms used 0 B/op and 0 allocs/op. The rule therefore removes about 1.57
microseconds per 92 KiB result; end-to-end percentage depends on the retained
producer's own cost.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5114",
		Doc:  "filepath.FromSlash wraps a filepath producer whose result is already in native-separator form",
		Run:  runPS5114,
	},
})

func runPS5114(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ownedByMixedSlashChain := ps5114CallsOwnedByMixedSlashChain(pass, file)
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			root, ok := node.(*ast.CallExpr)
			if !ok || covered[root] || ownedByMixedSlashChain[root] {
				return true
			}
			match, ok := ps5114NativeFilepathProducerChain(pass, root)
			if !ok {
				return true
			}
			for _, wrapper := range match.wrappers {
				covered[wrapper] = true
			}
			diagnostic := analysis.Diagnostic{
				Pos: match.outer.Pos(), End: match.outer.End(),
				Message: fmt.Sprintf("filepath.FromSlash rescans the already-native result of filepath.%s through %d redundant FromSlash layer(s); retain the producer directly", match.producerFunction.Name(), len(match.wrappers)),
			}
			spans := []tokenSpan{
				{start: match.outer.Pos(), end: match.producerExpression.Pos()},
				{start: match.producerExpression.End(), end: match.outer.End()},
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, []string{"path/filepath"}, "remove FromSlash around the native filepath producer", spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5114NativeFilepathProducerChain(pass *analysis.Pass, root *ast.CallExpr) (typedPackageWrapperProducerChain, bool) {
	return matchTypedPackageWrapperProducerChain(pass, root, "path/filepath", "FromSlash", func(
		producer *types.Func,
		producerSignature *types.Signature,
		call *ast.CallExpr,
	) bool {
		return ps5114AcceptNativeFilepathProducer(producer, producerSignature, call)
	})
}

func ps5114AcceptNativeFilepathProducer(producer *types.Func, signature *types.Signature, call *ast.CallExpr) bool {
	if producer == nil || signature == nil || call == nil || signature.Recv() != nil || producer.Pkg() == nil || producer.Pkg().Path() != "path/filepath" {
		return false
	}
	switch producer.Name() {
	case "Clean", "Dir", "Base", "Ext", "VolumeName":
		return len(call.Args) == 1 && !call.Ellipsis.IsValid()
	case "Join":
		return true
	default:
		return false
	}
}

func ps5114NativeFilepathProducerExpression(pass *analysis.Pass, expression ast.Expr, binding *types.PkgName) bool {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok {
		return false
	}
	producer, signature, ok := typedCallee(pass, call.Fun)
	producerBinding, bindingOK := typedPackageBinding(pass, call.Fun)
	return ok && bindingOK && producerBinding == binding && ps5114AcceptNativeFilepathProducer(producer, signature, call)
}

// A mixed ToSlash/FromSlash chain owns a nested PS5114 composition because
// PS5113 deletes that nested wrapper as part of its larger one-shot rewrite.
// Pure FromSlash chains remain PS5114's responsibility. This avoids duplicate
// diagnostics and overlapping fixes while both analyzers remain useful alone.
func ps5114CallsOwnedByMixedSlashChain(pass *analysis.Pass, file *ast.File) map[*ast.CallExpr]bool {
	owned := make(map[*ast.CallExpr]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		root, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		chain, ok := ps5113SlashChain(pass, root)
		if !ok {
			return true
		}
		if _, pureFromSlash := ps5114NativeFilepathProducerChain(pass, root); pureFromSlash {
			return true
		}
		for _, nested := range chain.calls[1:] {
			if _, match := ps5114NativeFilepathProducerChain(pass, nested); match {
				owned[nested] = true
			}
		}
		return true
	})
	return owned
}
