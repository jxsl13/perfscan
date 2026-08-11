package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/config"
	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2001 reports calls to project allocator functions inside loops. Domain
// check: the allocator vocabulary comes from the perfscan.yaml config
// (allocatorFuncs); with an empty vocabulary the check stays silent and the
// runner warns.
var PS2001 = register(&lint.Check{
	ID:          "PS2001",
	Category:    "alloc",
	Slug:        "alloc-in-loop",
	Level:       lint.LevelStructured,
	NeedsConfig: true,
	Vocab:       []string{"allocatorFuncs"},
	Doc: lint.Documentation{
		Title: "a project allocation entry point called inside a loop",
		Text: `An allocation constructor (tensor.New, Zeros, Cast, …) inside a
per-item loop allocates and initializes a fresh object every iteration.
Hoist the allocation and reuse the buffer, or take scratch from a pool.

Domain check: which functions count as allocators is project vocabulary,
supplied via the allocatorFuncs list in perfscan.yaml.`,
		Before: `for i := range batches {
	tmp := tensor.Zeros(tensor.F64, n)
	process(batches[i], tmp)
}`,
		After: `tmp := tensor.Zeros(tensor.F64, n)
for i := range batches {
	process(batches[i], tmp) // tmp fully overwritten per iteration
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2001",
		Doc:  "project allocator called inside a loop",
		Run:  runPS2001,
	},
})

func runPS2001(pass *analysis.Pass) (any, error) {
	allocators := config.Current().AllocatorFuncs
	if len(allocators) == 0 {
		return nil, nil
	}
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := astutil.CalleeName(call.Fun)
			if !allocators[name] {
				return true
			}
			if _, inLoop := astutil.InLoop(stack); !inLoop {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("allocator %s called inside a loop allocates every iteration; hoist and reuse the buffer or take scratch from a pool", name),
			})
			return true
		})
	}
	return nil, nil
}
