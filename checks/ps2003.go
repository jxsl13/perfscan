package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2003 reports allocating strings transforms inside loops.
var PS2003 = register(&lint.Check{
	ID:       "PS2003",
	Category: "alloc",
	Slug:     "strings-alloc-in-loop",
	Level:    lint.LevelIdiomatic,
	Doc: lint.Documentation{
		Title: "an allocating strings transform (Replace/Map/Repeat) in a loop",
		Text: `strings.Replace, ReplaceAll, Map and Repeat allocate a fresh
result string per call. In a loop that is one allocation (often more, via
internal growth) per iteration. Hoist the transform when its inputs are
loop-invariant; for repeated replacements build a strings.Replacer once; for
byte-level work use a reused []byte buffer.

Advisory: whether the inputs are invariant is often a data question, so the
rewrite is left to the reader.`,
		Before: `for _, name := range names {
	clean := strings.ReplaceAll(name, "-", "_")
	emit(clean)
}`,
		After: `r := strings.NewReplacer("-", "_")
for _, name := range names {
	emit(r.Replace(name))
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2003",
		Doc:  "allocating strings transform in a loop",
		Run:  runPS2003,
	},
})

var stringsAllocFuncs = map[string]bool{
	"Replace":    true,
	"ReplaceAll": true,
	"Map":        true,
	"Repeat":     true,
}

func runPS2003(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := astutil.PkgFuncCall(call.Fun, "strings", stringsAllocFuncs)
			if !ok {
				return true
			}
			if _, inLoop := astutil.InLoop(stack); !inLoop {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("strings.%s in a loop allocates a fresh string per iteration; hoist the transform, build a strings.Replacer once, or reuse a byte buffer", name),
			})
			return true
		})
	}
	return nil, nil
}
