package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5100 flattens nested MultiReader source trees at Copy/CopyBuffer. The root
// concrete multiReader always owns Copy's WriterTo fast path, so its identity
// is never passed to a destination ReaderFrom implementation.
var PS5100 = register(&lint.Check{
	ID:       "PS5100",
	Category: "alloc",
	Slug:     "nested-multireader-source-in-copy",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "io.Copy/CopyBuffer receives a nested MultiReader source tree",
		Text: `The concrete reader returned by io.MultiReader implements io.WriterTo.
io.Copy and io.CopyBuffer always prefer a source WriterTo over a destination
ReaderFrom, so a nested MultiReader source drives the complete copy itself:

  io.Copy(dst, io.MultiReader(io.MultiReader(a, b), c))
  -> io.Copy(dst, io.MultiReader(a, b, c))

  io.CopyBuffer(dst,
      io.MultiReader(a, io.MultiReader(b, c)),
      scratch)
  -> io.CopyBuffer(dst, io.MultiReader(a, b, c), scratch)

The nested constructors allocate intermediate reader objects and argument
slices. MultiReader.WriteTo already detects nested multiReaders recursively and
reuses one copy buffer; source flattening supplies that same ordered leaf list
to one root and removes the intermediate allocations and recursion.

The rule reuses PS5098's typed constructor-tree abstraction. Copy/CopyBuffer,
the root MultiReader, and every nested constructor must resolve through
go/types to one ordinary io package binding. Aliases and parentheses work;
dot imports, function values, user lookalikes, empty/ellipsis nested calls,
MultiWriter destinations, CopyN, and arbitrary copy-like helpers remain
untouched.

The rewrite is BIT-IDENTICAL. Destination evaluation still precedes every
reader expression; readers retain depth-first left-to-right order; CopyBuffer's
buffer expression remains last. The root before and after implements WriterTo,
so Copy never passes it to user ReaderFrom code. Flattened WriteTo invokes each
underlying Reader/WriterTo and destination Write in the same order with the
same shared buffer, byte count, short-write handling, earliest error, and
stopping point. Intermediate/root mutation used for retry or early GC cannot
escape after the terminal Copy call.

CopyBuffer still evaluates and validates its supplied buffer before dispatch:
a non-nil empty buffer panics identically, while a valid buffer remains ignored
by the source WriterTo fast path in both forms. All semantic operands survive
byte-for-byte. Comments in removed scaffolding keep the finding advisory, and
the retained io.Copy/root constructor keep the import live. A branched source
tree reaches one constructor in one -fix pass.`,
		Before: `written, err := io.Copy(dst, io.MultiReader(
	io.MultiReader(first, second),
	third,
))`,
		After: `written, err := io.Copy(dst, io.MultiReader(
	first, second,
	third,
))`,
		MeasuredWin: `On an Apple M2 Pro, seven one-second single-CPU samples of
benchmarks/ps5100_test.go reduced io.Copy from a branched MultiReader tree from
a median 1,665 ns/op, 32,936 B/op, and 7 allocations to 1,500 ns/op, 32,856
B/op, and 3 allocations: about 1.11x faster while removing all 4 intermediate
adapter/slice allocations. MultiReader's required 32 KiB WriteTo buffer remains
and dominates the byte count.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5100",
		Doc:  "io.Copy/CopyBuffer source is a nested MultiReader constructor tree",
		Run:  runPS5100,
	},
})

type ps5100Match struct {
	consumer     *ast.CallExpr
	consumerName string
	nested       []typedNestedPackageCall
}

func runPS5100(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			consumer, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			match, ok := ps5100CopyTree(pass, consumer)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     match.consumer.Pos(),
				End:     match.consumer.End(),
				Message: fmt.Sprintf("io.%s uses MultiReader's WriterTo path; %d nested source adapter(s) allocate and recurse without an observable boundary", match.consumerName, len(match.nested)),
			}
			if fix, ok := fixReplacedCallScaffoldingPaths(pass, file, []string{"io"}, "flatten the nested MultiReader source tree", flattenNestedPackageCallEdits(match.nested)...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return false
		})
	}
	return nil, nil
}

func ps5100CopyTree(pass *analysis.Pass, consumer *ast.CallExpr) (ps5100Match, bool) {
	if consumer == nil || consumer.Ellipsis.IsValid() {
		return ps5100Match{}, false
	}
	fn, signature, ok := typedCallee(pass, consumer.Fun)
	if !ok || signature.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != "io" {
		return ps5100Match{}, false
	}
	if fn.Name() == "Copy" && len(consumer.Args) != 2 || fn.Name() == "CopyBuffer" && len(consumer.Args) != 3 ||
		fn.Name() != "Copy" && fn.Name() != "CopyBuffer" {
		return ps5100Match{}, false
	}
	binding, ok := typedPackageBinding(pass, consumer.Fun)
	if !ok {
		return ps5100Match{}, false
	}
	root, ok := ps2110Unparen(consumer.Args[1]).(*ast.CallExpr)
	if !ok || root.Ellipsis.IsValid() || !typedPackageCallWithBinding(pass, root, "io", "MultiReader", binding) {
		return ps5100Match{}, false
	}
	var nested []typedNestedPackageCall
	collectTypedNestedPackageCallTree(pass, root.Args, "io", "MultiReader", binding, &nested)
	if len(nested) == 0 {
		return ps5100Match{}, false
	}
	return ps5100Match{consumer: consumer, consumerName: fn.Name(), nested: nested}, true
}
