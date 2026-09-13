package checks

import "testing"

func TestPS6136ReturnedCallableBoundaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, maker, body string
		known             bool
	}{
		{"closed_return", `return func(){leaf()}`, `mk:=makeFactory();mk()`, true},
		{"returned_named", `return leaf`, `mk:=makeFactory();mk()`, true},
		{"unknown_dispatch", `if flag{return leaf};return other`, `mk:=makeFactory();mk()`, false},
		{"nil_dispatch", `if flag{return leaf};return nil`, `mk:=makeFactory();mk()`, false},
		{"rebound_cell", `return leaf`, `mk:=makeFactory();mk=other;mk()`, true},
		{"escaped_cell", `return leaf`, `mk:=makeFactory();expose(&mk);mk()`, false},
		{"captured_write", `return leaf`, `mk:=makeFactory();change:=func(){mk=other};change();mk()`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent;var flag bool;func leaf(){};func other(){};func expose(*func()){};func makeFactory()func(){`+test.maker+`};func root(){`+test.body+`}`)
			root := ps6125NewSSAContext(pkg.Func("root"), nil, nil, 24)
			found, known := false, false
			for _, call := range ps6125ContextCalls(root) {
				if call.Call.StaticCallee() != nil {
					continue
				}
				found = true
				known = ps6136Call(root, call) != nil
			}
			// A compiler-proven direct replacement dispatch is accepted only
			// as that actual replacement, never as the original allocator.
			if !found && test.name == "rebound_cell" {
				for _, call := range ps6125ContextCalls(root) {
					if call.Call.StaticCallee() == pkg.Func("other") {
						found, known = true, true
					}
				}
			}
			if !found || known != test.known {
				t.Fatalf("dispatch found=%v closed=%v, want %v", found, known, test.known)
			}
		})
	}
}
