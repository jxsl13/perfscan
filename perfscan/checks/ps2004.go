package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2004 reports per-call scratch make() bound to a non-escaping local in a
// pointer-method loop.
var PS2004 = register(&lint.Check{
	ID:       "PS2004",
	Category: "alloc",
	Slug:     "poolable-loop-scratch",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "per-call scratch make() bound to a non-escaping local in a pointer-method loop",
		Text: `A slice make() inside a per-item loop of a pointer-receiver
method, bound to a local that does not escape (not returned, not stored into
a field or slot), is scratch reallocated on every call. On a reusable
stateful object — an optimizer stepping over its parameters — that is pure
GC churn. Hoist it to a reused receiver field (grow-on-demand; zero it only
if it is read before written).

Ring slots and returned buffers escape and are deliberately excluded — they
need a different fix. Pointer-element slices (make([]*T, …)) are skipped:
pooling a pointer-slice header is negligible churn dwarfed by the elements'
own allocations.

Verify the buffer is fully overwritten before use before hoisting.`,
		Before: `func (o *Optimizer) Step(params []Param) {
	for _, p := range params {
		grad := make([]float64, p.N())
		o.apply(p, grad)
	}
}`,
		After: `func (o *Optimizer) Step(params []Param) {
	for _, p := range params {
		o.scratch = growF64(o.scratch, p.N()) // reused receiver field
		o.apply(p, o.scratch)
	}
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2004",
		Doc:  "poolable per-call scratch in a pointer-method loop",
		Run:  runPS2004,
	},
})

func runPS2004(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !isPointerMethod(fn) {
				continue
			}
			escaping := collectEscaping(fn.Body)
			astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isSliceMake(call) {
					return true
				}
				// Skip pointer-element slices: orchestration scaffolding,
				// not numeric scratch.
				if at, ok := call.Args[0].(*ast.ArrayType); ok {
					if _, isPtr := at.Elt.(*ast.StarExpr); isPtr {
						return true
					}
				}
				loop, inLoop := astutil.InLoop(stack)
				if !inLoop {
					return true
				}
				as, ok := stack[len(stack)-1].(*ast.AssignStmt)
				if !ok {
					return true
				}
				for i, rhs := range as.Rhs {
					if rhs != ast.Expr(call) || i >= len(as.Lhs) {
						continue
					}
					id, ok := as.Lhs[i].(*ast.Ident)
					if !ok || id.Name == "_" || escaping[id.Name] {
						continue
					}
					pass.Report(analysis.Diagnostic{
						Pos:     loop.Pos(),
						End:     as.End(),
						Message: id.Name + ": make() per iteration of a pointer-method loop, bound to a non-escaping local — per-call scratch reallocated every call; hoist to a reused receiver field (grow-on-demand, zero only if read before written)",
					})
				}
				return true
			})
		}
	}
	return nil, nil
}
