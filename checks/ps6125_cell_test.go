package checks

import "testing"

func TestPS6125ClosedFunctionCells(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
func target(x int)int{return x}
func other(x int)int{return x*2}
var saved func()int
var exported *func(int)int
func escape(*func(int)int){}
func closed(x int)int {f:=target; saved=func()int{return f(x)}; return f(x)}
func exportedCell(x int)int {f:=target; exported=&f; return f(x)}
func calledCell(x int)int {f:=target; escape(&f); return f(x)}
func mutated(x int)int {f:=target; saved=func()int{f=other; return f(x)}; return f(x)}
func overwritten(x int)int {f:=target; saved=func()int{return f(x)}; f=other; return f(x)}
func conditional(x int, choose bool)int {
 var f func(int)int
 saved=func()int{return f(x)}
 if choose {f=target}
 return f(x)
}
func nested(x int)int {
 f:=target
 return func()int{return func()int{return f(x)}()}()
}
func initializedAfterCapture(x int)int {
 var f func(int)int
 read:=func()int{return f(x)}
 f=target
 return read()
}
func invokedBeforeInitialization(x int)int {
 var f func(int)int
 read:=func()int{return f(x)}
 result:=read()
 f=target
 return result
}
`)
	for _, name := range []string{"closed", "exportedCell", "calledCell", "mutated", "overwritten", "conditional", "nested"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := ps6125NewSSAContext(pkg.Func(name), nil, nil, 8)
			calls := ps6125ContextCalls(root)
			call := calls[len(calls)-1]
			child := root.call(call)
			known := name == "closed" || name == "nested"
			if (child != nil) != known {
				t.Fatalf("%s closed callable=%v, want %v", name, child != nil, known)
			}
			if name == "nested" {
				child = child.call(ps6125ContextCalls(child)[0])
				if child == nil {
					t.Fatal("nested read-only capture context lost")
				}
				child = child.call(ps6125ContextCalls(child)[0])
			}
			if known && (child == nil || child.flow.function != pkg.Func("target")) {
				t.Fatal("closed cell did not retain its exact callable target")
			}
		})
	}
	for _, name := range []string{"initializedAfterCapture", "invokedBeforeInitialization"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := ps6125NewSSAContext(pkg.Func(name), nil, nil, 8)
			reader := root.call(ps6125ContextCalls(root)[0])
			if reader == nil {
				t.Fatal("exact reader closure missing")
			}
			callee := reader.call(ps6125ContextCalls(reader)[0])
			if (callee != nil) != (name == "initializedAfterCapture") {
				t.Fatal("cell initialization was not checked at invocation time")
			}
		})
	}
}
