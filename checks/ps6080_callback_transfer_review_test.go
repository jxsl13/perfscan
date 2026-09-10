package checks

import (
	"go/ast"
	"go/types"
	"testing"
)

func TestPS6080CallbackGraphUnifiedWrapperTransfer(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`dispatch(func(){ func(){ f(31) }() })`,
		`dispatch(func(){ if false { opaque(f) }; f(31) })`,
	} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			pass := ps6080CallbackGrowthPass(t, "package probe\nvar opaque func(func(int))\nfunc dispatch(cb func()){ cb() }\nfunc walk(f func(int)){"+body+"}")
			graph := ps6080BuildCallbackGraph(pass)
			walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
			node := graph.nodes[ps6080CallbackNodeKey{function: walk, parameter: 0}]
			if len(node.reachable) != 1 || node.unknown {
				t.Fatalf("shared transfer lost reachable source call or fabricated unreachable escape: %+v", node)
			}
			for site := range node.reachable {
				if len(site.call.Args) != 1 {
					t.Fatal("wrapper substituted IIFE invocation for callback terminal")
				}
				argument, ok := site.call.Args[0].(*ast.BasicLit)
				if !ok || argument.Value != "31" {
					t.Fatal("wrapper lost actual callback argument")
				}
			}
		})
	}
}
