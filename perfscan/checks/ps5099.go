package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5099 reuses PS5098's typed constructor-tree flattener at exact io package
// consumers that cannot retain or otherwise expose the adapter identity.
var PS5099 = register(&lint.Check{
	ID:       "PS5099",
	Category: "alloc",
	Slug:     "nested-io-multi-tree-in-nonretaining-consumer",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a non-retaining io consumer receives a nested MultiReader/MultiWriter tree",
		Text: `Several io package helpers consume only one interface behavior and
never expose or retain the supplied adapter. Nested MultiReader/MultiWriter
constructors at those boundaries allocate intermediate adapters and argument
slices that the consumer cannot observe:

  io.ReadAll(io.MultiReader(io.MultiReader(a, b), c))
  -> io.ReadAll(io.MultiReader(a, b, c))

  io.ReadFull(io.MultiReader(io.MultiReader(a, b), c), buf)
  -> io.ReadFull(io.MultiReader(a, b, c), buf)

  io.ReadAtLeast(io.MultiReader(io.MultiReader(a, b), c), buf, min)
  -> io.ReadAtLeast(io.MultiReader(a, b, c), buf, min)

  io.WriteString(io.MultiWriter(io.MultiWriter(a, b), c), text)
  -> io.WriteString(io.MultiWriter(a, b, c), text)

The rule shares PS5098's typed, comment-safe constructor-tree abstraction. It
resolves the outer consumer, retained root constructor, and every nested
constructor through go/types and one ordinary io package binding. Aliases and
parentheses work. Dot imports, function values, user lookalikes, empty or
ellipsis nested calls, cross-family trees, io.Copy/CopyN, and arbitrary
reader/writer consumers remain untouched.

The rewrite is BIT-IDENTICAL. Reader/writer expressions retain their exact
depth-first left-to-right evaluation order, followed by the consumer's
remaining arguments. MultiReader flattening preserves every individual Read
result, including EOF suppression, positive-n-with-EOF, zero progress, custom
errors, and underlying state. Consequently ReadAll performs the same loop and
returns the same bytes, error, nilness, length, and capacity; ReadFull and
ReadAtLeast derive the same count and EOF/ErrUnexpectedEOF/ErrShortBuffer
result. MultiWriter's root receives the same ordered writer list, and
io.WriteString dispatches to its WriteString method, preserving StringWriter
selection, bytes, counts, short writes, and errors.

The exact consumer allowlist is load-bearing. io.Copy and CopyN may pass a
reader or writer to user-defined WriterTo/ReaderFrom implementations, which can
inspect or retain adapter structure. General calls can retain their interface
argument too. Those shapes are deliberately excluded.

Only nested constructor punctuation is deleted; all semantic operands survive
byte-for-byte. Comments in removed scaffolding keep the diagnostic advisory,
and the retained io consumer/root keep the import live. A branched tree reaches
its flat fixed point in one -fix pass.`,
		Before: `data, err := io.ReadAll(io.MultiReader(
	io.MultiReader(first, second),
	third,
))`,
		After: `data, err := io.ReadAll(io.MultiReader(
	first, second,
	third,
))`,
		MeasuredWin: `On an Apple M2 Pro, seven 300 ms samples of
benchmarks/ps5099_test.go reduced io.WriteString over a branched MultiWriter
tree from a median 146.9 ns/op, 232 B/op, and 7 allocations to 56.83 ns/op,
88 B/op, and 2 allocations: about 2.58x faster while eliminating 144 bytes and
all 5 intermediate adapter/slice allocations.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5099",
		Doc:  "non-retaining io consumer receives a nested MultiReader/MultiWriter constructor tree",
		Run:  runPS5099,
	},
})

type ps5099ConsumerSpec struct {
	constructor string
	arity       int
}

var ps5099Consumers = map[string]ps5099ConsumerSpec{
	"ReadAll":     {constructor: "MultiReader", arity: 1},
	"ReadFull":    {constructor: "MultiReader", arity: 2},
	"ReadAtLeast": {constructor: "MultiReader", arity: 3},
	"WriteString": {constructor: "MultiWriter", arity: 2},
}

type ps5099Match struct {
	consumer     *ast.CallExpr
	consumerName string
	constructor  string
	nested       []typedNestedPackageCall
}

func runPS5099(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			consumer, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			match, ok := ps5099ConsumerTree(pass, consumer)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     match.consumer.Pos(),
				End:     match.consumer.End(),
				Message: fmt.Sprintf("io.%s consumes only the flattened behavior of io.%s; %d nested adapter(s) allocate with no observable boundary", match.consumerName, match.constructor, len(match.nested)),
			}
			if fix, ok := fixReplacedCallScaffoldingPaths(pass, file, []string{"io"}, "flatten nested io multi-adapter constructors at the non-retaining consumer", flattenNestedPackageCallEdits(match.nested)...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return false
		})
	}
	return nil, nil
}

func ps5099ConsumerTree(pass *analysis.Pass, consumer *ast.CallExpr) (ps5099Match, bool) {
	if consumer == nil || consumer.Ellipsis.IsValid() {
		return ps5099Match{}, false
	}
	fn, signature, ok := typedCallee(pass, consumer.Fun)
	if !ok || signature.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != "io" {
		return ps5099Match{}, false
	}
	spec, ok := ps5099Consumers[fn.Name()]
	if !ok || len(consumer.Args) != spec.arity {
		return ps5099Match{}, false
	}
	binding, ok := typedPackageBinding(pass, consumer.Fun)
	if !ok {
		return ps5099Match{}, false
	}
	root, ok := ps2110Unparen(consumer.Args[0]).(*ast.CallExpr)
	if !ok || root.Ellipsis.IsValid() || !typedPackageCallWithBinding(pass, root, "io", spec.constructor, binding) {
		return ps5099Match{}, false
	}
	var nested []typedNestedPackageCall
	collectTypedNestedPackageCallTree(pass, root.Args, "io", spec.constructor, binding, &nested)
	if len(nested) == 0 {
		return ps5099Match{}, false
	}
	return ps5099Match{
		consumer: consumer, consumerName: fn.Name(), constructor: spec.constructor, nested: nested,
	}, true
}
