package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5107 flattens nested errors.Join trees only at errors.Is, where the fresh
// join nodes cannot escape and traversal of the ordered leaves is identical.
var PS5107 = register(&lint.Check{
	ID:       "PS5107",
	Category: "alloc",
	Slug:     "nested-errors-join-tree-in-errors-is",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "errors.Is receives a nested errors.Join tree",
		Text: `errors.Join stores its non-nil arguments in an error tree. Building
nested Join values solely for errors.Is allocates intermediate join nodes and
slices even though errors.Is traverses the same ordered leaves:

  errors.Is(errors.Join(errors.Join(a, b), c), target)
  -> errors.Is(errors.Join(a, b, c), target)

  errors.Is(errors.Join(
      errors.Join(a, b),
      errors.Join(c, d),
  ), target)
  -> errors.Is(errors.Join(a, b, c, d), target)

PS5107 uses the package-agnostic typed variadic-call tree abstraction shared by
PS5098–PS5100. The errors.Is consumer, retained errors.Join root, and every
nested Join resolve through go/types and one ordinary errors package binding.
Aliases and parentheses work; dot imports, function values, user lookalikes,
empty/ellipsis nested calls, standalone Join values, errors.As, errors.Unwrap,
and arbitrary consumers remain untouched.

The rewrite is BIT-IDENTICAL. Error expressions keep their exact depth-first
left-to-right evaluation order, followed by target. Join filters nil arguments
without invoking user code, so flattening yields the same ordered non-nil leaf
sequence and the same all-nil root. errors.Is checks the fresh root, then walks
Unwrap() []error depth-first. Join nodes define neither Is nor a single-error
Unwrap; removing an intermediate node therefore preserves every comparable
target test and every user-defined Is invocation in the same order, including
short-circuiting, panics, and non-comparable targets.

The exact errors.Is boundary is load-bearing. A standalone/returned Join has
reflectively observable Unwrap structure. errors.As can store the root or an
intermediate join into a target implementing interface{ Unwrap() []error }, so
flattening there would change the captured value even when its boolean result
agrees. Those contexts are deliberately excluded.

Only Join call punctuation is removed; all errors and target expressions remain
byte-for-byte. Comments in removed scaffolding keep the diagnostic advisory,
and the retained errors.Is/Join calls keep the import live. A branched tree
reaches one Join in one -fix pass.`,
		Before: `matched := errors.Is(errors.Join(
	errors.Join(first, second),
	third,
), target)`,
		After: `matched := errors.Is(errors.Join(
	first, second,
	third,
), target)`,
		MeasuredWin: `On an Apple M2 Pro, benchmarks/ps5107_test.go (10 runs,
single CPU) measured the branched errors.Is miss at a median 184.35 ns/op,
168 B/op, and 6 allocs/op, versus 91.335 ns/op, 88 B/op, and 2 allocs/op after
flattening: about 2.02x faster, 80 fewer bytes, and 4 fewer allocations per
operation.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5107",
		Doc:  "errors.Is target search is performed over a nested errors.Join constructor tree",
		Run:  runPS5107,
	},
})

type ps5107Match struct {
	consumer *ast.CallExpr
	nested   []typedNestedPackageCall
}

func runPS5107(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			consumer, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			match, ok := ps5107ErrorsIsTree(pass, consumer)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     match.consumer.Pos(),
				End:     match.consumer.End(),
				Message: fmt.Sprintf("errors.Is traverses a nested errors.Join tree with %d intermediate join node(s); flatten the ordered leaves and avoid their allocations", len(match.nested)),
			}
			if fix, ok := fixReplacedCallScaffoldingPaths(pass, file, []string{"errors"}, "flatten the nested errors.Join tree", flattenNestedPackageCallEdits(match.nested)...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return false
		})
	}
	return nil, nil
}

func ps5107ErrorsIsTree(pass *analysis.Pass, consumer *ast.CallExpr) (ps5107Match, bool) {
	if consumer == nil || len(consumer.Args) != 2 || consumer.Ellipsis.IsValid() {
		return ps5107Match{}, false
	}
	fn, signature, ok := typedCallee(pass, consumer.Fun)
	if !ok || signature.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != "errors" || fn.Name() != "Is" {
		return ps5107Match{}, false
	}
	binding, ok := typedPackageBinding(pass, consumer.Fun)
	if !ok {
		return ps5107Match{}, false
	}
	root, ok := ps2110Unparen(consumer.Args[0]).(*ast.CallExpr)
	if !ok || root.Ellipsis.IsValid() || !typedPackageCallWithBinding(pass, root, "errors", "Join", binding) {
		return ps5107Match{}, false
	}
	var nested []typedNestedPackageCall
	collectTypedNestedPackageCallTree(pass, root.Args, "errors", "Join", binding, &nested)
	if len(nested) == 0 {
		return ps5107Match{}, false
	}
	return ps5107Match{consumer: consumer, nested: nested}, true
}
