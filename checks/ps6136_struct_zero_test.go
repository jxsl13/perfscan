package checks

import "testing"

func TestPS6136ClosedStructZero(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
type ops struct{fn func(); flag bool; other int}
func leaf(ops){}
func escape(*ops){}
func zero(){o:=ops{other:1};leaf(o)}
func explicit(){o:=ops{fn:nil,flag:false};leaf(o)}
func nonzero(){o:=ops{fn:func(){},flag:true};leaf(o)}
func borrowed(o ops){leaf(o)}
func mixed(){o:=ops{};o.other=1;leaf(o)}
func escaped(){o:=ops{};escape(&o);leaf(o)}
func late(){o:=ops{};leaf(o);o.fn=func(){}}
func captured(){o:=ops{};fn:=func(){o.flag=true};fn();leaf(o)}
`)
	for _, name := range []string{"zero", "explicit", "nonzero", "borrowed", "mixed", "escaped", "late", "captured"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			context := ps6125NewSSAContext(pkg.Func(name), nil, nil, 64)
			count := 0
			for _, call := range ps6125ContextCalls(context) {
				if call.Call.StaticCallee() != pkg.Func("leaf") {
					continue
				}
				for _, field := range []int{0, 1} {
					if got := ps6136StructFieldZero(context, call.Call.Args[0], field, 32); got != (name == "zero" || name == "explicit") {
						t.Fatalf("%s field %d zero = %v", name, field, got)
					}
				}
				count++
			}
			if count != 1 {
				t.Fatalf("leaf count %d", count)
			}
		})
	}
}
