package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6136CapturedOwnerIdentity(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body string
		known      bool
	}{
		{"readonly", `d:=input;mk:=func(){leaf(d)};mk()`, true},
		{"initialize_after_creation", `var d *owner;mk:=func(){leaf(d)};d=input;mk()`, true},
		{"initialize_after_call", `var d *owner;mk:=func(){leaf(d)};mk();d=input;_=d`, false},
		{"second_store", `d:=input;mk:=func(){leaf(d)};d=&owner{};mk()`, false},
		{"mixed_short_store", `d:=input;mk:=func(){leaf(d)};d,z:= &owner{},0;_=z;mk()`, false},
		{"captured_write", `d:=input;mk:=func(){d=&owner{};leaf(d)};mk()`, false},
		{"escaped_cell", `d:=input;mk:=func(){leaf(d)};expose(&d);mk()`, false},
		{"opaque_owner", `d:=opaque(input);mk:=func(){leaf(d)};mk()`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent;type owner struct{width int};func leaf(*owner){};func expose(**owner){};func opaque(input *owner)*owner{return input};func root(input *owner){`+test.body+`}`)
			function := pkg.Func("root")
			root := ps6125NewSSAContext(function, nil, nil, 8)
			var child *ps6125SSAContext
			for _, call := range ps6125ContextCalls(root) {
				if next := root.call(call); next != nil && len(next.flow.function.FreeVars) > 0 {
					child = next
					break
				}
			}
			if child == nil {
				t.Fatal("closed factory invocation unavailable")
			}
			var value ssa.Value
			for _, call := range ps6125ContextCalls(child) {
				if call.Call.StaticCallee() == pkg.Func("leaf") {
					value = call.Call.Args[0]
					break
				}
			}
			if value == nil {
				t.Fatal("owner leaf argument unavailable")
			}
			owner := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			got := ps6136OwnerRoot(child, value, owner)
			want := root.reference(function.Params[0])
			if (got == want) != test.known {
				t.Fatalf("captured owner identity=%v want %v", got == want, test.known)
			}
		})
	}
}
