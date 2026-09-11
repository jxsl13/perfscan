package checks

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6125StructCallbackBindings(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
type operations struct { allocate func([]float32) int; spare func([]float32) int }
func first(data []float32)int{return len(data)}
func second(data []float32)int{return len(data)*2}
func construct(ops operations, data []float32)int {
 mk:=func(input []float32)int{return ops.allocate(input)}
 return mk(data)
}

func direct(ops operations, data []float32)int{return ops.allocate(data)}
func known(data []float32)int{return construct(operations{allocate:first,spare:second},data)}
func sibling(data []float32)int{return direct(operations{allocate:second,spare:first},data)}
func unknown(ops operations,data []float32)int{return construct(ops,data)}
func overwritten(data []float32)int {
 ops:=operations{allocate:first}; ops.allocate=second
 return construct(ops,data)
}
func escape(*operations){}
func escaped(data []float32)int {
 ops:=operations{allocate:first}; escape(&ops)
 return construct(ops,data)
}
func changed(ops operations,data []float32)int {
 mk:=func(input []float32)int{return ops.allocate(input)}
 ops.allocate=second
 return mk(data)
}
func changedCapture(data []float32)int{return changed(operations{allocate:first},data)}
func mixed(data []float32, choose bool)int {
 a:=operations{allocate:first}; b:=operations{allocate:second}
 ops:=a; if choose {ops=b}
 return construct(ops,data)
}
`)
	for _, name := range []string{"known", "sibling", "unknown", "overwritten", "escaped", "changedCapture", "mixed"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			function := pkg.Func(name)
			root := ps6125NewSSAContext(function, nil, nil, 12)
			calls := ps6125ContextCalls(root)
			child := root.call(calls[len(calls)-1])
			if child == nil {
				t.Fatal("direct constructor call lost")
			}
			if name != "sibling" {
				child = child.call(ps6125ContextCalls(child)[0])
				if child == nil {
					t.Fatal("captured factory call lost")
				}
			}
			allocated := child.call(ps6125ContextCalls(child)[0])
			want := (*ssa.Function)(nil)
			if name == "known" {
				want = pkg.Func("first")
			} else if name == "sibling" {
				want = pkg.Func("second")
			}
			if want == nil {
				if allocated != nil {
					t.Fatal("unproved operations storage supplied a callback target")
				}
				return
			}
			if allocated == nil || allocated.flow.function != want {
				t.Fatalf("typed operations field callback missing: got %v, want %s", allocated != nil, want)
			}
			if allocated.reference(allocated.flow.function.Params[0]) != root.reference(function.Params[0]) {
				t.Fatal("backend callback lost the exact caller input")
			}
		})
	}
}

func TestPS6125StructCapturedInitialization(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
type operations struct{ allocate func(int)int }
func target(x int)int{return x}
func other(x int)int{return x*2}
var saved func(int)int
var exported *func(int)int
func initialized(x int)int {
 var ops operations
 read:=func()int{return ops.allocate(x)}
 ops.allocate=target
 return read()
}
func early(x int)int {
 var ops operations
 read:=func()int{return ops.allocate(x)}
 result:=read()
 ops.allocate=target
 return result
}
func conditional(x int, choose bool)int {
 var ops operations
 read:=func()int{return ops.allocate(x)}
 if choose {ops.allocate=target}
 return read()
}
func writing(x int)int {
 ops:=operations{allocate:target}
 saved=func(v int)int {ops.allocate=other;return v}
 read:=func()int{return ops.allocate(x)}
 return read()
}
func fieldEscape(x int)int {
 ops:=operations{allocate:target}
 exported=&ops.allocate
 read:=func()int{return ops.allocate(x)}
 return read()
}
`)
	for _, name := range []string{"initialized", "early", "conditional", "writing", "fieldEscape"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := ps6125NewSSAContext(pkg.Func(name), nil, nil, 8)
			reader := root.call(ps6125ContextCalls(root)[0])
			if reader == nil {
				t.Fatal("source reader context missing")
			}
			callback := reader.call(ps6125ContextCalls(reader)[0])
			if (callback != nil) != (name == "initialized") {
				t.Fatal("mutable, escaping or uninitialized captured struct supplied a callback")
			}
		})
	}
}

func TestPS6125StructRepeatedConstructorContexts(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
type operations struct { allocate func([]float32)int }
func first(data []float32)int{return len(data)}
func second(data []float32)int{return len(data)*2}
func construct(ops operations,data []float32)int {
 return func()int{return ops.allocate(data)}()
}
func roots(a,b []float32){
 construct(operations{allocate:first},a)
 construct(operations{allocate:second},b)
}
`)
	function := pkg.Func("roots")
	root := ps6125NewSSAContext(function, nil, nil, 12)
	calls := ps6125ContextCalls(root)
	if len(calls) != 2 {
		t.Fatal("changed two-constructor fixture")
	}
	for index, call := range calls {
		child := root.call(call)
		if child == nil {
			t.Fatal("constructor context missing")
		}
		factory := child.call(ps6125ContextCalls(child)[0])
		if factory == nil {
			t.Fatal("factory context missing")
		}
		callback := factory.call(ps6125ContextCalls(factory)[0])
		want := pkg.Func("first")
		if index == 1 {
			want = pkg.Func("second")
		}
		if callback == nil || callback.flow.function != want {
			t.Fatal("separate constructor instances conflated callback targets")
		}
		// Captured data is a memory load, not a stable descriptor binding.
		// The callback target is known independently of that loaded content.
		if callback.reference(callback.flow.function.Params[0]).context != factory {
			t.Fatal("captured data load moved to the wrong source context")
		}
	}
}
