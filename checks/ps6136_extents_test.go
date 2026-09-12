package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

// Unit tests for the extent layer, not a substitute for complete owner replay.
func TestPS6136InvocationExtents(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
type owner struct{width int; other int}
func leaf([]float32){}
func work(d,sibling *owner,tokens []int,lastOnly bool){
 rows:=len(tokens);if lastOnly{rows=1}
 leaf(make([]float32,rows*d.width))
 leaf(make([]float32,rows*sibling.width))
 leaf(make([]float32,rows*d.other))
}
func common(d,sibling *owner,tokens []int){work(d,sibling,tokens,true)}
func bulk(d,sibling *owner,tokens []int){work(d,sibling,tokens,false)}
`)
	for _, name := range []string{"common", "bulk"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			entry := pkg.Func(name)
			var pool ps6125ExtentPool
			structure := pkg.Pkg.Scope().Lookup("owner").Type().Underlying().(*types.Struct)
			width := ps6125SymbolicExtent(pool.identity(entry.Params[0].Object(), []*types.Var{structure.Field(0)}, false))
			rows := ps6125SymbolicExtent(pool.identity(entry.Params[2].Object(), nil, true))
			root := ps6125NewSSAContext(entry, nil, map[*ssa.Parameter]ps6125Extent{entry.Params[2]: rows}, 8)
			child := root.call(ps6125ContextCalls(root)[0])
			if child == nil {
				t.Fatal("exact helper invocation unavailable")
			}
			facts := ps6136Extents{owner: root.reference(entry.Params[0]), immutable: map[*types.Var]ps6125Extent{structure.Field(0): width}, remaining: 1000}
			var calls []*ssa.Call
			for _, call := range ps6125ContextCalls(child) {
				if call.Call.StaticCallee() == pkg.Func("leaf") {
					calls = append(calls, call)
				}
			}
			if len(calls) != 3 {
				t.Fatalf("leaf count %d", len(calls))
			}
			active := rows
			if name == "common" {
				active = ps6125ConstantExtent(1)
			}
			want := ps6125MultiplyExtents(active, width)
			if got := facts.length(child, calls[0].Call.Args[0]); !ps6125SameExtent(got, want) {
				t.Fatal("actual owner specialized host extent unavailable")
			}
			for _, call := range calls[1:] {
				if facts.length(child, call.Call.Args[0]).known {
					t.Fatal("sibling owner/unknown field became immutable geometry")
				}
			}
			facts.remaining = 0
			if facts.length(child, calls[0].Call.Args[0]).known {
				t.Fatal("exhausted proof budget accepted")
			}
		})
	}
}
