package checks

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3003 reports reads of integer-keyed maps inside loops.
var PS3003 = register(&lint.Check{
	ID:       "PS3003",
	Category: "indirect",
	Slug:     "int-key-map-in-loop",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "a read of an integer-keyed map inside a loop",
		Text: `Every probe of a map hashes the key and walks a bucket. When the
keys are dense integers in a known range — indices, ids, offsets — a slice
is the natural container: the probe becomes one bounds-checked load.

Advisory (L2): densification changes the container type and the reset
strategy (a generation counter replaces per-pass clears), which is a
restructuring, not a mechanical edit. Sparse or unbounded key spaces should
keep the map.`,
		Before: `seen := map[int]bool{}
for _, id := range ids {
	if seen[id] { continue }
	seen[id] = true
}`,
		After: `seen := make([]bool, maxID+1)
for _, id := range ids {
	if seen[id] { continue }
	seen[id] = true
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3003",
		Doc:  "integer-keyed map read in a loop",
		Run:  runPS3003,
	},
})

func isIntKeyMap(t types.Type) bool {
	m, ok := t.Underlying().(*types.Map)
	if !ok {
		return false
	}
	b, ok := m.Key().Underlying().(*types.Basic)
	if !ok {
		return false
	}
	return b.Info()&types.IsInteger != 0
}

func runPS3003(pass *analysis.Pass) (any, error) {
	type key struct {
		loop ast.Node
		name string
	}
	seen := map[key]bool{}
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			idx, ok := n.(*ast.IndexExpr)
			if !ok {
				return true
			}
			// Skip pure writes: m[i] = v (but keep compound reads like
			// m[i] += v, which read the slot first).
			if len(stack) > 0 {
				if as, ok := stack[len(stack)-1].(*ast.AssignStmt); ok && as.Tok.String() == "=" {
					for _, lhs := range as.Lhs {
						if lhs == ast.Node(idx) {
							return true
						}
					}
				}
			}
			loop, inLoop := astutil.InLoop(stack)
			if !inLoop {
				return true
			}
			t := pass.TypesInfo.TypeOf(idx.X)
			if t == nil || !isIntKeyMap(t) {
				return true
			}
			name := ""
			if id, ok := idx.X.(*ast.Ident); ok {
				name = id.Name
			}
			k := key{loop, name}
			if name != "" && seen[k] {
				return true
			}
			seen[k] = true
			label := name
			if label == "" {
				label = "map"
			}
			pass.Report(analysis.Diagnostic{
				Pos:     idx.Pos(),
				End:     idx.End(),
				Message: fmt.Sprintf("integer-keyed map %s probed in a loop pays a hash per probe; dense keys in a known range fit a slice", label),
			})
			return true
		})
	}
	return nil, nil
}
