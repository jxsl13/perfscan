package checks

import "testing"

func TestPS6136SourceZeroBranchSpecialization(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
type ops struct{fn func(); eager bool}
func small(){}
func large(){}
func mutate(*ops){}
func work(o ops){
 f:=func(){if o.fn==nil{small()}else{large()};if o.eager{large()}else{small()}}
 f()
}
func zero(){work(ops{})}
func explicit(){work(ops{fn:nil,eager:false})}
func nonzero(){work(ops{fn:func(){},eager:true})}
func unknown(o ops){work(o)}
func changed(o ops){f:=func(){if o.fn==nil{small()}else{large()}};mutate(&o);f()}
func assignment(o ops){f:=func(){if o.fn==nil{small()}else{large()}};o.fn=func(){};f()}
`)
	for _, name := range []string{"zero", "explicit", "nonzero", "unknown", "changed", "assignment"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			context := ps6125NewSSAContext(pkg.Func(name), nil, nil, 64)
			if name != "changed" && name != "assignment" {
				context = context.call(ps6125ContextCalls(context)[0])
			}
			var child *ps6125SSAContext
			for _, call := range ps6125ContextCalls(context) {
				if call.Call.StaticCallee() != pkg.Func("mutate") {
					child = ps6136ZeroSourceFlow(ps6136Call(context, call))
				}
			}
			if child == nil {
				t.Fatal("actual closed callback invocation unavailable")
			}
			small, large := 0, 0
			for _, call := range ps6125ContextCalls(child) {
				if call.Call.StaticCallee() == pkg.Func("small") {
					small++
				}
				if call.Call.StaticCallee() == pkg.Func("large") {
					large++
				}
			}
			wantSmall, wantLarge := 2, 2
			if name == "zero" || name == "explicit" {
				wantLarge = 0
			}
			if name == "changed" || name == "assignment" {
				wantSmall, wantLarge = 1, 1
			}
			if small != wantSmall || large != wantLarge {
				t.Fatalf("source branches small=%d large=%d, want %d,%d", small, large, wantSmall, wantLarge)
			}
		})
	}
}
