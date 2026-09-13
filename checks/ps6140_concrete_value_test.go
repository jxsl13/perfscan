package checks

import (
	"go/types"
	"testing"
)

func TestPS6140ConcretePhiBoundaries(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
type api interface{run()};type f32 struct{n int};func(f32)run(){};type quant struct{};func(quant)run(){}
func leaf(api){};func unknown()api
func same(flag bool){var x api=f32{1};if flag{x=f32{2}};leaf(x)}
func mixed(flag bool){var x api=f32{1};if flag{x=quant{}};leaf(x)}
func unresolved(flag bool){var x api=f32{1};if flag{x=unknown()};leaf(x)}
func cycle(flag bool){var x api=f32{1};for flag{if flag{x=f32{2}};flag=false};leaf(x)}
`)
	concrete := pkg.Pkg.Scope().Lookup("f32").Type().(*types.Named)
	for _, name := range []string{"same", "mixed", "unresolved", "cycle"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			context := ps6125NewSSAContext(pkg.Func(name), nil, nil, 64)
			found := false
			for _, call := range ps6125ContextCalls(context) {
				if call.Call.StaticCallee() != pkg.Func("leaf") {
					continue
				}
				found = true
				if got := ps6140ConcreteValue(context, call.Call.Args[0], concrete, 32); got != (name == "same") {
					t.Fatalf("%s concrete=%v", name, got)
				}
				if ps6140ConcreteValue(context, call.Call.Args[0], concrete, 1) {
					t.Fatal("finite total work budget exhausted without rejection")
				}
			}
			if !found {
				t.Fatal("missing typed leaf call")
			}
		})
	}
}
