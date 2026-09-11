package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func ps6125ContextCalls(context *ps6125SSAContext) []*ssa.Call {
	var result []*ssa.Call
	for _, block := range context.flow.function.Blocks {
		if context.flow.blocks[block] {
			for _, instruction := range block.Instrs {
				if call, ok := instruction.(*ssa.Call); ok {
					result = append(result, call)
				}
			}
		}
	}
	return result
}

func TestPS6125ContextReturnedArguments(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
type slot struct { data []float32 }
func wrap(data []float32) *slot { return &slot{data:data} }
func relay(data []float32) *slot { return wrap(data) }
func roots(first, second []float32) { relay(first); relay(second) }
`)
	function := pkg.Func("roots")
	root := ps6125NewSSAContext(function, nil, map[*ssa.Parameter]ps6125Extent{
		function.Params[0]: ps6125ConstantExtent(16), function.Params[1]: ps6125ConstantExtent(32),
	}, 8)
	calls := ps6125ContextCalls(root)
	if len(calls) != 2 {
		t.Fatal("changed two-call fixture")
	}
	field := pkg.Pkg.Scope().Lookup("slot").Type().Underlying().(*types.Struct).Field(0)
	var allocations []ps6125SSAReference
	for index, call := range calls {
		relay := root.call(call)
		if relay == nil || root.call(call) != relay {
			t.Fatal("exact call context was not memoized")
		}
		wrapper := relay.call(ps6125ContextCalls(relay)[0])
		if wrapper == nil {
			t.Fatal("helper chain lost its exact callee")
		}
		if !ps6125SameExtent(wrapper.flow.lengths[wrapper.flow.function.Params[0]], ps6125ConstantExtent(int64((index+1)*16))) {
			t.Fatal("caller descriptor lengths were merged")
		}
		returned := wrapper.flow.function.Blocks[0].Instrs[len(wrapper.flow.function.Blocks[0].Instrs)-1].(*ssa.Return)
		allocation, values, known := wrapper.returnedFields(returned, 0)
		want := ps6125SSAReference{context: root, value: function.Params[index]}
		if !known || values[field] != want {
			t.Fatalf("returned field lost exact caller argument: known=%v source=%+v", known, values[field])
		}
		allocations = append(allocations, allocation)
	}
	if allocations[0].value != allocations[1].value || allocations[0].context == allocations[1].context {
		t.Fatal("same helper allocation site lost distinct invocation contexts")
	}
}

func TestPS6125ContextForwardedClosure(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
type owner struct{ width int }
type slot struct{ data []float32 }
func invoke(f func([]float32)*slot, data []float32) *slot { return f(data) }
func captured(d *owner, data []float32) *slot {
 mk := func(value []float32)*slot { _ = d.width; return &slot{data:value} }
 return invoke(mk,data)
}
`)
	function := pkg.Func("captured")
	root := ps6125NewSSAContext(function, nil, nil, 8)
	call := ps6125ContextCalls(root)[0]
	closure, ok := call.Call.Args[0].(*ssa.MakeClosure)
	if !ok || len(closure.Bindings) != 1 {
		t.Fatal("changed captured-cell fixture")
	}
	relay := root.call(call)
	child := relay.call(ps6125ContextCalls(relay)[0])
	if child == nil || len(child.flow.function.FreeVars) != 1 {
		t.Fatal("closure target was lost through a function parameter")
	}
	if root.reference(child.flow.function.FreeVars[0]).value != nil {
		t.Fatal("foreign capture assigned the wrong invocation context")
	}
	if got := child.reference(child.flow.function.FreeVars[0]); got != root.reference(closure.Bindings[0]) || got.context != root {
		t.Fatal("capture rebound to the invoking helper instead of its creating context")
	}
	if child.reference(child.flow.function.Params[0]) != root.reference(function.Params[1]) {
		t.Fatal("forwarded closure argument lost its root context")
	}
	for _, block := range child.flow.function.Blocks {
		for _, instruction := range block.Instrs {
			if load, ok := instruction.(*ssa.UnOp); ok {
				if child.reference(load) != (ps6125SSAReference{context: child, value: load}) || child.scalar(load).state != ps6125Unknown {
					t.Fatal("captured memory was replaced with an inferred pointee or invariant")
				}
			}
		}
	}
}

func TestPS6125ContextCallableJoins(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
func first(x int) int {return x}
func second(x int) int {return x*2}
func invoke(a,b func(int)int, x int, choose bool) int { f:=a; if choose {f=b}; return f(x) }
func same(x int, choose bool)int {return invoke(first,first,x,choose)}
func mixed(x int, choose bool)int {return invoke(first,second,x,choose)}
func cycle(a,b func(int)int, x int, choose bool)int {
 f:=a; for choose {if x>0 {f=b}}; return f(x)
}
func cyclic(x int, choose bool)int {return cycle(first,second,x,choose)}
`)
	for _, name := range []string{"same", "mixed", "cyclic"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := ps6125NewSSAContext(pkg.Func(name), nil, nil, 8)
			relay := root.call(ps6125ContextCalls(root)[0])
			child := relay.call(ps6125ContextCalls(relay)[0])
			if (child != nil) != (name == "same") {
				t.Fatalf("callable join %s incorrectly resolved: %v", name, child != nil)
			}
		})
	}
}

func TestPS6125ContextBoundsAndMutableLengths(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
func leaf(data map[int]int)int{return len(data)}
func relay(data map[int]int)int{return leaf(data)}
func root(data map[int]int)int{return relay(data)}
func recursive(x int)int{return recursive(x)}
`)
	function := pkg.Func("root")
	lengths := map[*ssa.Parameter]ps6125Extent{function.Params[0]: ps6125ConstantExtent(16)}
	lengths[pkg.Func("relay").Params[0]] = ps6125ConstantExtent(64)
	root := ps6125NewSSAContext(function, nil, lengths, 2)
	if len(root.flow.lengths) != 1 {
		t.Fatal("foreign input length retained in the invocation")
	}
	lengths[function.Params[0]] = ps6125ConstantExtent(99)
	if !ps6125SameExtent(root.flow.lengths[function.Params[0]], ps6125ConstantExtent(16)) {
		t.Fatal("caller mutated completed context length facts")
	}
	call := ps6125ContextCalls(root)[0]
	relay := root.call(call)
	if relay == nil || root.call(call) != relay || root.budget.remaining != 0 {
		t.Fatal("context budget or memoization is incorrect")
	}
	if relay.flow.lengths[relay.flow.function.Params[0]].known {
		t.Fatal("mutable map length transferred as a stable descriptor fact")
	}
	if relay.call(ps6125ContextCalls(relay)[0]) != nil {
		t.Fatal("context allocation exceeded its explicit budget")
	}
	if root.call(ps6125ContextCalls(relay)[0]) != nil {
		t.Fatal("foreign call accepted")
	}
	if root.reference(relay.flow.function.Params[0]).value != nil {
		t.Fatal("foreign parameter assigned the wrong invocation context")
	}
	if ps6125NewSSAContext(function, nil, nil, 0) != nil {
		t.Fatal("empty budget allocated a root context")
	}
	recursive := ps6125NewSSAContext(pkg.Func("recursive"), nil, nil, 8)
	if recursive.call(ps6125ContextCalls(recursive)[0]) != nil {
		t.Fatal("recursive context was expanded")
	}
}
