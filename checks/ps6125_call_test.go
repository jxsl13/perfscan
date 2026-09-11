package checks

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6125SSACallBindings(t *testing.T) {
	t.Parallel()
	const source = `package extent
type owner struct { width int }
func target(value int) int { return value }
func other(value int) int { return value + 1 }
func direct(value int) int { return target(value) }
func captured(d *owner, value int) int {
 f := func(x int) int { return d.width + x }
 return f(value)
}
func selected(value int, choose bool) int {
 f := target
 if choose { f = other }
 return f(value)
}
func selectedClosure(d, sibling *owner, value int, choose bool) int {
 f := func(x int) int { return d.width + x }
 if choose { f = func(x int) int { return sibling.width + x } }
 return f(value)
}
func opaque(f func(int) int, value int) int { return f(value) }
func cyclic(value int, choose bool) int {
 f := target
 for choose { if value > 0 { f = other } }
 return f(value)
}
`
	pkg := ps6125TestSSA(t, source)
	for _, test := range []struct {
		name     string
		flag     *bool
		known    bool
		callee   string
		argument int
		captures int
	}{
		{"direct", nil, true, "target", 0, 0},
		{"captured", nil, true, "captured$1", 1, 1},
		{"selected", nil, false, "", 0, 0},
		{"selected", ps6125TestBool(false), true, "target", 0, 0},
		{"selected", ps6125TestBool(true), true, "other", 0, 0},
		{"selectedClosure", nil, false, "", 0, 0},
		{"selectedClosure", ps6125TestBool(false), true, "selectedClosure$1", 2, 1},
		{"selectedClosure", ps6125TestBool(true), true, "selectedClosure$2", 2, 1},
		{"opaque", nil, false, "", 1, 0},
		{"cyclic", nil, false, "", 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			function := pkg.Func(test.name)
			inputs := make(map[*ssa.Parameter]ps6125Scalar)
			if test.flag != nil {
				inputs[function.Params[len(function.Params)-1]] = ps6125Scalar{state: ps6125Boolean, truth: *test.flag}
			}
			flow := ps6125AnalyzeSSAExtents(function, inputs, nil)
			origins := &ps6125SSAOrigins{flow: flow}
			found := 0
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					call, ok := instruction.(*ssa.Call)
					if !ok || !flow.blocks[block] {
						continue
					}
					found++
					binding, known := origins.call(call)
					if known != test.known {
						t.Fatalf("call binding known=%v, want %v", known, test.known)
					}
					transferred, _, _ := flow.callInputs(call)
					if (transferred != nil) != (test.known && test.captures == 0) {
						t.Fatal("scalar transfer lost exact callable specialization or guessed captured memory")
					}
					if !known {
						continue
					}
					if binding.function.Name() != test.callee || len(binding.function.FreeVars) != test.captures || binding.values[binding.function.Params[0]] != function.Params[test.argument] {
						t.Fatalf("incorrect exact callee/argument/capture binding: %+v", binding)
					}
					origin := origins.resolve(call.Call.Value)
					if closure, ok := origin.value.(*ssa.MakeClosure); ok {
						for index, free := range binding.function.FreeVars {
							if binding.values[free] != closure.Bindings[index] {
								t.Fatal("captured cell was substituted with a guessed pointee")
							}
						}
					}
				}
			}
			if found != 1 {
				t.Fatalf("got %d calls, want 1", found)
			}
		})
	}
}
